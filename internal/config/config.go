// Package config loads and saves application configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName      = "aiadventure"
	configFile   = "config.json"
	authFile     = "auth.json"
	defaultModel = "grok-4.5"
)

// Config holds user-configurable paths and defaults.
type Config struct {
	// SessionsDir is where adventure sessions are stored.
	SessionsDir string `json:"sessions_dir"`
	// AuthPath is the OAuth token store path.
	AuthPath string `json:"auth_path"`
	// Model is the last-selected xAI model id.
	Model string `json:"model,omitempty"`
	// Effort is the last-selected reasoning effort (empty if unused).
	Effort string `json:"effort,omitempty"`
}

// Paths holds resolved filesystem locations for a run.
type Paths struct {
	ConfigDir   string
	ConfigPath  string
	SessionsDir string
	AuthPath    string
}

// DefaultPaths returns standard user-config locations.
// Optional configPath and sessionsDir overrides replace defaults when non-empty.
func DefaultPaths(configPath, sessionsDir string) (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("user config dir: %w", err)
	}
	appConfigDir := filepath.Join(configDir, appName)
	p := Paths{
		ConfigDir:  appConfigDir,
		ConfigPath: filepath.Join(appConfigDir, configFile),
	}
	if configPath != "" {
		p.ConfigPath = configPath
		p.ConfigDir = filepath.Dir(configPath)
	}

	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("user home: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}

	p.SessionsDir = filepath.Join(dataDir, appName, "sessions")
	p.AuthPath = filepath.Join(p.ConfigDir, authFile)

	if sessionsDir != "" {
		p.SessionsDir = sessionsDir
	}
	return p, nil
}

// Options controls Load overrides.
type Options struct {
	// SessionsDirOverride, when non-empty, wins over the config file.
	SessionsDirOverride string
	// AuthPathOverride, when non-empty, wins over the config file.
	AuthPathOverride string
}

// Load reads config from path, applying overrides and defaults.
func Load(paths Paths, opts Options) (Config, error) {
	cfg := Config{
		SessionsDir: paths.SessionsDir,
		AuthPath:    paths.AuthPath,
		Model:       defaultModel,
	}

	data, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.SessionsDir == "" {
		cfg.SessionsDir = paths.SessionsDir
	}
	if cfg.AuthPath == "" {
		cfg.AuthPath = paths.AuthPath
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if opts.SessionsDirOverride != "" {
		cfg.SessionsDir = opts.SessionsDirOverride
	}
	if opts.AuthPathOverride != "" {
		cfg.AuthPath = opts.AuthPathOverride
	}
	return cfg, nil
}

// Save writes config to path.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// EnsureDirs creates sessions and config directories.
func EnsureDirs(cfg Config, paths Paths) error {
	if err := os.MkdirAll(cfg.SessionsDir, 0o700); err != nil {
		return fmt.Errorf("mkdir sessions: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.AuthPath), 0o700); err != nil {
		return fmt.Errorf("mkdir auth: %w", err)
	}
	if err := os.MkdirAll(paths.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("mkdir config: %w", err)
	}
	return nil
}
