package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListMkdirCreateDeleteRename(t *testing.T) {
	s := testServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(`{"path":"/docs"}`))
	rec := httptest.NewRecorder()
	s.handleMkdir(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir code=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/docs/a.txt","content":"hi"}`))
	rec = httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rec.Code, rec.Body.String())
	}
	b, err := os.ReadFile(filepath.Join(s.root, "docs", "a.txt"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("file content=%q err=%v", b, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/list?path=/docs", nil)
	rec = httptest.NewRecorder()
	s.handleList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code=%d", rec.Code)
	}
	var items []ListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "a.txt" || items[0].IsDir {
		t.Fatalf("items=%+v", items)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(`{"from":"/docs/a.txt","to":"/docs/b.txt"}`))
	rec = httptest.NewRecorder()
	s.handleRename(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename code=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.root, "docs", "b.txt")); err != nil {
		t.Fatal("rename target missing")
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/docs/b.txt", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code=%d", rec.Code)
	}

	_ = os.WriteFile(filepath.Join(s.root, "docs", "x"), []byte("x"), 0o600)
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/docs", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("non-empty delete code=%d want 409", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/list?path=/../etc", nil)
	rec = httptest.NewRecorder()
	s.handleList(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("escape code=%d want 400", rec.Code)
	}
}
