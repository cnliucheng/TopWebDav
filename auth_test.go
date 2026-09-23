package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: cfg, cfgPath: cfgPath, root: cfg.DataDir, mu: sync.Mutex{}}
}

func TestCheckPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") || CheckPassword(h, "nope") {
		t.Fatal("CheckPassword mismatch")
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	s := testServer(t)
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("missing WWW-Authenticate")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d want 204", rec.Code)
	}
}

func TestChangePassword(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader(`{"old_password":"admin","new_password":"newpass1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/password", body)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	// Old password must fail Basic Auth now.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.SetBasicAuth("admin", "admin")
	rec2 := httptest.NewRecorder()
	s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatal("old password should fail")
	}
	// Wrong old password rejected without changing hash.
	req3 := httptest.NewRequest(http.MethodPost, "/api/password",
		strings.NewReader(`{"old_password":"admin","new_password":"zzzzzzzz"}`))
	req3.SetBasicAuth("admin", "newpass1")
	rec3 := httptest.NewRecorder()
	s.handlePassword(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec3.Code)
	}
	b, _ := os.ReadFile(s.cfgPath)
	if strings.Contains(string(b), "zzzzzzzz") {
		t.Fatal("plaintext password must not be stored")
	}
}

func TestChangePasswordTooLong(t *testing.T) {
	s := testServer(t)
	long := strings.Repeat("x", 73)
	body := strings.NewReader(`{"old_password":"admin","new_password":"` + long + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/password", body)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d want 400", rec.Code)
	}
}
