package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/webdav"
)

// davHandler serves the data root as WebDAV at /dav/.
func (s *Server) davHandler() http.Handler {
	inner := &webdav.Handler{
		Prefix: "/dav/",
		// davFS (not webdav.Dir) so symlink escapes are rejected like /api/*.
		FileSystem: davFS{root: s.root},
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("webdav %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same total-body cap as /api/upload; the webdav library streams
		// PUT/POST bodies straight to disk with no limit of its own.
		if n := s.maxUploadBytes(); n > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, n)
		}
		// A browser GET/HEAD on a collection gets a mount-instructions page
		// instead of the library's bare 405. Only directory GETs are
		// intercepted: WebDAV methods and file GETs pass through untouched.
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && s.isDavCollection(r.URL.Path) {
			http.ServeFileFS(w, r, mustWebFS(), "dav-hint.html")
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// isDavCollection reports whether urlPath names a directory under the data
// root once the /dav/ prefix is stripped. Unresolvable paths (escapes,
// missing entries, broken links) return false so the WebDAV handler keeps
// producing its usual 404s.
func (s *Server) isDavCollection(urlPath string) bool {
	full, err := ResolveUnder(s.root, strings.TrimPrefix(urlPath, "/dav/"))
	if err != nil {
		return false
	}
	fi, err := os.Stat(full)
	return err == nil && fi.IsDir()
}
