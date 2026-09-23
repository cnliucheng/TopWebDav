package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"sync"
)

// Server holds process-wide state for HTTP handlers.
type Server struct {
	cfg     *Config
	cfgPath string
	root    string
	mu      sync.Mutex
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// routes builds the full mux: API, WebDAV, static UI — all behind Basic Auth.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/list", s.handleList)
	mux.HandleFunc("POST /api/mkdir", s.handleMkdir)
	mux.HandleFunc("POST /api/create", s.handleCreate)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/download", s.handleDownload)
	mux.HandleFunc("GET /api/read", s.handleRead)
	mux.HandleFunc("POST /api/write", s.handleWrite)
	mux.HandleFunc("DELETE /api/delete", s.handleDelete)
	mux.HandleFunc("POST /api/rename", s.handleRename)
	mux.HandleFunc("POST /api/password", s.handlePassword)
	// Unmatched /api/* must stay JSON — FileServer returns text/plain 404.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "api not found")
	})
	mux.Handle("/dav/", s.davHandler())
	mux.Handle("/", http.FileServer(http.FS(mustWebFS())))
	return s.requireAuth(stripAPITrailingSlash(mux))
}

// stripAPITrailingSlash turns /api/list/ into /api/list so proxies and
// users that add a slash still hit the handler instead of FileServer 404.
func stripAPITrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") && strings.HasSuffix(p, "/") && p != "/api/" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimRight(p, "/")
			next.ServeHTTP(w, r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mustWebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
