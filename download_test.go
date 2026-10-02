package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadTicket(t *testing.T) {
	s := testServer(t)
	if err := os.WriteFile(filepath.Join(s.root, "large.txt"), []byte("download contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	mux := s.routes()

	request := func(method, target, body string, auth bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if auth {
			req.SetBasicAuth("admin", "admin")
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := request(http.MethodPost, "/api/download-ticket", `{"path":"/large.txt"}`, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ticket code=%d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/download-progress?ticket=unknown", "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated progress code=%d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/download?path=/large.txt", "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated download code=%d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/download?ticket=invalid", "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid ticket code=%d", rec.Code)
	}
	if rec := request(http.MethodGet, "/api/download/?path=/large.txt", "", true); rec.Code != http.StatusOK {
		t.Fatalf("trailing slash download code=%d", rec.Code)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if rec := request(http.MethodPost, "/api/download-ticket", `{"path":"/escape.txt"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("escaping symlink ticket code=%d", rec.Code)
	}
	rec := request(http.MethodPost, "/api/download-ticket", `{"path":"/large.txt"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket code=%d body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || len(result.Ticket) != 64 {
		t.Fatalf("ticket=%q err=%v", result.Ticket, err)
	}
	progressURL := "/api/download-progress?ticket=" + result.Ticket
	checkProgress := func(wantSent int64, wantDone bool) {
		t.Helper()
		progressRec := request(http.MethodGet, progressURL, "", true)
		if progressRec.Code != http.StatusOK {
			t.Fatalf("progress code=%d body=%s", progressRec.Code, progressRec.Body.String())
		}
		var progress downloadProgress
		if err := json.Unmarshal(progressRec.Body.Bytes(), &progress); err != nil {
			t.Fatal(err)
		}
		if progress.Total != 17 || progress.Sent != wantSent || progress.Done != wantDone {
			t.Fatalf("progress=%+v want sent=%d done=%v", progress, wantSent, wantDone)
		}
	}
	checkProgress(0, false)
	rec = request(http.MethodHead, "/api/download?path=/large.txt", "", true)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Length") != "17" || rec.Body.Len() != 0 {
		t.Fatalf("head code=%d len=%q body=%q", rec.Code, rec.Header().Get("Content-Length"), rec.Body.String())
	}
	rec = request(http.MethodGet, "/api/download?ticket="+result.Ticket, "", false)
	if rec.Code != http.StatusOK || rec.Body.String() != "download contents" {
		t.Fatalf("ticket download code=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cache control=%q", rec.Header().Get("Cache-Control"))
	}
	checkProgress(17, true)
	rangeReq := httptest.NewRequest(http.MethodGet, "/api/download?ticket="+result.Ticket, nil)
	rangeReq.Header.Set("Range", "bytes=0-7")
	rangeRec := httptest.NewRecorder()
	mux.ServeHTTP(rangeRec, rangeReq)
	if rangeRec.Code != http.StatusPartialContent || rangeRec.Body.String() != "download" {
		t.Fatalf("range code=%d body=%q", rangeRec.Code, rangeRec.Body.String())
	}
	checkProgress(8, true)
	s.downloadMu.Lock()
	expired := s.downloads[result.Ticket]
	expired.expires = time.Now().Add(-time.Second)
	s.downloads[result.Ticket] = expired
	s.downloadMu.Unlock()
	if rec := request(http.MethodGet, "/api/download?ticket="+result.Ticket, "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired ticket code=%d", rec.Code)
	}
	// Restore the expiry to verify password changes invalidate a live ticket.
	s.downloadMu.Lock()
	expired.expires = time.Now().Add(time.Minute)
	s.downloads[result.Ticket] = expired
	s.downloadMu.Unlock()
	s.mu.Lock()
	s.cfg.PasswordHash = "changed"
	s.mu.Unlock()
	if rec := request(http.MethodGet, "/api/download?ticket="+result.Ticket, "", false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("ticket after password change code=%d", rec.Code)
	}
}

type gatedDownloadWriter struct {
	*httptest.ResponseRecorder
	writes  int
	blocked chan struct{}
	release chan struct{}
}

func (w *gatedDownloadWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == 2 {
		close(w.blocked)
		<-w.release
	}
	return w.ResponseRecorder.Write(p)
}

func TestDownloadProgressDuringTransfer(t *testing.T) {
	s := testServer(t)
	content := bytes.Repeat([]byte("x"), 256<<10)
	if err := os.WriteFile(filepath.Join(s.root, "large.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	mux := s.routes()
	issue := httptest.NewRequest(http.MethodPost, "/api/download-ticket", strings.NewReader(`{"path":"/large.bin"}`))
	issue.SetBasicAuth("admin", "admin")
	issued := httptest.NewRecorder()
	mux.ServeHTTP(issued, issue)
	if issued.Code != http.StatusOK {
		t.Fatalf("ticket code=%d body=%s", issued.Code, issued.Body.String())
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(issued.Body.Bytes(), &ticket); err != nil {
		t.Fatal(err)
	}
	w := &gatedDownloadWriter{
		ResponseRecorder: httptest.NewRecorder(),
		blocked:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/download?ticket="+ticket.Ticket, nil))
	}()
	select {
	case <-w.blocked:
	case <-time.After(5 * time.Second):
		close(w.release)
		t.Fatal("download did not stream multiple writes")
	}
	progressReq := httptest.NewRequest(http.MethodGet, "/api/download-progress?ticket="+ticket.Ticket, nil)
	progressReq.SetBasicAuth("admin", "admin")
	progressRec := httptest.NewRecorder()
	mux.ServeHTTP(progressRec, progressReq)
	var progress downloadProgress
	if err := json.Unmarshal(progressRec.Body.Bytes(), &progress); err != nil {
		close(w.release)
		t.Fatal(err)
	}
	if !progress.Started || progress.Done || progress.Sent <= 0 || progress.Sent >= progress.Total {
		close(w.release)
		t.Fatalf("in-flight progress=%+v", progress)
	}
	close(w.release)
	<-finished
	if w.Code != http.StatusOK || w.Body.Len() != len(content) {
		t.Fatalf("download code=%d size=%d", w.Code, w.Body.Len())
	}
}
