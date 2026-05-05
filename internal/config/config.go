// Package config persists user preferences across sessions.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Offset  float64 `json:"offset"`
	Speed   float64 `json:"speed"`
	Keys    string  `json:"keys"`
	LastDir string  `json:"last_dir"`
	Lanes   int     `json:"lanes"`
	MinGap  float64 `json:"min_gap"`
}

func defaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tempo-tty", "config.json")
}

func Defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Offset:  0,
		Speed:   3.5,
		Keys:    "dfjk",
		LastDir: home,
		Lanes:   4,
		MinGap:  0.08,
	}
}

func Load() *Config {
	cfg := Defaults()
	data, err := os.ReadFile(defaultPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, cfg)
	if cfg.Speed == 0 {
		cfg.Speed = 3.5
	}
	if cfg.Keys == "" {
		cfg.Keys = "dfjk"
	}
	if cfg.Lanes == 0 {
		cfg.Lanes = 4
	}
	if cfg.MinGap == 0 {
		cfg.MinGap = 0.08
	}
	if cfg.LastDir == "" {
		home, _ := os.UserHomeDir()
		cfg.LastDir = home
	}
	return cfg
}

func Save(cfg *Config) error {
	p := defaultPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
