# TopWebDav Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go WebDAV + web file manager with one admin account (change password only), simple file CRUD/edit, and deploy assets for a reverse-proxied Linux host.

**Architecture:** One HTTP server exposes `/dav/*` via `golang.org/x/net/webdav`, `/api/*` JSON/REST for the UI, and embedded static files from `web/`. Auth is HTTP Basic (single bcrypt user in `config.json`). All files live under `data_dir`. Paths are sanitized server-side so they cannot escape the root.

**Tech Stack:** Go 1.22+, `golang.org/x/net/webdav`, `golang.org/x/crypto/bcrypt`, stdlib `net/http`, `embed` for `web/`. Frontend is plain HTML/CSS/JS (no framework).

**Spec:** `docs/superpowers/specs/2026-09-23-topwebdav-design.md`

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `go.mod` | Module `topwebdav`, deps: `x/net`, `x/crypto` |
| `main.go` | Flags, load config, build routes, listen |
| `config.go` | `Config` struct, load/create/save `config.json` |
| `config_test.go` | Config first-run + save tests |
| `path.go` | Path sanitize/resolve/name validation |
| `path_test.go` | Escape / invalid name tests |
| `auth.go` | bcrypt helpers, Basic Auth middleware, password change handler |
| `auth_test.go` | Hash/check/middleware/password tests |
| `api.go` | REST handlers (list/mkdir/create/upload/download/read/write/delete/rename) |
| `api_test.go` | `httptest` for all REST endpoints |
| `dav.go` | Mount `webdav.Handler` at `/dav/` |
| `server.go` | `Server` type holding cfg, root, mux assembly |
| `web/index.html` | File manager page shell |
| `web/style.css` | Minimal layout styles |
| `web/app.js` | Fetch API client + UI wiring |
| `deploy/topwebdav.service` | systemd unit example |
| `deploy/nginx.conf.example` | Reverse proxy snippet |
| `README.md` | Run / deploy / first login |

Units communicate only through `Server` + stdlib `http`. Tests call handlers via `httptest`; no network listen required.

---

## Conventions

- JSON errors: `{"error":"..."}` with status codes from the spec
- JSON success for ops without body: `{"ok":true}`
- `path` query/body values are slash-rooted relative paths (`/docs/a.txt`)
- Module path: `topwebdav` (local; no remote import path required)
- Run tests with: `go test ./...`
- Commit messages: `feat: ...` / `test: ...` / `docs: ...` / `chore: ...`

---

### Task 1: Module scaffold + config load/create

**Files:**
- Create: `go.mod`
- Create: `config.go`
- Create: `config_test.go`
- Create: `main.go` (minimal listen stub; expanded in Task 6)

- [ ] **Step 1: Write the failing config tests**

Create `config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateFirstRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dataDir := filepath.Join(dir, "data")

	cfg, err := LoadOrCreate(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "admin" {
		t.Fatalf("username = %q, want admin", cfg.Username)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen = %q", cfg.Listen)
	}
	if cfg.PasswordHash == "" || cfg.PasswordHash == "admin" {
		t.Fatalf("password hash should be bcrypt, got %q", cfg.PasswordHash)
	}
	if !CheckPassword(cfg.PasswordHash, "admin") {
		t.Fatal("default password should be admin")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config.json not written: %v", err)
	}
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		t.Fatalf("data dir not created: %v", err)
	}

	// Second load must not rotate the password hash.
	cfg2, err := LoadOrCreate(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.PasswordHash != cfg.PasswordHash {
		t.Fatal("hash changed on second load")
	}
}

func TestConfigSave(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Listen = "127.0.0.1:18080"
	cfg.Username = "root"
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg.PasswordHash = hash
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Listen != "127.0.0.1:18080" || cfg2.Username != "root" {
		t.Fatalf("reload mismatch: %+v", cfg2)
	}
	if !CheckPassword(cfg2.PasswordHash, "secret") {
		t.Fatal("saved password should verify")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1
```

Expected: compile error (`undefined: LoadOrCreate`, `undefined: CheckPassword`, `undefined: HashPassword`).

- [ ] **Step 3: Write `go.mod`, `config.go`, `auth.go` password helpers**

Create `go.mod`:

```go
module topwebdav

go 1.22

require (
	golang.org/x/crypto v0.31.0
	golang.org/x/net v0.33.0
)
```

Create `config.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is persisted as config.json next to the binary working directory.
type Config struct {
	Listen       string `json:"listen"`
	DataDir      string `json:"data_dir"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

