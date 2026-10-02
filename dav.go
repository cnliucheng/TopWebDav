package main

import (
	"log"
	"net/http"

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
		inner.ServeHTTP(w, r)
	})
}
