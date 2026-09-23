package main

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether pw matches the bcrypt hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

const authRealm = `Basic realm="topwebdav"`

// requireAuth wraps h with HTTP Basic Auth against the configured user.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		s.mu.Lock()
		u := s.cfg.Username
		h := s.cfg.PasswordHash
		s.mu.Unlock()
		if !ok || user != u || !CheckPassword(h, pass) {
			w.Header().Set("WWW-Authenticate", authRealm)
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handlePassword changes the single account password.
// POST /api/password  {"old_password","new_password"}
func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if len(body.NewPassword) < 4 {
		writeErr(w, http.StatusBadRequest, "new password too short")
		return
	}
	if len(body.NewPassword) > 72 {
		writeErr(w, http.StatusBadRequest, "new password too long")
		return
	}
	user, pass, ok := r.BasicAuth()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok || user != s.cfg.Username || !CheckPassword(s.cfg.PasswordHash, pass) || pass != body.OldPassword {
		writeErr(w, http.StatusUnauthorized, "old password incorrect")
		return
	}
	hash, err := HashPassword(body.NewPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "hash failed")
		return
	}
	s.cfg.PasswordHash = hash
	if err := s.cfg.Save(s.cfgPath); err != nil {
		writeErr(w, http.StatusInternalServerError, "save config failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
