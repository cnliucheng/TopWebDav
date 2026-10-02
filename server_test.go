package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPINotFoundIsJSON(t *testing.T) {
	s := testServer(t)
	mux := s.routes()
	req := httptest.NewRequest(http.MethodGet, "/api/nope", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type=%q want json", ct)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestAPITrailingSlashList(t *testing.T) {
	s := testServer(t)
	mux := s.routes()
	req := httptest.NewRequest(http.MethodGet, "/api/list/?path=/", nil)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoutesAuth(t *testing.T) {
	s := testServer(t)
	mux := s.routes()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("list unauth code=%d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("API must not send WWW-Authenticate (avoid browser basic dialog)")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil)
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list auth code=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("PROPFIND", "/dav/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("dav unauth code=%d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("WebDAV must send WWW-Authenticate")
	}
	// Static UI is public (custom login page).
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("index code=%d want 200", rec.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := testServer(t)
	mux := s.routes()

	check := func(rec *httptest.ResponseRecorder, label string) {
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: missing nosniff", label)
		}
		if h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s: missing X-Frame-Options", label)
		}
		if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatalf("%s: CSP=%q", label, h.Get("Content-Security-Policy"))
		}
		if h.Get("Referrer-Policy") == "" {
			t.Fatalf("%s: missing Referrer-Policy", label)
		}
	}

	// Headers apply to static UI, authenticated API errors and WebDAV alike.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	check(rec, "static")

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil))
	check(rec, "api 401")

	req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	check(rec, "dav")
}
