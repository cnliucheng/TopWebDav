package main

import (
	"encoding/json"
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
	fi, err := os.Stat(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("data dir mode = %v, want 0700", fi.Mode().Perm())
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

func TestLoadOrCreateCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data")); err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestLoadOrCreateFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dataDir := filepath.Join(dir, "data")
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(&Config{Username: "admin", PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrCreate(cfgPath, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Fatalf("listen = %q, want 127.0.0.1:8080", cfg.Listen)
	}
	if cfg.DataDir != dataDir {
		t.Fatalf("data dir = %q, want %q", cfg.DataDir, dataDir)
	}
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		t.Fatalf("data dir not created: %v", err)
	}
	fi, err := os.Stat(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("data dir mode = %v, want 0700", fi.Mode().Perm())
	}
}

func TestSaveFileMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := LoadOrCreate(cfgPath, filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", fi.Mode().Perm())
	}
	if _, err := os.Stat(cfgPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not exist after Save, err=%v", err)
	}
}

func TestLoadOrCreateRejectsEmptyCredentials(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")

	cfgPath := filepath.Join(dir, "nouser.json")
	if err := os.WriteFile(cfgPath, []byte(`{"password_hash":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(cfgPath, dataDir); err == nil {
		t.Fatal("expected error for missing username")
	}

	cfgPath = filepath.Join(dir, "nopass.json")
	if err := os.WriteFile(cfgPath, []byte(`{"username":"admin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(cfgPath, dataDir); err == nil {
		t.Fatal("expected error for missing password_hash")
	}
}
