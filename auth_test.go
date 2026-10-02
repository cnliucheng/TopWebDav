package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	return &Server{cfg: cfg, cfgPath: cfgPath, root: cfg.DataDir, mu: sync.Mutex{}}
}

func TestCheckPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(h, "s3cret") || CheckPassword(h, "nope") {
		t.Fatal("CheckPassword mismatch")
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	s := testServer(t)
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("missing WWW-Authenticate")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/list", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("api code=%d want 401", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") != "" {
		t.Fatal("api must not challenge browser basic auth")
	}

	req = httptest.NewRequest(http.MethodGet, "/dav/", nil)
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d want 204", rec.Code)
	}
}

func TestChangePassword(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader(`{"old_password":"admin","new_password":"newpass1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/password", body)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	// Old password must fail Basic Auth now.
	req2 := httptest.NewRequest(http.MethodGet, "/dav/", nil)
	req2.SetBasicAuth("admin", "admin")
	rec2 := httptest.NewRecorder()
	s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatal("old password should fail")
	}
	// Wrong old password rejected without changing hash.
	req3 := httptest.NewRequest(http.MethodPost, "/api/password",
		strings.NewReader(`{"old_password":"admin","new_password":"zzzzzzzz"}`))
	req3.SetBasicAuth("admin", "newpass1")
	rec3 := httptest.NewRecorder()
	s.handlePassword(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec3.Code)
	}
	b, _ := os.ReadFile(s.cfgPath)
	if strings.Contains(string(b), "zzzzzzzz") {
		t.Fatal("plaintext password must not be stored")
	}
}

func TestChangePasswordTooLong(t *testing.T) {
	s := testServer(t)
	long := strings.Repeat("x", 73)
	body := strings.NewReader(`{"old_password":"admin","new_password":"` + long + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/password", body)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d want 400", rec.Code)
	}
}

func TestPasswordBodyLimit(t *testing.T) {
	s := testServer(t)
	// The endpoint's body cap is tiny; an oversized one is a clean 413.
	huge := `{"old_password":"` + strings.Repeat("x", 16<<10) + `","new_password":"abcd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/password", strings.NewReader(huge))
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize body code=%d want 413", rec.Code)
	}
}

func TestAuthLimiterBounded(t *testing.T) {
	l := newAuthLimiter()
	// Fill the table with IPs that are all mid-cooldown (nothing the expired
	// sweep can reclaim), then keep failing from fresh IPs: the table must
	// not grow past its cap no matter how many source addresses appear.
	for i := 0; i < authFailMaxTracked; i++ {
		ip := fmt.Sprintf("10.1.%d.%d", i/256, i%256)
		for j := 0; j < authFailThreshold; j++ {
			l.fail(ip)
		}
	}
	for i := 0; i < 100; i++ {
		l.fail(fmt.Sprintf("10.9.%d.%d", i/256, i%256))
	}
	l.mu.Lock()
	n := len(l.fails)
	l.mu.Unlock()
	if n > authFailMaxTracked {
		t.Fatalf("tracked IPs = %d, want <= %d", n, authFailMaxTracked)
	}
}

func TestAuthRateLimit(t *testing.T) {
	s := testServer(t)
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	attempt := func(user, pass string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
		req.SetBasicAuth(user, pass)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Failures under the threshold stay at 401 and don't lock the good login.
	for i := 0; i < authFailThreshold-1; i++ {
		if c := attempt("admin", "wrong").Code; c != http.StatusUnauthorized {
			t.Fatalf("pre-threshold fail %d: code=%d want 401", i, c)
		}
	}
	if c := attempt("admin", "admin").Code; c != http.StatusNoContent {
		t.Fatalf("pre-threshold good login code=%d want 204", c)
	}

	// The 5th consecutive failure starts the cooldown; further attempts get 429.
	for i := 0; i < authFailThreshold; i++ {
		if c := attempt("admin", "wrong").Code; c != http.StatusUnauthorized {
			t.Fatalf("failure %d: code=%d want 401", i+1, c)
		}
	}
	rec := attempt("admin", "wrong")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked wrong login code=%d want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("locked response must carry Retry-After")
	}

	// The wrong attempt above consumed this IP's verification window, so an
	// immediate retry is throttled (429) rather than spending bcrypt...
	rec = attempt("admin", "admin")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled good login code=%d want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("throttled response must carry Retry-After")
	}
	// ...but once the window reopens a correct password must get through the
	// lockout: a cooldown may delay the real user, never lock them out.
	s.limiter.now = func() time.Time { return time.Now().Add(authVerifyInterval + time.Second) }
	if c := attempt("admin", "admin").Code; c != http.StatusNoContent {
		t.Fatalf("cooling good login code=%d want 204", c)
	}

	// The success above cleared the counter; a fresh round behaves the same.
	s.limiter.now = func() time.Time { return time.Now().Add(2 * authFailMaxWait) }
	if c := attempt("admin", "admin").Code; c != http.StatusNoContent {
		t.Fatalf("post-cooldown good login code=%d want 204", c)
	}
	for i := 0; i < authFailThreshold-1; i++ {
		if c := attempt("admin", "wrong").Code; c != http.StatusUnauthorized {
			t.Fatalf("post-reset fail %d: code=%d want 401", i, c)
		}
	}
}

func TestAuthPasswordCache(t *testing.T) {
	s := testServer(t)
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	attempt := func(user, pass string) int {
		req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
		req.SetBasicAuth(user, pass)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if c := attempt("admin", "admin"); c != http.StatusNoContent {
		t.Fatalf("first login code=%d want 204", c)
	}
	s.pass.mu.Lock()
	cached := s.pass.hash
	s.pass.mu.Unlock()
	if cached == "" {
		t.Fatal("successful login must populate the password cache")
	}
	// Repeated logins keep working (these are the cache hits).
	for i := 0; i < 3; i++ {
		if c := attempt("admin", "admin"); c != http.StatusNoContent {
			t.Fatalf("repeat login %d code=%d want 204", i, c)
		}
	}
	// Only successes are cached: a wrong password is never admitted.
	if c := attempt("admin", "nope"); c != http.StatusUnauthorized {
		t.Fatalf("wrong password code=%d want 401", c)
	}

	// A password change invalidates the cached verdict.
	body := strings.NewReader(`{"old_password":"admin","new_password":"newpass1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/password", body)
	req.SetBasicAuth("admin", "admin")
	rec := httptest.NewRecorder()
	s.handlePassword(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("password change code=%d body=%s", rec.Code, rec.Body.String())
	}
	if c := attempt("admin", "admin"); c != http.StatusUnauthorized {
		t.Fatalf("old password after change code=%d want 401", c)
	}
	if c := attempt("admin", "newpass1"); c != http.StatusNoContent {
		t.Fatalf("new password code=%d want 204", c)
	}
}
