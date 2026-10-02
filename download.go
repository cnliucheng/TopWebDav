package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

const downloadTicketLifetime = 10 * time.Minute
const maxDownloadTickets = 1024

type downloadTicket struct {
	path    string
	hash    string
	expires time.Time
	total   int64
	sent    int64
	started bool
	active  bool
	done    bool
}

type downloadProgress struct {
	Total   int64 `json:"total"`
	Sent    int64 `json:"sent"`
	Started bool  `json:"started"`
	Done    bool  `json:"done"`
}

// progressWriter counts bytes accepted by the HTTP server. Reverse proxies
// may buffer them, so this is server transfer progress, not disk progress.
type progressWriter struct {
	http.ResponseWriter
	onWrite func(int)
}

func (w progressWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.onWrite(n)
	}
	return n, err
}

func (w progressWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// handleDownloadTicket grants a temporary URL for a single file. The URL
// lets the browser's download manager stream bytes directly to disk.
func (s *Server) handleDownloadTicket(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	clean, err := SanitizePath(body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	full, err := ResolveUnder(s.root, clean)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		writeErr(w, http.StatusInternalServerError, "ticket failed")
		return
	}
	token := hex.EncodeToString(secret)
	now := time.Now()
	s.mu.Lock()
	passwordHash := s.cfg.PasswordHash
	s.mu.Unlock()
	s.downloadMu.Lock()
	if s.downloads == nil {
		s.downloads = make(map[string]downloadTicket)
	}
	for key, ticket := range s.downloads {
		if !ticket.expires.After(now) && !ticket.active {
			delete(s.downloads, key)
		}
	}
	if len(s.downloads) >= maxDownloadTickets {
		oldestKey := ""
		var oldest time.Time
		for key, ticket := range s.downloads {
			if !ticket.active && (oldestKey == "" || ticket.expires.Before(oldest)) {
				oldestKey, oldest = key, ticket.expires
			}
		}
		if oldestKey == "" {
			s.downloadMu.Unlock()
			writeErr(w, http.StatusServiceUnavailable, "too many downloads")
			return
		}
		delete(s.downloads, oldestKey)
	}
	s.downloads[token] = downloadTicket{path: clean, hash: passwordHash, expires: now.Add(downloadTicketLifetime), total: info.Size()}
	s.downloadMu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"ticket": token})
}

func (s *Server) handleDownloadProgress(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("ticket")
	s.downloadMu.Lock()
	ticket, ok := s.downloads[token]
	s.downloadMu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "download not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, downloadProgress{
		Total: ticket.total, Sent: ticket.sent, Started: ticket.started, Done: ticket.done,
	})
}

func (s *Server) handleDownloadRoute(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("ticket")
	if token == "" {
		s.requireAuth(http.HandlerFunc(s.handleDownload)).ServeHTTP(w, r)
		return
	}
	s.mu.Lock()
	currentHash := s.cfg.PasswordHash
	s.mu.Unlock()
	s.downloadMu.Lock()
	ticket, ok := s.downloads[token]
	if !ok || !ticket.expires.After(time.Now()) || ticket.hash != currentHash {
		s.downloadMu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		writeErr(w, http.StatusUnauthorized, "invalid download ticket")
		return
	}
	if r.Method != http.MethodHead {
		// A follow-up Range request continues the same transfer. A fresh
		// request after a completed full download starts at zero again.
		if ticket.done && ticket.sent >= ticket.total {
			ticket.sent = 0
		}
		ticket.started, ticket.active, ticket.done = true, true, false
		s.downloads[token] = ticket
	}
	s.downloadMu.Unlock()
	// Resolve the path again at request time in case the file or a symlink
	// changed after the ticket was issued.
	if r.Method == http.MethodHead {
		s.serveDownload(w, r, ticket.path)
		return
	}
	defer func() {
		s.downloadMu.Lock()
		if state, exists := s.downloads[token]; exists {
			state.active, state.done = false, true
			s.downloads[token] = state
		}
		s.downloadMu.Unlock()
	}()
	s.serveDownload(progressWriter{ResponseWriter: w, onWrite: func(n int) {
		s.downloadMu.Lock()
		if state, exists := s.downloads[token]; exists {
			state.sent += int64(n)
			if state.sent > state.total {
				state.sent = state.total
			}
			s.downloads[token] = state
		}
		s.downloadMu.Unlock()
	}}, r, ticket.path)
}
