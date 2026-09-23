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