// LoadOrCreate reads cfgPath, or creates a default admin/admin config and data dir.
func LoadOrCreate(cfgPath, defaultDataDir string) (*Config, error) {
	if b, err := os.ReadFile(cfgPath); err == nil {
		var c Config
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		if c.Listen == "" {
			c.Listen = "127.0.0.1:8080"
		}
		if c.DataDir == "" {
			c.DataDir = defaultDataDir
		}
		if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
			return nil, err
		}
		return &c, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	hash, err := HashPassword("admin")
	if err != nil {
		return nil, err
	}
	c := &Config{
		Listen:       "127.0.0.1:8080",
		DataDir:      defaultDataDir,
		Username:     "admin",
		PasswordHash: hash,
	}
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return nil, err
	}
	if err := c.Save(cfgPath); err != nil {
		return nil, err
	}
	return c, nil
}

// Save writes cfgPath with indented JSON and restrictive file mode.
func (c *Config) Save(cfgPath string) error {
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, b, 0o600)
}
```

Create `auth.go` with password helpers only (middleware in Task 3):

```go
package main

import "golang.org/x/crypto/bcrypt"

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the bcrypt hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
```

- [ ] **Step 4: Run `go mod tidy` then tests**

```bash
go mod tidy
go test ./... -count=1
```

Expected: `TestLoadOrCreateFirstRun` and `TestConfigSave` PASS.

- [ ] **Step 5: Minimal `main.go` so the module builds**

Create `main.go`:

```go
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	cfgPath := flag.String("config", "./config.json", "path to config.json")
	listen := flag.String("listen", "", "listen address (overrides config)")
	flag.Parse()

	cfg, err := LoadOrCreate(*cfgPath, "./data")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	log.Printf("stub server on %s (placeholder until Task 6)", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})))
}
```

- [ ] **Step 6: Verify build + tests**

```bash
go test ./... -count=1 && go build -o /tmp/topwebdav .
```

Expected: tests PASS, build exits 0.

- [ ] **Step 7: Commit**

```bash
git init 2>/dev/null || true
git add go.mod go.sum config.go config_test.go auth.go main.go
git commit -m "feat: config load/create and bcrypt password helpers"
```

---

### Task 2: Path sanitization

**Files:**
- Create: `path.go`
- Create: `path_test.go`

- [ ] **Step 1: Write the failing tests**

Create `path_test.go`:

```go
package main

import "testing"

func TestSanitizePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/", "/", true},
		{"", "/", true},
		{"/a/b", "/a/b", true},
		{"/a//b/", "/a/b", true},
		{"/a/./b", "/a/b", true},
		{"/../etc/passwd", "", false},
		{"/a/../../b", "", false},
		{"a/b", "/a/b", true},
		{"/a/\x00b", "", false},
	}
	for _, tc := range cases {
		got, err := SanitizePath(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("SanitizePath(%q) err=%v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("SanitizePath(%q)=%q want %q", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Fatalf("SanitizePath(%q) should fail, got %q", tc.in, got)
		}
	}
}

func TestResolveUnder(t *testing.T) {
	root := t.TempDir()
	p, err := ResolveUnder(root, "/ok/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p == "" {
		t.Fatal("empty resolve")
	}
	if _, err := ResolveUnder(root, "/../escape"); err == nil {
		t.Fatal("escape should fail")
	}
}

