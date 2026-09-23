package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
