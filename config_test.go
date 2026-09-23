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
