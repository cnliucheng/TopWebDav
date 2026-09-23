package main

import (
	"encoding/json"
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
