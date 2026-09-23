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
	log.Printf("stub server on %s (placeholder until routes task)", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})))
}
