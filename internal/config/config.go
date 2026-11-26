// Package config loads and saves the TOML settings file.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/open-rms/rigctlweb/internal/rig"
)

// Config is everything persisted between runs.
type Config struct {
	// Listen is the HTTP bind address. Loopback by default; anything else requires Token.
	Listen string `toml:"listen"`
	// Token gates the UI and API. Generated on first run.
	Token string `toml:"token"`
	// Rigs are the rigctld endpoints to manage.
	Rigs []rig.Config `toml:"rig"`
	// FavoriteModels are Hamlib model numbers pinned to the top of pickers.
	FavoriteModels []int `toml:"favorite_models"`
	// Theme is "system", "light", "dark" or "red" (night).
	Theme string `toml:"theme"`
	// Views are user-made interface views plus edits to the built-in ones.
	Views []ViewDef `toml:"view"`
	// DefaultView is the view shown to a browser that hasn't picked one.
	DefaultView string `toml:"default_view"`
	// Audio holds the receive-audio monitor settings per rig ID. Off unless enabled.
	Audio map[string]AudioConfig `toml:"audio"`
}

// AudioConfig is one rig's audio monitor setting. Listening is opt-in: nothing is
// captured unless Enabled is set and a browser asks to listen.
type AudioConfig struct {
	Enabled bool   `toml:"enabled" json:"enabled"`
	Device  string `toml:"device" json:"device"`   // capture device ID; empty = first one found
	Channel string `toml:"channel" json:"channel"` // "left", "right" or "mix" (default)

	// RestoreVolume is the sound input's volume before the app first changed it (a fraction
	// of 100%), and RestoreDevice the input it belongs to. They let "Restore original" undo
	// the change even after a restart. Never set from the API.
	RestoreDevice string   `toml:"restore_device,omitempty" json:"-"`
	RestoreVolume *float64 `toml:"restore_volume,omitempty" json:"-"`
}

func copyAudio(m map[string]AudioConfig) map[string]AudioConfig {
	out := make(map[string]AudioConfig, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ViewDef is a named set of controls to show. Built-in views live in the web UI; an entry
// with Builtin set overrides the control list of the built-in view with the same ID, and
// deleting it restores the default.
type ViewDef struct {
	ID          string   `toml:"id" json:"id"`
	Name        string   `toml:"name" json:"name"`
	Description string   `toml:"description,omitempty" json:"description,omitempty"`
	All         bool     `toml:"all,omitempty" json:"all,omitempty"`
	Controls    []string `toml:"controls" json:"controls"`
	Builtin     bool     `toml:"builtin,omitempty" json:"builtin,omitempty"`
}

// Store serialises access to the config file.
type Store struct {
	Path string
	// Created is true when this run generated a new config (and token).
	Created bool
	mu      sync.Mutex
	cfg     Config
}

// DefaultPath returns the per-user config location (XDG on Linux, AppData on Windows).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rigctlweb", "config.toml"), nil
}

// Open loads path, creating a default file (with a fresh token) if missing.
func Open(path string) (*Store, error) {
	s := &Store{Path: path}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.cfg = Config{Listen: "127.0.0.1:8480", Theme: "system"}
	case err != nil:
		return nil, err
	default:
		if err := toml.Unmarshal(b, &s.cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if s.cfg.Listen == "" {
		s.cfg.Listen = "127.0.0.1:8480"
	}
	if s.cfg.Token == "" {
		t, err := NewToken()
		if err != nil {
			return nil, err
		}
		s.cfg.Token = t
	}
	if errors.Is(err, os.ErrNotExist) {
		s.Created = true
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// NewToken returns a 256-bit URL-safe random token.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Get returns a copy of the config.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Rigs = append([]rig.Config(nil), c.Rigs...)
	c.FavoriteModels = append([]int(nil), c.FavoriteModels...)
	c.Views = append([]ViewDef(nil), c.Views...)
	c.Audio = copyAudio(c.Audio)
	return c
}

// Update applies fn to the config and saves it atomically.
func (s *Store) Update(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	next.Rigs = append([]rig.Config(nil), next.Rigs...)
	next.FavoriteModels = append([]int(nil), next.FavoriteModels...)
	next.Views = append([]ViewDef(nil), next.Views...)
	next.Audio = copyAudio(next.Audio)
	if err := fn(&next); err != nil {
		return err
	}
	prev := s.cfg
	s.cfg = next
	if err := s.saveLocked(); err != nil {
		s.cfg = prev
		return err
	}
	return nil
}

func (s *Store) saveLocked() error {
	b, err := toml.Marshal(s.cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && os.PathSeparator == '/' {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}