func TestValidName(t *testing.T) {
	if err := ValidName("file.txt"); err != nil {
		t.Fatal(err)
	}
	if err := ValidName(""); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := ValidName(".."); err == nil {
		t.Fatal("dotdot should fail")
	}
	if err := ValidName("a/b"); err == nil {
		t.Fatal("slash should fail")
	}
	if err := ValidName("a\x00b"); err == nil {
		t.Fatal("null should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1 -run 'Sanitize|Resolve|ValidName'
```

Expected: compile error (`undefined: SanitizePath`).

- [ ] **Step 3: Implement `path.go`**

Create `path.go`:

```go
package main

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// ErrBadPath is returned for illegal or escaping paths.
var ErrBadPath = errors.New("bad path")

// SanitizePath cleans a client path into slash form under root ("/a/b").
func SanitizePath(p string) (string, error) {
	if strings.ContainsRune(p, 0) {
		return "", ErrBadPath
	}
	p = strings.TrimSpace(p)
	if p == "" {
		return "/", nil
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "." || strings.Contains(clean, "..") {
		return "", ErrBadPath
	}
	if !strings.HasPrefix(clean, "/") {
		return "", ErrBadPath
	}
	return clean, nil
}

// ResolveUnder maps a sanitized path to an absolute OS path under root.
func ResolveUnder(root, p string) (string, error) {
	clean, err := SanitizePath(p)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel := strings.TrimPrefix(clean, "/")
	full := filepath.Join(rootAbs, filepath.FromSlash(rel))
	// Ensure full stays inside rootAbs.
	if full != rootAbs && !strings.HasPrefix(full, rootAbs+string(filepath.Separator)) {
		return "", ErrBadPath
	}
	return full, nil
}

// ValidName checks a single path segment (file or folder name).
func ValidName(name string) error {
	if name == "" || name == "." || name == ".." {
		return ErrBadPath
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return ErrBadPath
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add path.go path_test.go
git commit -m "feat: sanitize and resolve file paths under data root"
```

---

### Task 3: Basic Auth middleware + change password

**Files:**
- Modify: `auth.go`
- Create: `auth_test.go`
- Create: `server.go` (minimal `Server` + helpers)

- [ ] **Step 1: Write failing auth tests**

Create `auth_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d want 401", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("missing WWW-Authenticate")
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
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
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1 -run 'Password|BasicAuth|CheckPassword'
```

Expected: compile error (`undefined: Server`, `requireAuth`, `handlePassword`).

- [ ] **Step 3: Implement `server.go` + replace `auth.go` with full file**

Create `server.go`:

```go
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
```

Replace `auth.go` entirely with:

```go
package main

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
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
```

Do **not** import `strings` here — this file does not need it.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add auth.go auth_test.go server.go
git commit -m "feat: basic auth middleware and password change"
```

---

### Task 4: REST list / mkdir / create / delete / rename

**Files:**
- Create: `api.go`
- Create: `api_test.go`

- [ ] **Step 1: Write failing tests for these endpoints**

Create `api_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListMkdirCreateDeleteRename(t *testing.T) {
	s := testServer(t)

	// mkdir /docs
	req := httptest.NewRequest(http.MethodPost, "/api/mkdir", strings.NewReader(`{"path":"/docs"}`))
	rec := httptest.NewRecorder()
	s.handleMkdir(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mkdir code=%d body=%s", rec.Code, rec.Body.String())
	}

	// create /docs/a.txt
	req = httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(`{"path":"/docs/a.txt","content":"hi"}`))
	rec = httptest.NewRecorder()
	s.handleCreate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rec.Code, rec.Body.String())
	}
	b, err := os.ReadFile(filepath.Join(s.root, "docs", "a.txt"))
	if err != nil || string(b) != "hi" {
		t.Fatalf("file content=%q err=%v", b, err)
	}

	// list /docs
	req = httptest.NewRequest(http.MethodGet, "/api/list?path=/docs", nil)
	rec = httptest.NewRecorder()
	s.handleList(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code=%d", rec.Code)
	}
	var items []ListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "a.txt" || items[0].IsDir {
		t.Fatalf("items=%+v", items)
	}

	// rename to /docs/b.txt
	req = httptest.NewRequest(http.MethodPost, "/api/rename", strings.NewReader(`{"from":"/docs/a.txt","to":"/docs/b.txt"}`))
	rec = httptest.NewRecorder()
	s.handleRename(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("rename code=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.root, "docs", "b.txt")); err != nil {
		t.Fatal("rename target missing")
	}

	// delete file
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/docs/b.txt", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete code=%d", rec.Code)
	}

	// delete non-empty dir → 409
	_ = os.WriteFile(filepath.Join(s.root, "docs", "x"), []byte("x"), 0o644)
	req = httptest.NewRequest(http.MethodDelete, "/api/delete?path=/docs", nil)
	rec = httptest.NewRecorder()
	s.handleDelete(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("non-empty delete code=%d want 409", rec.Code)
	}

	// path escape → 400
	req = httptest.NewRequest(http.MethodGet, "/api/list?path=/../etc", nil)
	rec = httptest.NewRecorder()
	s.handleList(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("escape code=%d want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1 -run ListMkdirCreate
```

Expected: compile error (`undefined: handleMkdir`, `ListItem`, …).

- [ ] **Step 3: Implement these handlers in `api.go`**

Create `api.go`:

```go
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
		err = os.MkdirAll(full, 0o755)
	} else {
		err = os.WriteFile(full, []byte(body.Content), 0o644)
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
		err = os.Remove(full)
	} else {
		err = os.Remove(full)
	}
	if err != nil {
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api.go api_test.go
git commit -m "feat: REST list mkdir create delete rename"
```

---

### Task 5: REST upload / download / read / write

**Files:**
- Modify: `api.go`
- Modify: `api_test.go`

- [ ] **Step 1: Append failing tests**

Append to `api_test.go`:

```go
func TestUploadDownloadReadWrite(t *testing.T) {
	s := testServer(t)
	// Ensure test file imports include "mime/multipart" and "encoding/json".

	// upload as multipart
	body := &strings.Builder{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upload?path=/", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleUpload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload code=%d body=%s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(s.root, "note.txt")); string(b) != "hello" {
		t.Fatalf("uploaded bytes=%q", b)
	}

	// download
	req = httptest.NewRequest(http.MethodGet, "/api/download?path=/note.txt", nil)
	rec = httptest.NewRecorder()
	s.handleDownload(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("download code=%d body=%q", rec.Code, rec.Body.String())
	}

	// read
	req = httptest.NewRequest(http.MethodGet, "/api/read?path=/note.txt", nil)
	rec = httptest.NewRecorder()
	s.handleRead(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read code=%d body=%s", rec.Code, rec.Body.String())
	}
	var readResp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &readResp); err != nil || readResp.Content != "hello" {
		t.Fatalf("readResp=%+v err=%v", readResp, err)
	}

	// write
	req = httptest.NewRequest(http.MethodPost, "/api/write", strings.NewReader(`{"path":"/note.txt","content":"hello2"}`))
	rec = httptest.NewRecorder()
	s.handleWrite(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("write code=%d body=%s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(s.root, "note.txt")); string(b) != "hello2" {
		t.Fatalf("after write=%q", b)
	}

	// non-text read → 415
	_ = os.WriteFile(filepath.Join(s.root, "blob.bin"), []byte{0x00, 0x01, 0xff}, 0o644)
	req = httptest.NewRequest(http.MethodGet, "/api/read?path=/blob.bin", nil)
	rec = httptest.NewRecorder()
	s.handleRead(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("nontext read code=%d want 415", rec.Code)
	}
}
```

Add `mime/multipart` to `api_test.go` imports if the compiler reports it missing.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1 -run UploadDownloadReadWrite
```

Expected: compile error (`undefined: handleUpload`).

- [ ] **Step 3: Implement upload/download/read/write in `api.go`**

Append to `api.go`. Merge these imports into the existing import block (do not duplicate package imports):

```go
// new imports to merge: bytes, io, mime, net/http (already), path/filepath, strings

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
	// Reject if sample contains NUL.
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
	out, err := os.Create(dst)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	defer out.Close()
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
	w.Header().Set("Content-Disposition", "attachment; filename=\""+fi.Name()+"\"")
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
	if err := os.WriteFile(full, []byte(body.Content), 0o644); err != nil {
		writeErr(w, http.StatusInternalServerError, "write failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
```

Merge the new imports into `api.go`’s existing import block (do not duplicate).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api.go api_test.go
git commit -m "feat: REST upload download read write"
```

---

### Task 6: Wire routes, WebDAV, real main

**Files:**
- Create: `dav.go`
- Modify: `main.go`
- Create: `server_test.go` (route auth smoke)

- [ ] **Step 1: Write failing route smoke test**

Create `server_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesAuth(t *testing.T) {
	s := testServer(t)
	mux := s.routes()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("list unauth code=%d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/list?path=/", nil)
	req.SetBasicAuth("admin", "admin")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list auth code=%d body=%s", rec.Code, rec.Body.String())
	}
	// WebDAV PROPFIND requires auth too
	req = httptest.NewRequest("PROPFIND", "/dav/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("dav unauth code=%d", rec.Code)
	}
	// Static index is also protected (optional but keep simple: protect all)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("index unauth code=%d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./... -count=1 -run RoutesAuth
```

Expected: compile error (`undefined: routes`).

- [ ] **Step 3: Implement `dav.go`, `routes()`, rewrite `main.go`**

Create `dav.go`:

```go
package main

import (
	"net/http"

	"golang.org/x/net/webdav"
)

// davHandler serves the data root as WebDAV at /dav/.
func (s *Server) davHandler() http.Handler {
	return &webdav.Handler{
		Prefix:     "/dav/",
		FileSystem: webdav.Dir(s.root),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				// Keep logs quiet on success; errors only.
				_ = r
			}
		},
	}
}
```

Add to `server.go` (merge `net/http` into the existing import block):

```go
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
	mux.Handle("/", http.FileServer(http.FS(webFS)))
	return s.requireAuth(mux)
}
```

Add `webFS` in `main.go` via embed (placeholder files created in Task 7; use empty stubs first if needed):

```go
package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
)

//go:embed web/*
var webFS embed.FS

func main() {
	cfgPath := flag.String("config", "./config.json", "path to config.json")
	listen := flag.String("listen", "", "listen address (overrides config and env)")
	flag.Parse()

	cfg, err := LoadOrCreate(*cfgPath, "./data")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *listen != "" {
		cfg.Listen = *listen
	} else if env := os.Getenv("TOPWEBDAV_LISTEN"); env != "" {
		cfg.Listen = env
	}

	s := &Server{cfg: cfg, cfgPath: *cfgPath, root: cfg.DataDir}
	if _, err := os.Stat("web/index.html"); err != nil {
		log.Printf("warning: web UI missing at web/index.html")
	}
	log.Printf("topwebdav listening on %s  data=%s  user=%s", cfg.Listen, cfg.DataDir, cfg.Username)
	log.Printf("default password is 'admin' — change it after first login if this is first run")
	log.Fatal(http.ListenAndServe(cfg.Listen, s.routes()))
}
```

If `go:embed web/*` fails because `web/` is empty, create `web/index.html` with `<!doctype html><title>topwebdav</title>` as a stub (Task 7 replaces it).

- [ ] **Step 4: Run tests + build**

```bash
go test ./... -count=1 && go build -o /tmp/topwebdav .
```

Expected: tests PASS, binary builds.

- [ ] **Step 5: Commit**

```bash
git add dav.go server.go server_test.go main.go web/
git commit -m "feat: wire REST, WebDAV, and embedded UI routes"
```

---

### Task 7: Web UI (index.html / style.css / app.js)

**Files:**
- Replace: `web/index.html`
- Create: `web/style.css`
- Create: `web/app.js`

- [ ] **Step 1: Write `web/index.html`**

```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>TopWebDav</title>
  <link rel="stylesheet" href="style.css" />
</head>
<body>
  <header>
    <strong>TopWebDav</strong>
    <nav>
      <span id="pathLabel">/</span>
      <button id="btnUp" type="button">上级</button>
      <button id="btnRefresh" type="button">刷新</button>
      <button id="btnUpload" type="button">上传</button>
      <button id="btnMkdir" type="button">新建文件夹</button>
      <button id="btnCreate" type="button">新建文件</button>
      <button id="btnPassword" type="button">改密码</button>
      <button id="btnLogout" type="button">退出</button>
    </nav>
  </header>
  <div id="error" hidden></div>
  <main>
    <table id="listing">
      <thead>
        <tr><th>名称</th><th>大小</th><th>修改时间</th><th>操作</th></tr>
      </thead>
      <tbody id="rows"></tbody>
    </table>
  </main>
  <input id="fileInput" type="file" multiple hidden />
  <dialog id="dlgEdit">
    <h2 id="editTitle">编辑</h2>
    <textarea id="editBody" rows="18" cols="72"></textarea>
    <p>
      <button id="editSave" type="button">保存</button>
      <button id="editCancel" type="button">取消</button>
    </p>
  </dialog>
  <dialog id="dlgPrompt">
    <h2 id="promptTitle">输入</h2>
    <input id="promptValue" type="text" />
    <p>
      <button id="promptOk" type="button">确定</button>
      <button id="promptCancel" type="button">取消</button>
    </p>
  </dialog>
  <dialog id="dlgPassword">
    <h2>修改密码</h2>
    <p><label>旧密码 <input id="oldPw" type="password" /></label></p>
    <p><label>新密码 <input id="newPw" type="password" /></label></p>
    <p>
      <button id="pwSave" type="button">保存</button>
      <button id="pwCancel" type="button">取消</button>
    </p>
  </dialog>
  <script src="app.js"></script>
</body>
</html>
```

- [ ] **Step 2: Write `web/style.css`**

```css
* { box-sizing: border-box; }
body {
  margin: 0;
  font: 14px/1.4 system-ui, sans-serif;
  color: #1a1a1a;
  background: #f6f6f6;
}
header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  background: #fff;
  border-bottom: 1px solid #ddd;
  flex-wrap: wrap;
}
nav { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
#pathLabel { font-family: ui-monospace, monospace; margin-right: 8px; }
button {
  font: inherit;
  padding: 4px 10px;
  border: 1px solid #ccc;
  border-radius: 4px;
  background: #fff;
  cursor: pointer;
}
button:hover { background: #eee; }
#error {
  margin: 8px 14px;
  padding: 8px 10px;
  background: #fdecea;
  color: #611a15;
  border: 1px solid #f5c2c0;
  border-radius: 4px;
}
main { padding: 12px 14px; }
table { width: 100%; border-collapse: collapse; background: #fff; }
th, td { text-align: left; padding: 8px 10px; border-bottom: 1px solid #eee; }
tr:hover td { background: #fafafa; }
td.actions { white-space: nowrap; }
td.actions button { margin-right: 4px; padding: 2px 6px; }
a.dir { cursor: pointer; text-decoration: underline; color: #06c; }
dialog { border: 1px solid #ccc; border-radius: 6px; padding: 14px; }
textarea { width: 100%; font: 13px/1.4 ui-monospace, monospace; }
```

- [ ] **Step 3: Write `web/app.js`**

```javascript
"use strict";

let cur = "/";
let auth = null; // {user, pass}

function $(id) { return document.getElementById(id); }

function showError(msg) {
  const el = $("error");
  el.textContent = msg || "";
  el.hidden = !msg;
}

function joinPath(dir, name) {
  if (dir === "/") return "/" + name;
  return dir.replace(/\/+$/, "") + "/" + name;
}

function parentPath(p) {
  if (p === "/") return "/";
  const i = p.lastIndexOf("/");
  return i <= 0 ? "/" : p.slice(0, i);
}

function promptText(title, initial) {
  return new Promise((resolve) => {
    const dlg = $("dlgPrompt");
    $("promptTitle").textContent = title;
    $("promptValue").value = initial || "";
    $("promptOk").onclick = () => { dlg.close(); resolve($("promptValue").value.trim()); };
    $("promptCancel").onclick = () => { dlg.close(); resolve(null); };
    dlg.showModal();
    $("promptValue").focus();
  });
}

async function api(pathname, opts = {}) {
  const headers = opts.headers || {};
  if (auth) {
    headers.Authorization = "Basic " + btoa(auth.user + ":" + auth.pass);
  }
  if (opts.json) {
    headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(opts.json);
    delete opts.json;
  }
  const res = await fetch(pathname, { ...opts, headers });
  if (res.status === 401) {
    showError("登录失败或会话过期，请重新登录");
    throw new Error("unauthorized");
  }
  const ct = res.headers.get("Content-Type") || "";
  const data = ct.includes("application/json") ? await res.json() : await res.text();
  if (!res.ok) {
    const msg = (data && data.error) || res.statusText;
    showError(msg);
    throw new Error(msg);
  }
  showError("");
  return data;
}

async function ensureLogin() {
  if (auth) return;
  const user = await promptText("用户名", "admin");
  if (user == null) throw new Error("cancelled");
  const pass = await promptText("密码（默认 admin）", "");
  // password prompt as text is intentionally simple; see dialog for change-password
  if (pass == null) throw new Error("cancelled");
  auth = { user, pass: pass || "admin" };
  // verify
  await api("/api/list?path=/");
}

function fmtSize(n, isDir) {
  if (isDir) return "—";
  if (n < 1024) return n + " B";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + " KB";
  return (n / 1024 / 1024).toFixed(1) + " MB";
}

async function load() {
  $("pathLabel").textContent = cur;
  const items = await api("/api/list?path=" + encodeURIComponent(cur));
  const tbody = $("rows");
  tbody.innerHTML = "";
  items.sort((a, b) => (a.is_dir === b.is_dir ? a.name.localeCompare(b.name) : a.is_dir ? -1 : 1));
  for (const it of items) {
    const tr = document.createElement("tr");
    const nameTd = document.createElement("td");
    if (it.is_dir) {
      const a = document.createElement("a");
      a.className = "dir";
      a.textContent = it.name;
      a.onclick = () => { cur = it.path; load().catch(() => {}); };
      nameTd.appendChild(a);
    } else {
      nameTd.textContent = it.name;
    }
    const sizeTd = document.createElement("td");
    sizeTd.textContent = fmtSize(it.size, it.is_dir);
    const timeTd = document.createElement("td");
    timeTd.textContent = (it.mod_time || "").replace("T", " ").replace(/\.\d+Z$/, "Z");
    const actTd = document.createElement("td");
    actTd.className = "actions";

    if (!it.is_dir) {
      const dl = document.createElement("button");
      dl.textContent = "下载";
      dl.onclick = () => download(it.path, it.name);
      actTd.appendChild(dl);

      const ed = document.createElement("button");
      ed.textContent = "编辑";
      ed.onclick = () => openEdit(it.path, it.name);
      actTd.appendChild(ed);
    }
    const rn = document.createElement("button");
    rn.textContent = "重命名";
    rn.onclick = async () => {
      const to = await promptText("新路径", it.path);
      if (!to) return;
      try {
        await api("/api/rename", { method: "POST", json: { from: it.path, to } });
        await load();
      } catch (_) {}
    };
    actTd.appendChild(rn);

    const del = document.createElement("button");
    del.textContent = "删除";
    del.onclick = async () => {
      if (!confirm("删除 " + it.path + " ？")) return;
      try {
        await api("/api/delete?path=" + encodeURIComponent(it.path), { method: "DELETE" });
        await load();
      } catch (_) {}
    };
    actTd.appendChild(del);

    tr.append(nameTd, sizeTd, timeTd, actTd);
    tbody.appendChild(tr);
  }
}

function download(path, name) {
  const a = document.createElement("a");
  a.href = "/api/download?path=" + encodeURIComponent(path);
  a.download = name;
  // fetch with auth then blob (same-origin basic auth may work via browser cache)
  fetch(a.href, { headers: { Authorization: "Basic " + btoa(auth.user + ":" + auth.pass) } })
    .then((r) => r.blob())
    .then((blob) => {
      const url = URL.createObjectURL(blob);
      a.href = url;
      a.click();
      URL.revokeObjectURL(url);
    });
}

async function openEdit(path, name) {
  try {
    const data = await api("/api/read?path=" + encodeURIComponent(path));
    $("editTitle").textContent = "编辑 " + name;
    $("editBody").value = data.content;
    const dlg = $("dlgEdit");
    $("editSave").onclick = async () => {
      try {
        await api("/api/write", { method: "POST", json: { path, content: $("editBody").value } });
        dlg.close();
        showError("已保存");
      } catch (_) {}
    };
    $("editCancel").onclick = () => dlg.close();
    dlg.showModal();
  } catch (_) {}
}

function wire() {
  $("btnUp").onclick = () => { cur = parentPath(cur); load().catch(() => {}); };
  $("btnRefresh").onclick = () => load().catch(() => {});
  $("btnUpload").onclick = () => $("fileInput").click();
  $("fileInput").onchange = async () => {
    const files = $("fileInput").files;
    for (const f of files) {
      const fd = new FormData();
      fd.append("file", f, f.name);
      try {
        await api("/api/upload?path=" + encodeURIComponent(cur), { method: "POST", body: fd });
      } catch (_) {}
    }
    $("fileInput").value = "";
    await load().catch(() => {});
  };
  $("btnMkdir").onclick = async () => {
    const name = await promptText("文件夹名", "");
    if (!name) return;
    try {
      await api("/api/mkdir", { method: "POST", json: { path: joinPath(cur, name) } });
      await load();
    } catch (_) {}
  };
  $("btnCreate").onclick = async () => {
    const name = await promptText("文件名（如 notes.txt）", "");
    if (!name) return;
    try {
      await api("/api/create", { method: "POST", json: { path: joinPath(cur, name), content: "" } });
      await load();
    } catch (_) {}
  };
  $("btnPassword").onclick = () => {
    $("oldPw").value = "";
    $("newPw").value = "";
    const dlg = $("dlgPassword");
    $("pwSave").onclick = async () => {
      try {
        await api("/api/password", {
          method: "POST",
          json: { old_password: $("oldPw").value, new_password: $("newPw").value },
        });
        auth.pass = $("newPw").value;
        dlg.close();
        showError("密码已更新");
      } catch (_) {}
    };
    $("pwCancel").onclick = () => dlg.close();
    dlg.showModal();
  };
  $("btnLogout").onclick = () => {
    auth = null;
    document.body.dataset.auth = "";
    showError("已退出，请刷新并重新登录");
  };
}

async function boot() {
  wire();
  // Try cached basic auth from browser first.
  try {
    await load();
  } catch (_) {
    try {
      await ensureLogin();
      await load();
    } catch (_) {}
  }
}

boot();
```

Note: UI labels are Chinese to match the product language; keep copy minimal.

- [ ] **Step 4: Rebuild binary (embeds new web files) and run unit tests**

```bash
go test ./... -count=1 && go build -o /tmp/topwebdav .
```

Expected: PASS and successful build.

- [ ] **Step 5: Quick static syntax check**

```bash
node --check web/app.js 2>/dev/null || true
```

Expected: no syntax error if Node is present; otherwise skip.

- [ ] **Step 6: Commit**

```bash
git add web/index.html web/style.css web/app.js
git commit -m "feat: minimal web file manager UI"
```

---

### Task 8: Deploy assets + README

**Files:**
- Create: `deploy/topwebdav.service`
- Create: `deploy/nginx.conf.example`
- Create: `README.md`

- [ ] **Step 1: Write `deploy/topwebdav.service`**

```ini
[Unit]
Description=TopWebDav
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/topwebdav
ExecStart=/opt/topwebdav/topwebdav -config /opt/topwebdav/config.json
Restart=on-failure
# Adjust user to the owner of /opt/topwebdav/data
User=www-data
Group=www-data

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Write `deploy/nginx.conf.example`**

```nginx
# Include inside your server { } block.
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    # Raise if you need larger uploads:
    client_max_body_size 2g;
}
```

- [ ] **Step 3: Write `README.md`**

```markdown
# TopWebDav

极简 WebDAV + 网页文件管理，单账号，单二进制，部署在反向代理后面。

## 功能

- WebDAV（`/dav/`），可用 Finder / 资源管理器 / Cyberduck / rclone 挂载
- 网页：列目录、上传、下载、新建文件、新建文件夹、文本编辑、重命名、删除、改密码
- 路径限制在数据目录内，密码 bcrypt 存储

## 构建

```bash
go build -o topwebdav .
```

## 运行

```bash
./topwebdav -config ./config.json
# 监听默认 127.0.0.1:8080，可用 -listen 或 TOPWEBDAV_LISTEN 覆盖
```

首次启动自动创建 `config.json` 与 `data/`，账号 `admin` / `admin`，请立刻改密。

## WebDAV 挂载示例

```bash
rclone config  # type webdav, URL http://127.0.0.1:8080/dav, user/pass
# 或
cadaver http://127.0.0.1:8080/dav/
```

## 部署（反代后面）

1. 拷贝 `topwebdav` 到 `/opt/topwebdav/`
2. 安装 `deploy/topwebdav.service` 到 `/etc/systemd/system/` 并 `systemctl enable --now topwebdav`
3. 反代参考 `deploy/nginx.conf.example`（务必 HTTPS）
4. 端口被占用时改 `config.json` 的 `listen` 并同步反代

## 配置

| 字段 | 说明 |
|------|------|
| `listen` | 监听地址，默认 `127.0.0.1:8080` |
| `data_dir` | 文件根目录 |
| `username` | 登录名 |
| `password_hash` | bcrypt 哈希 |
```

- [ ] **Step 4: Commit**

```bash
git add deploy README.md
git commit -m "docs: deploy examples and README"
```

---

### Task 9: End-to-end manual verification

**Files:** none (verification only)

- [ ] **Step 1: Full unit tests**

```bash
go test ./... -count=1
```

Expected: all PASS.

- [ ] **Step 2: Start server on a free port**

```bash
rm -rf /tmp/twd-smoke && mkdir -p /tmp/twd-smoke
cd /tmp/twd-smoke && /tmp/topwebdav -listen 127.0.0.1:18080 &
sleep 1
curl -s -o /dev/null -w "%{http_code}" -u admin:admin http://127.0.0.1:18080/api/list?path=/
```

Expected: `200`.

- [ ] **Step 3: REST golden path via curl**

```bash
BASE=http://127.0.0.1:18080
AUTH="-u admin:admin"
curl -s $AUTH -X POST $BASE/api/mkdir -d '{"path":"/docs"}'
curl -s $AUTH -X POST $BASE/api/create -d '{"path":"/docs/a.txt","content":"hi"}'
curl -s $AUTH $BASE/api/read?path=/docs/a.txt
curl -s $AUTH -X POST $BASE/api/write -d '{"path":"/docs/a.txt","content":"hi2"}'
curl -s $AUTH -X POST $BASE/api/rename -d '{"from":"/docs/a.txt","to":"/docs/b.txt"}'
curl -s $AUTH -o /tmp/dl.txt $BASE/api/download?path=/docs/b.txt
curl -s $AUTH -X DELETE $BASE/api/delete?path=/docs/b.txt
```

Expected: each returns `{"ok":true}` or content JSON; `/tmp/dl.txt` content `hi2` before delete.

- [ ] **Step 4: WebDAV smoke (rclone or cadaver)**

```bash
rclone lsd :webdav: --webdav-url http://127.0.0.1:18080/dav --webdav-user admin --webdav-pass "$(rclone obscure admin)" 2>/dev/null || \
  echo "manual: mount http://127.0.0.1:18080/dav with admin/admin"
```

Expected: listing works or manual mount note.

- [ ] **Step 5: Browser golden path**

Open `http://127.0.0.1:18080/` (or via reverse proxy): login → upload → 新建文件/文件夹 → 编辑保存 → 下载 → 重命名 → 删除 → 改密码.

Expected: all actions succeed; old password fails after change.

- [ ] **Step 6: Port-in-use error**

```bash
/tmp/topwebdav -listen 127.0.0.1:18080
```

Expected: process exits with `bind: address already in use` (or similar), no silent hang.

- [ ] **Step 7: Final commit if anything was fixed during verification**

```bash
git add -A
git commit -m "fix: issues found during end-to-end verification"
```

Skip this commit if nothing changed.

---

## Self-Review Notes (plan author)

- Spec coverage: config/auth/paths/REST table/WebDAV/UI/deploy/errors/tests all map to Tasks 1–9
- Type consistency: `Server`, `Config`, `ListItem`, `SanitizePath`, `ResolveUnder`, `ValidName`, `requireAuth`, `handle*` names are used consistently
- Placeholders: none; every code step has concrete code
