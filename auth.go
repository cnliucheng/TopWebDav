package main

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

const (
	authFailThreshold  = 5                // failures before cooldowns start
	authFailMaxTracked = 4096             // IPs kept before an expired-entry sweep
	authFailCooldown   = 30 * time.Second // first cooldown; doubles per extra failure
	authFailMaxWait    = 15 * time.Minute
)

// authFails is one IP's failed-login state. until is zero while under the
// threshold, then a cooldown deadline that doubles with each extra failure.
type authFails struct {
	n     int
	until time.Time
}

// authLimiter throttles repeated failed Basic-Auth attempts per client IP to
// cap online password guessing. It counts RemoteAddr only: behind a reverse
// proxy every client shares the proxy IP, so the throttle is effectively
// global there — which still stops brute force, at the cost of also cooling
// down the real user while an attacker is trying.
type authLimiter struct {
	mu    sync.Mutex
	fails map[string]*authFails
	now   func() time.Time // swap in tests
}

func newAuthLimiter() *authLimiter {
	return &authLimiter{fails: make(map[string]*authFails), now: time.Now}
}

func (l *authLimiter) locked(ip string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.fails[ip]
	if !ok {
		return 0, false
	}
	if d := f.until.Sub(l.now()); d > 0 {
		return d, true
	}
	return 0, false
}

func (l *authLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, seen := l.fails[ip]; !seen && len(l.fails) >= authFailMaxTracked {
		now := l.now()
		for k, v := range l.fails {
			if !v.until.After(now) {
				delete(l.fails, k)
			}
		}
	}
	f := l.fails[ip]
	if f == nil {
		f = &authFails{}
		l.fails[ip] = f
	}
	f.n++
	if f.n < authFailThreshold {
		return
	}
	shift := uint(f.n - authFailThreshold)
	if shift > 5 {
		shift = 5
	}
	cool := authFailCooldown << shift
	if cool > authFailMaxWait {
		cool = authFailMaxWait
	}
	f.until = l.now().Add(cool)
}

func (l *authLimiter) success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}

// authIP reduces RemoteAddr to its host part.
func authIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requireAuth wraps h with HTTP Basic Auth against the configured user.
// /api/* does not send WWW-Authenticate so browsers never show the native
// Basic dialog — the web UI uses its own login page instead.
// Repeated failures from one IP are first throttled (429) and then still
// rejected after the cooldown, so a cooldown never admits a stale attempt.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.limiterOnce.Do(func() { s.limiter = newAuthLimiter() })
		ip := authIP(r)
		if d, ok := s.limiter.locked(ip); ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "too many failed attempts, retry later")
			return
		}
		user, pass, ok := r.BasicAuth()
		s.mu.Lock()
		u := s.cfg.Username
		h := s.cfg.PasswordHash
		s.mu.Unlock()
		if !ok || user != u || !CheckPassword(h, pass) {
			s.limiter.fail(ip)
			p := r.URL.Path
			if p != "/api" && !strings.HasPrefix(p, "/api/") {
				w.Header().Set("WWW-Authenticate", authRealm)
			}
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		s.limiter.success(ip)
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
	prev := s.cfg.PasswordHash
	s.cfg.PasswordHash = hash
	if err := s.cfg.Save(s.cfgPath); err != nil {
		s.cfg.PasswordHash = prev
		writeErr(w, http.StatusInternalServerError, "save config failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
