package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
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
	mux.Handle("/dav/", s.davHandler())
	mux.Handle("/", http.FileServer(http.FS(mustWebFS())))
	return s.requireAuth(mux)
}

func mustWebFS() fs.FS {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
