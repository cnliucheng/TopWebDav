package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed all:web
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
	log.Printf("topwebdav listening on %s  data=%s  user=%s", cfg.Listen, cfg.DataDir, cfg.Username)
	log.Printf("if this is first run, default password is 'admin' — change it after login")

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
