package main

import (
	"encoding/json"
	"mime/multipart"
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

func TestUploadDownloadReadWrite(t *testing.T) {
	s := testServer(t)

	body := &strings.Builder{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upload?path=/", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleUpload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload code=%d body=%s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(s.root, "note.txt")); string(b) != "hello" {
		t.Fatalf("uploaded bytes=%q", b)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/download?path=/note.txt", nil)
	rec = httptest.NewRecorder()
	s.handleDownload(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("download code=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/read?path=/note.txt", nil)
	rec = httptest.NewRecorder()
	s.handleRead(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read code=%d body=%s", rec.Code, rec.Body.String())
	}
	var readResp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &readResp); err != nil || readResp.Content != "hello" {
		t.Fatalf("readResp=%+v err=%v", readResp, err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/write", strings.NewReader(`{"path":"/note.txt","content":"hello2"}`))
	rec = httptest.NewRecorder()
	s.handleWrite(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("write code=%d body=%s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(s.root, "note.txt")); string(b) != "hello2" {
		t.Fatalf("after write=%q", b)
	}

	_ = os.WriteFile(filepath.Join(s.root, "blob.bin"), []byte{0x00, 0x01, 0xff}, 0o600)
	req = httptest.NewRequest(http.MethodGet, "/api/read?path=/blob.bin", nil)
	rec = httptest.NewRecorder()
	s.handleRead(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("nontext read code=%d want 415", rec.Code)
	}
}
