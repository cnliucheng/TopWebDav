package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	authVerifyInterval = time.Second // bcrypt attempts allowed per cooling-down IP
)

// authFails is one IP's failed-login state. until is zero while under the
// threshold, then a cooldown deadline that doubles with each extra failure.
// lastVerify records when a cooling-down IP last spent its one-per-interval
// password verification.
type authFails struct {
	n          int
	until      time.Time
	lastVerify time.Time
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
		if len(l.fails) >= authFailMaxTracked {
			// Every remaining entry is still cooling down (an attacker using
			// many source IPs could fill the table with them). Evict one
			// rather than grow without bound — evicting a cooling entry only
			// lets that IP start over from zero failures.
			for k := range l.fails {
				delete(l.fails, k)
				break
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

// verifyGate grants a cooling-down IP one password verification per
// authVerifyInterval. Cooling-down requests may not be answered before the
// password is checked (the real user must be able to get in), but that check
// is deliberately slow, so without this gate an attacker could keep one
// bcrypt running per request for free. Returns how long to wait and whether
// verification is allowed now.
func (l *authLimiter) verifyGate(ip string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.fails[ip]
	if f == nil {
		return 0, true
	}
	elapsed := l.now().Sub(f.lastVerify)
	if elapsed >= authVerifyInterval {
		f.lastVerify = l.now()
		return 0, true
	}
	return authVerifyInterval - elapsed, false
}

// passCache memoizes accepted credentials so authenticated requests skip the
// deliberately slow bcrypt comparison. Only successful verdicts are stored —
// a miss always falls back to bcrypt, so guessing never gets cheaper. Entries
// are keyed by the bcrypt hash, which invalidates them on password change,
// and the password itself is kept as SHA-256, never plaintext.
type passCache struct {
	mu   sync.Mutex
	user string
	hash string
	sum  [sha256.Size]byte
}

// credentialsOK reports whether user/pass is the configured account.
func (s *Server) credentialsOK(user, pass string) bool {
	s.mu.Lock()
	u, h := s.cfg.Username, s.cfg.PasswordHash
	s.mu.Unlock()
	if user != u {
		return false
	}
	sum := sha256.Sum256([]byte(pass))
	s.pass.mu.Lock()
	hit := s.pass.hash == h && s.pass.user == u && s.pass.sum == sum
	s.pass.mu.Unlock()
	if hit {
		return true
	}
	if !CheckPassword(h, pass) {
		return false
	}
	s.pass.mu.Lock()
	s.pass.user, s.pass.hash, s.pass.sum = u, h, sum
	s.pass.mu.Unlock()
	return true
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
// Repeated failures from one IP are first throttled (429); while the cooldown
// lasts, wrong passwords and credential-less requests stay on 429 but a
// correct password still gets through (see serveCooling), so an attacker who
// can trigger failures can never lock the real user out.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.limiterOnce.Do(func() { s.limiter = newAuthLimiter() })
		ip := authIP(r)
		if _, cooling := s.limiter.locked(ip); cooling {
			s.serveCooling(w, r, ip, next)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !s.credentialsOK(user, pass) {
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

// serveCooling answers a request from an IP in failed-login cooldown. A
// correct password is verified and admitted — that is what keeps a lockout
// from becoming a denial of service against the real user — but verification
// runs at most authVerifyInterval per IP so hammering the endpoint stays
// cheap for the server. Requests without credentials, and wrong passwords,
// get 429; a wrong password also renews the cooldown.
func (s *Server) serveCooling(w http.ResponseWriter, r *http.Request, ip string, next http.Handler) {
	wait := s.coolingWait(ip)
	if user, pass, ok := r.BasicAuth(); ok {
		if left, allowed := s.limiter.verifyGate(ip); allowed {
			if s.credentialsOK(user, pass) {
				s.limiter.success(ip)
				next.ServeHTTP(w, r)
				return
			}
			s.limiter.fail(ip)
			wait = s.coolingWait(ip)
		} else {
			wait = left
		}
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	writeErr(w, http.StatusTooManyRequests, "too many failed attempts, retry later")
}

func (s *Server) coolingWait(ip string) time.Duration {
	d, _ := s.limiter.locked(ip)
	return d
}

// handlePassword changes the single account password.
// POST /api/password  {"old_password","new_password"}
func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10) // passwords are tiny
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request too large")
			return
		}
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
