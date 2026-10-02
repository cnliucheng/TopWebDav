package main

import (
	"bytes"
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

func TestRESTErrorContract(t *testing.T) {
	s := testServer(t)

	// create existing → 409
	req := httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/dup.txt","content":"a"}`))
	rec := httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/dup.txt","content":"b"}`))
	rec = httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("create existing code=%d want 409", rec.Code)
	}

	// delete missing → 404
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/nope.txt", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing code=%d want 404", rec.Code)
	}

	// delete root → 400
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("delete root code=%d want 400", rec.Code)
	}

	// rename missing → 404
	req = httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(`{"from":"/ghost.txt","to":"/ghost2.txt"}`))
	rec = httptest.NewRecorder()
	s.handleRename(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("rename missing code=%d want 404", rec.Code)
	}

	// rename onto existing → 409
	req = httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/tgt.txt","content":"t"}`))
	rec = httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create tgt code=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(`{"from":"/dup.txt","to":"/tgt.txt"}`))
	rec = httptest.NewRecorder()
	s.handleRename(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rename onto existing code=%d want 409", rec.Code)
	}

	// read >1MiB file → 413
	big := bytes.Repeat([]byte("a"), maxEditBytes+10)
	if err := os.WriteFile(filepath.Join(s.root, "big.txt"), big, 0o600); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/read?path=/big.txt", nil)
	rec = httptest.NewRecorder()
	s.handleRead(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("read big code=%d want 413", rec.Code)
	}

	// write >1MiB content → 413
	payload := `{"path":"/note.txt","content":"` + strings.Repeat("a", maxEditBytes+10) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/api/write", strings.NewReader(payload))
	rec = httptest.NewRecorder()
	s.handleWrite(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("write big code=%d want 413", rec.Code)
	}

	// ValidName reject: create path with quote in name → 400
	req = httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/bad\"name.txt","content":"x"}`))
	rec = httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create quote name code=%d want 400", rec.Code)
	}
}

func uploadRequest(t *testing.T, url string, filename string, payload []byte) (*http.Request, *multipart.Writer) {
	t.Helper()
	body := &strings.Builder{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req, mw
}

func TestSymlinkListAndDelete(t *testing.T) {
	s := testServer(t)

	// A directory plus a symlink pointing at it inside the root.
	if err := os.Mkdir(filepath.Join(s.root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "docs", "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(s.root, "docs"), filepath.Join(s.root, "docslink")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "note.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(s.root, "note.txt"), filepath.Join(s.root, "notelink.txt")); err != nil {
		t.Fatal(err)
	}

	// Listing resolves symlink targets: the folder link must read as a dir
	// (it used to show up as a file and 404 when opened) and the file link
	// must report the target's size, not the link's.
	req := httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil)
	rec := httptest.NewRecorder()
	s.handleList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code=%d body=%s", rec.Code, rec.Body.String())
	}
	var items []ListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	byName := map[string]ListItem{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if it, ok := byName["docslink"]; !ok || !it.IsDir {
		t.Fatalf("docslink = %+v, want IsDir", it)
	}
	if it, ok := byName["notelink.txt"]; !ok || it.IsDir || it.Size != 2 {
		t.Fatalf("notelink.txt = %+v, want file size 2", it)
	}

	// Deleting the link removes the link only — a non-empty target used to
	// make this return 409, even though the link itself is what gets deleted.
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/docslink", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete symlink code=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Lstat(filepath.Join(s.root, "docslink")); !os.IsNotExist(err) {
		t.Fatalf("symlink still present, err=%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(s.root, "docs", "keep.txt")); err != nil || string(b) != "keep" {
		t.Fatalf("target disturbed: %q err=%v", b, err)
	}

	// Renaming a symlink moves the link; the target stays put.
	req = httptest.NewRequest(http.MethodPost, "/api/rename",
		strings.NewReader(`{"from":"/notelink.txt","to":"/renamed-link.txt"}`))
	rec = httptest.NewRecorder()
	s.handleRename(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename symlink code=%d body=%s", rec.Code, rec.Body.String())
	}
	fi, err := os.Lstat(filepath.Join(s.root, "renamed-link.txt"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("renamed entry should be the symlink, err=%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(s.root, "note.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("target after rename: %q err=%v", b, err)
	}
}

func TestUploadSizeLimit(t *testing.T) {
	s := testServer(t)
	oneMB := 1
	s.mu.Lock()
	s.cfg.MaxUploadMB = &oneMB
	s.mu.Unlock()

	// A body over the configured cap is a clean 413, not a generic 400.
	oversize := bytes.Repeat([]byte("a"), (1<<20)+4096)
	req, _ := uploadRequest(t, "/api/upload?path=/", "big.bin", oversize)
	rec := httptest.NewRecorder()
	s.handleUpload(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize upload code=%d body=%s want 413", rec.Code, rec.Body.String())
	}

	// Explicit 0 means unlimited: the same size would be accepted.
	s.mu.Lock()
	zero := 0
	s.cfg.MaxUploadMB = &zero
	s.mu.Unlock()
	req, _ = uploadRequest(t, "/api/upload?path=/", "big2.bin", oversize)
	rec = httptest.NewRecorder()
	s.handleUpload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unlimited upload code=%d body=%s want 200", rec.Code, rec.Body.String())
	}
	if fi, err := os.Stat(filepath.Join(s.root, "big2.bin")); err != nil || fi.Size() != int64(len(oversize)) {
		t.Fatalf("unlimited upload not written: size=%v err=%v", fi, err)
	}
}

func TestDavPutSizeLimit(t *testing.T) {
	s := testServer(t)
	oneMB := 1
	s.mu.Lock()
	s.cfg.MaxUploadMB = &oneMB
	s.mu.Unlock()
	mux := s.routes()

	put := func(payload []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/dav/big.bin", bytes.NewReader(payload))
		req.SetBasicAuth("admin", "admin")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	oversize := bytes.Repeat([]byte("a"), (1<<20)+4096)
	if rec := put(oversize); rec.Code == http.StatusCreated || rec.Code == http.StatusNoContent {
		t.Fatalf("oversize PUT code=%d want a failure status", rec.Code)
	}
	if rec := put([]byte("put")); rec.Code != http.StatusCreated {
		t.Fatalf("small PUT code=%d body=%s want 201", rec.Code, rec.Body.String())
	}
}
