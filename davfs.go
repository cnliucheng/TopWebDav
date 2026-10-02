package main

import (
	"context"
	"os"

	"golang.org/x/net/webdav"
)

// davFS adapts the API's symlink-aware path resolution (SanitizePath +
// ResolveUnder) to webdav.FileSystem so /dav/ has the same containment rules
// as /api/*: ".." segments are rejected and symlinks inside the data root
// cannot escape it — webdav.Dir, unlike this, follows symlinks anywhere.
// Like webdav.Dir does for its own unresolvable names, escapes surface as
// bare os.ErrNotExist (not a wrapper: the handler's os.IsNotExist checks do
// not unwrap) so the handler answers 404 without revealing anything about
// paths outside the root.
type davFS struct {
	root string
}

var _ webdav.FileSystem = davFS{}

func (d davFS) resolve(name string) (string, error) {
	full, err := ResolveUnder(d.root, name)
	if err != nil {
		return "", os.ErrNotExist
	}
	return full, nil
}

// resolveEntry addresses the final path component itself when it is a
// symlink (POSIX remove/rename semantics), so DELETE and MOVE act on the
// link instead of dragging its target around. Containment of the target is
// still checked by ResolveEntry.
func (d davFS) resolveEntry(name string) (string, error) {
	full, err := ResolveEntry(d.root, name)
	if err != nil {
		return "", os.ErrNotExist
	}
	return full, nil
}

func (d davFS) rootPath() (string, error) {
	full, err := ResolveUnder(d.root, "/")
	if err != nil {
		return "", os.ErrNotExist
	}
	return full, nil
}

func (d davFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	full, err := d.resolve(name)
	if err != nil {
		return err
	}
	return os.Mkdir(full, perm)
}

func (d davFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	full, err := d.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(full, flag, perm)
}

func (d davFS) RemoveAll(ctx context.Context, name string) error {
	full, err := d.resolveEntry(name)
	if err != nil {
		return err
	}
	root, err := d.rootPath()
	if err != nil {
		return err
	}
	if full == root {
		// Prohibit removing the data root, mirroring webdav.Dir.
		return os.ErrInvalid
	}
	return os.RemoveAll(full)
}

func (d davFS) Rename(ctx context.Context, oldName, newName string) error {
	src, err := d.resolveEntry(oldName)
	if err != nil {
		return err
	}
	dst, err := d.resolveEntry(newName)
	if err != nil {
		return err
	}
	root, err := d.rootPath()
	if err != nil {
		return err
	}
	if src == root || dst == root {
		// Prohibit renaming from or to the data root, mirroring webdav.Dir.
		return os.ErrInvalid
	}
	return os.Rename(src, dst)
}

func (d davFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	full, err := d.resolve(name)
	if err != nil {
		return nil, err
	}
	return os.Stat(full)
}
