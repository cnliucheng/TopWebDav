package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"time"
)

// ListItem is one row in the directory listing response.
type ListItem struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type pathBody struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	From    string `json:"from"`
	To      string `json:"to"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	full, err := ResolveUnder(s.root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "read dir failed")
		return
	}
	rel, _ := SanitizePath(r.URL.Query().Get("path"))
	items := make([]ListItem, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := path.Join(rel, e.Name())
		if rel == "/" {
			p = "/" + e.Name()
		}
		items = append(items, ListItem{
			Name:    e.Name(),
			Path:    p,
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC(),
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	s.createEntry(w, r, true)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	s.createEntry(w, r, false)
}

func (s *Server) createEntry(w http.ResponseWriter, r *http.Request, dir bool) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body pathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	full, err := ResolveUnder(s.root, body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	name := path.Base(body.Path)
	if err := ValidName(name); err != nil {
		writeErr(w, http.StatusBadRequest, "bad name")
		return
	}
	if _, err := os.Stat(full); err == nil {
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	if dir {
		err = os.MkdirAll(full, 0o700)
	} else {
		err = os.WriteFile(full, []byte(body.Content), 0o600)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	full, err := ResolveUnder(s.root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	rootFull, err := ResolveUnder(s.root, "/")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "bad root")
		return
	}
	if full == rootFull {
		writeErr(w, http.StatusBadRequest, "refusing to delete root")
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if fi.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "read dir failed")
			return
		}
		if len(entries) > 0 {
			writeErr(w, http.StatusConflict, "directory not empty")
			return
		}
	}
	if err := os.Remove(full); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body pathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	src, err := ResolveUnder(s.root, body.From)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad from")
		return
	}
	dst, err := ResolveUnder(s.root, body.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad to")
		return
	}
	if err := ValidName(path.Base(body.To)); err != nil {
		writeErr(w, http.StatusBadRequest, "bad name")
		return
	}
	if _, err := os.Stat(src); err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if _, err := os.Stat(dst); err == nil {
		writeErr(w, http.StatusConflict, "target exists")
		return
	}
	if err := os.Rename(src, dst); err != nil {
		writeErr(w, http.StatusInternalServerError, "rename failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
