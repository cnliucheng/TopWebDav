package main

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrBadPath is returned for illegal or escaping paths.
var ErrBadPath = errors.New("bad path")

// SanitizePath cleans a client path into slash form under root ("/a/b").
func SanitizePath(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", ErrBadPath
	}
	p = strings.TrimSpace(p)
	if p == "" {
		return "/", nil
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	// Reject ".." segments in the input. path.Clean would collapse them and
	// can silently accept escape attempts like "/../etc/passwd" → "/etc/passwd".
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", ErrBadPath
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", ErrBadPath
	}
	if !strings.HasPrefix(clean, "/") {
		return "", ErrBadPath
	}
	return clean, nil
}

// realPathUnder resolves full's deepest existing prefix via EvalSymlinks,
// re-appends any missing tail without allowing "..", and requires the result
// to stay under realRoot.
func realPathUnder(realRoot, full string) (string, error) {
	existing := full
	var tail []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		tail = append([]string{filepath.Base(existing)}, tail...)
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", ErrBadPath
	}
	joined := filepath.Join(append([]string{real}, tail...)...)
	// Containment via Rel handles filesystem roots ("/", "C:\\") correctly;
	// a plain HasPrefix(root+sep) false-rejects when rootAbs is a volume root.
	rel, err := filepath.Rel(realRoot, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrBadPath
	}
	return joined, nil
}

// ResolveUnder maps a sanitized path to an absolute OS path under root.
func ResolveUnder(root, p string) (string, error) {
	clean, err := SanitizePath(p)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	relPath := strings.TrimPrefix(clean, "/")
	full := filepath.Join(rootAbs, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(rootAbs, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrBadPath
	}
	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	return realPathUnder(realRoot, full)
}

// ResolveEntry is ResolveUnder with one difference: when the final path
// component is a symlink, the link itself is returned rather than its
// target — while still verifying that the target stays inside root. This
// matches how rm and rename(2) treat their operands: they act on the
// directory entry, not on what it points at. Read-style operations
// (open, stat, read) should keep using ResolveUnder, which follows links.
// The root path ("/") never resolves to a link, so root-refusal checks
// keep working when data_dir itself is a symlink.
func ResolveEntry(root, p string) (string, error) {
	full, err := ResolveUnder(root, p)
	if err != nil {
		return "", err
	}
	clean, err := SanitizePath(p)
	if err != nil {
		return "", err
	}
	if clean == "/" {
		return full, nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	entry := filepath.Join(rootAbs, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
	if fi, err := os.Lstat(entry); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return entry, nil
	}
	return full, nil
}

// ValidName checks a single path segment (file or folder name).
func ValidName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrBadPath
	}
	if strings.ContainsAny(name, "/\\\";") {
		return ErrBadPath
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return ErrBadPath
		}
	}
	return nil
}
