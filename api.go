package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
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

const maxEditBytes = 1 << 20 // 1 MiB

func isTextish(name string, sample []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt", ".md", ".json", ".yaml", ".yml", ".toml", ".conf", ".cfg",
		".ini", ".log", ".csv", ".xml", ".html", ".css", ".js", ".go", ".sh",
		".env", ".sql":
		return true
	}
	if mt := mime.TypeByExtension(ext); strings.HasPrefix(mt, "text/") {
		return true
	}
	return len(sample) > 0 && !bytes.ContainsRune(sample, 0)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	dirRel := r.URL.Query().Get("path")
	if dirRel == "" {
		dirRel = "/"
	}
	dirFull, err := ResolveUnder(s.root, dirRel)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "bad multipart")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	if err := ValidName(name); err != nil {
		writeErr(w, http.StatusBadRequest, "bad name")
		return
	}
	dst := filepath.Join(dirFull, name)
	if _, err := os.Stat(dst); err == nil {
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	defer out.Close()
	_ = out.Chmod(0o600)
	if _, err := io.Copy(out, file); err != nil {
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	full, err := ResolveUnder(s.root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	fi, err := os.Stat(full)
	if err != nil || fi.IsDir() {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+fi.Name()+`"`)
	http.ServeFile(w, r, full)
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	full, err := ResolveUnder(s.root, r.URL.Query().Get("path"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if fi.IsDir() {
		writeErr(w, http.StatusBadRequest, "is a directory")
		return
	}
	if fi.Size() > maxEditBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "file too large to edit")
		return
	}
	sample, err := os.ReadFile(full)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "read failed")
		return
	}
	if !isTextish(filepath.Base(full), sample) {
		writeErr(w, http.StatusUnsupportedMediaType, "not a text file")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"content": string(sample)})
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if len(body.Content) > maxEditBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "content too large")
		return
	}
	full, err := ResolveUnder(s.root, body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	if !isTextish(filepath.Base(full), []byte(body.Content)) {
		writeErr(w, http.StatusUnsupportedMediaType, "not a text file")
		return
	}
	if err := os.WriteFile(full, []byte(body.Content), 0o600); err != nil {
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
