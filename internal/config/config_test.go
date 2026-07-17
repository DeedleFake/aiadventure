package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"deedles.dev/aiadventure/internal/config"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{
		ConfigDir:   dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
		AuthPath:    filepath.Join(dir, "auth.json"),
	}
	cfg, err := config.Load(paths, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model == "" {
		t.Fatal("default model")
	}
	cfg.Model = "grok-4.3"
	cfg.Effort = "medium"
	cfg.SessionsDir = filepath.Join(dir, "mysessions")
	if err := config.Save(paths.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(paths, config.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Model != "grok-4.3" || loaded.Effort != "medium" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.SessionsDir != cfg.SessionsDir {
		t.Fatalf("sessions dir = %q", loaded.SessionsDir)
	}
}

func TestSessionsDirOverride(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{
		ConfigDir:   dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
		SessionsDir: filepath.Join(dir, "default"),
		AuthPath:    filepath.Join(dir, "auth.json"),
	}
	_ = config.Save(paths.ConfigPath, config.Config{
		SessionsDir: filepath.Join(dir, "from-file"),
		Model:       "grok-4.5",
	})
	override := filepath.Join(dir, "cli-override")
	cfg, err := config.Load(paths, config.Options{SessionsDirOverride: override})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionsDir != override {
		t.Fatalf("got %q", cfg.SessionsDir)
	}
}

func TestEnsureDirs(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{ConfigDir: filepath.Join(dir, "cfg")}
	cfg := config.Config{
		SessionsDir: filepath.Join(dir, "sess"),
		AuthPath:    filepath.Join(dir, "cfg", "auth.json"),
	}
	if err := config.EnsureDirs(cfg, paths); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cfg.SessionsDir, paths.ConfigDir} {
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			t.Fatalf("dir %s: %v", p, err)
		}
	}
}
