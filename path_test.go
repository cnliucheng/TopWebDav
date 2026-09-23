package main

import (
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
	rel, err := filepath.Rel(root, p)
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
}
