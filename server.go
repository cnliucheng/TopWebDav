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

// routes builds the mux. Static UI is public (custom login page in the app);
// /api/* and /dav/* require Basic Auth (WebDAV sends WWW-Authenticate).
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	api := http.NewServeMux()
	api.HandleFunc("GET /api/list", s.handleList)
	api.HandleFunc("POST /api/mkdir", s.handleMkdir)
	api.HandleFunc("POST /api/create", s.handleCreate)
	api.HandleFunc("POST /api/upload", s.handleUpload)
	api.HandleFunc("GET /api/download", s.handleDownload)
	api.HandleFunc("GET /api/read", s.handleRead)
	api.HandleFunc("POST /api/write", s.handleWrite)
	api.HandleFunc("DELETE /api/delete", s.handleDelete)
	api.HandleFunc("POST /api/rename", s.handleRename)
	api.HandleFunc("POST /api/password", s.handlePassword)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "api not found")
	})
	mux.Handle("/api/", s.requireAuth(stripAPITrailingSlash(api)))
	mux.Handle("/dav/", s.requireAuth(s.davHandler()))
	mux.Handle("/", http.FileServer(http.FS(mustWebFS())))
	return mux
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
