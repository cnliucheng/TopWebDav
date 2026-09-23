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
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("index unauth code=%d", rec.Code)
	}
}
