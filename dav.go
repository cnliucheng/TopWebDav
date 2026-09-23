package main

import (
	"log"
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
				log.Printf("webdav %s %s: %v", r.Method, r.URL.Path, err)
			}
		},
	}
}
