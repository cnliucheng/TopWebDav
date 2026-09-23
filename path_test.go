package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/", "/", true},
		{"", "/", true},
		{"/a/b", "/a/b", true},
		{"/a//b/", "/a/b", true},
		{"/a/./b", "/a/b", true},
		{"/../etc/passwd", "", false},
		{"/a/../../b", "", false},
		{"a/b", "/a/b", true},
		{"/a/\x00b", "", false},
	}
	for _, tc := range cases {
		got, err := SanitizePath(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("SanitizePath(%q) err=%v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("SanitizePath(%q)=%q want %q", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Fatalf("SanitizePath(%q) should fail, got %q", tc.in, got)
		}
	}
}

func TestResolveUnder(t *testing.T) {
	root := t.TempDir()
	p, err := ResolveUnder(root, "/ok/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty resolve")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(realRoot, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("resolved outside root: %q (rel=%q err=%v)", p, rel, err)
	}
	if _, err := ResolveUnder(root, "/../escape"); err == nil {
		t.Fatal("escape should fail")
	}
	// Filesystem root must still accept children (HasPrefix(root+sep) would fail here).
	if _, err := ResolveUnder(string(filepath.Separator), "/etc/passwd"); err != nil {
		t.Fatalf("ResolveUnder(/): %v", err)
	}
	// A symlink under root that points outside must not allow escape.
	if err := os.Symlink("/etc", filepath.Join(root, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := ResolveUnder(root, "/link/passwd"); err == nil {
		t.Fatal("symlink escape should fail")
	}
}

func TestValidName(t *testing.T) {
	if err := ValidName("file.txt"); err != nil {
		t.Fatal(err)
	}
	if err := ValidName(""); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := ValidName(".."); err == nil {
		t.Fatal("dotdot should fail")
	}
	if err := ValidName("a/b"); err == nil {
		t.Fatal("slash should fail")
	}
	if err := ValidName("a\x00b"); err == nil {
		t.Fatal("null should fail")
	}
	if err := ValidName(`a"b`); err == nil {
		t.Fatal("double quote should fail")
	}
	if err := ValidName(`a;b`); err == nil {
		t.Fatal("semicolon should fail")
	}
	if err := ValidName("a\nb"); err == nil {
		t.Fatal("newline should fail")
	}
	if err := ValidName("a\x7fb"); err == nil {
		t.Fatal("DEL should fail")
	}
}
