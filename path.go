package main

import (
	"errors"
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
	rel := strings.TrimPrefix(clean, "/")
	full := filepath.Join(rootAbs, filepath.FromSlash(rel))
	// Ensure full stays inside rootAbs.
	if full != rootAbs && !strings.HasPrefix(full, rootAbs+string(filepath.Separator)) {
		return "", ErrBadPath
	}
	return full, nil
}

// ValidName checks a single path segment (file or folder name).
func ValidName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrBadPath
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return ErrBadPath
	}
	return nil
}
