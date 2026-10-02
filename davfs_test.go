package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newDavFS(t *testing.T) davFS {
	t.Helper()
	return davFS{root: t.TempDir()}
}

func TestDavFSRoundTrip(t *testing.T) {
	fs := newDavFS(t)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "/docs", 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := fs.OpenFile(ctx, "/docs/a.txt", os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	fi, err := fs.Stat(ctx, "/docs/a.txt")
	if err != nil || fi.Size() != 2 {
		t.Fatalf("stat size=%d err=%v", fi.Size(), err)
	}
	if err := fs.Rename(ctx, "/docs/a.txt", "/docs/b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(ctx, "/docs/a.txt"); !os.IsNotExist(err) {
		t.Fatalf("old name err=%v want not exist", err)
	}
	if err := fs.RemoveAll(ctx, "/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(ctx, "/docs/b.txt"); !os.IsNotExist(err) {
		t.Fatalf("removed dir err=%v want not exist", err)
	}
}

func TestDavFSRefusesRootOps(t *testing.T) {
	fs := newDavFS(t)
	ctx := context.Background()
	if err := fs.RemoveAll(ctx, "/"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("remove root err=%v want ErrInvalid", err)
	}
	if err := fs.Rename(ctx, "/", "/x"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("rename root err=%v want ErrInvalid", err)
	}
}

func TestDavFSRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	fs := davFS{root: root}
	ctx := context.Background()

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(outside, "dir")
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outsideDir, "inner.txt")
	if err := os.WriteFile(inner, []byte("INNER"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "dirlink")); err != nil {
		t.Fatal(err)
	}

	// Reading through the escaping symlink must fail...
	f, err := fs.OpenFile(ctx, "/leak", os.O_RDONLY, 0)
	if err == nil {
		f.Close()
		t.Fatal("open through escape symlink unexpectedly succeeded")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err=%v want bare os.ErrNotExist", err)
	}
	if _, err := fs.Stat(ctx, "/leak"); err == nil {
		t.Fatal("stat through escape symlink unexpectedly succeeded")
	}
	// ...including when the escape is in a parent component...
	if _, err := fs.OpenFile(ctx, "/dirlink/inner.txt", os.O_RDONLY, 0); err == nil {
		t.Fatal("read under dir symlink unexpectedly succeeded")
	}
	if _, err := fs.OpenFile(ctx, "/dirlink/planted.txt", os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		t.Fatal("create under dir symlink unexpectedly succeeded")
	}
	if b, rerr := os.ReadFile(inner); rerr != nil || string(b) != "INNER" {
		t.Fatalf("outside file changed: %q err=%v", b, rerr)
	}
	// ...and every mutating operation through the link must be refused.
	if err := fs.Mkdir(ctx, "/dirlink/newdir", 0o700); err == nil {
		t.Fatal("mkdir under dir symlink unexpectedly succeeded")
	}
	if err := fs.RemoveAll(ctx, "/leak"); err == nil {
		t.Fatal("removeall of escape symlink unexpectedly succeeded")
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("outside target must survive: %v", err)
	}
	if err := fs.Rename(ctx, "/leak", "/moved"); err == nil {
		t.Fatal("rename of escape symlink unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, "moved")); !os.IsNotExist(err) {
		t.Fatal("escape symlink was moved")
	}
}

func TestDavFSRemoveAllSymlink(t *testing.T) {
	root := t.TempDir()
	fs := davFS{root: root}
	ctx := context.Background()

	if err := os.Mkdir(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "docslink")); err != nil {
		t.Fatal(err)
	}

	// DELETE on the link must remove the link only — not drag its
	// non-empty target away the way a resolved path would.
	if err := fs.RemoveAll(ctx, "/docslink"); err != nil {
		t.Fatalf("removeall symlink: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "docslink")); !os.IsNotExist(err) {
		t.Fatalf("symlink still present, err=%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "docs", "keep.txt")); err != nil || string(b) != "keep" {
		t.Fatalf("target disturbed: %q err=%v", b, err)
	}

	// MOVE on a symlink moves the link itself too.
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "again")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Rename(ctx, "/again", "/moved"); err != nil {
		t.Fatalf("rename symlink: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(root, "moved"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("moved entry should be the symlink, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "keep.txt")); err != nil {
		t.Fatalf("target after rename: %v", err)
	}

	// Dangling links are removable as well.
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := fs.RemoveAll(ctx, "/dangling"); err != nil {
		t.Fatalf("removeall dangling: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "dangling")); !os.IsNotExist(err) {
		t.Fatalf("dangling link still present, err=%v", err)
	}
}

func TestDavHandlerBlocksEscapeOverHTTP(t *testing.T) {
	s := testServer(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(s.root, "leak")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "ok.txt"), []byte("fine"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := s.routes()

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetBasicAuth("admin", "admin")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/dav/leak"); rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "SECRET") {
		t.Fatalf("escaped GET code=%d body=%q", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("PROPFIND", "/dav/leak", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("escaped PROPFIND code=%d want 404", rec.Code)
	}
	// Sanity: ordinary files inside the root are still served.
	if rec := get("/dav/ok.txt"); rec.Code != http.StatusOK || rec.Body.String() != "fine" {
		t.Fatalf("normal GET code=%d body=%q", rec.Code, rec.Body.String())
	}
	// PUT still creates files inside the root.
	req = httptest.NewRequest(http.MethodPut, "/dav/made-via-dav.txt", strings.NewReader("put"))
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT code=%d body=%s", rec.Code, rec.Body.String())
	}
	if b, err := os.ReadFile(filepath.Join(s.root, "made-via-dav.txt")); err != nil || string(b) != "put" {
		t.Fatalf("put bytes=%q err=%v", b, err)
	}
}
