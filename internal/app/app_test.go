package app_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deedles.dev/aiadventure/internal/app"
	"deedles.dev/aiadventure/internal/config"
	"deedles.dev/aiadventure/internal/session"
)

func TestMainMenuQuit(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{
		ConfigDir:   dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
		AuthPath:    filepath.Join(dir, "auth.json"),
	}
	cfg := config.Config{
		SessionsDir: paths.SessionsDir,
		AuthPath:    paths.AuthPath,
		Model:       "grok-4.5",
		Effort:      "high",
	}
	_ = config.EnsureDirs(cfg, paths)

	var out bytes.Buffer
	a := app.New(cfg, paths)
	a.In = strings.NewReader("7\n")
	a.Out = &out
	a.Err = &out

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "AI Adventure") || !strings.Contains(s, "Goodbye") {
		t.Fatalf("output: %s", s)
	}
}

func TestCreateSessionPersistsUnderConfiguredDir(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{
		ConfigDir:   dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
		AuthPath:    filepath.Join(dir, "auth.json"),
	}
	cfg := config.Config{
		SessionsDir: paths.SessionsDir,
		AuthPath:    paths.AuthPath,
		Model:       "grok-4.5",
	}
	_ = config.EnsureDirs(cfg, paths)

	// Script: new session, title, then /menu to exit play without AI call.
	input := "4\nMy Test Quest\n/menu\n7\n"
	var out bytes.Buffer
	a := app.New(cfg, paths)
	a.In = strings.NewReader(input)
	a.Out = &out
	a.Err = &out

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}

	st := session.NewStore(cfg.SessionsDir)
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "My Test Quest" {
		t.Fatalf("list=%+v out=%s", list, out.String())
	}
	loaded, err := st.Load(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Phase != session.PhaseBrainstorm {
		t.Fatalf("phase=%s", loaded.Phase)
	}
}

func TestSearchFromMenu(t *testing.T) {
	dir := t.TempDir()
	paths := config.Paths{
		ConfigDir:   dir,
		ConfigPath:  filepath.Join(dir, "config.json"),
		SessionsDir: filepath.Join(dir, "sessions"),
		AuthPath:    filepath.Join(dir, "auth.json"),
	}
	cfg := config.Config{SessionsDir: paths.SessionsDir, AuthPath: paths.AuthPath, Model: "grok-4.5"}
	_ = config.EnsureDirs(cfg, paths)
	st := session.NewStore(cfg.SessionsDir)
	s := session.New("UniqueZebra Adventure", "grok-4.5", "")
	_, _ = s.Append(session.RoleUser, "hello")
	_ = st.Save(s)

	// search, empty open cancel, quit
	input := "6\nZebra\n\n7\n"
	var out bytes.Buffer
	a := app.New(cfg, paths)
	a.In = strings.NewReader(input)
	a.Out = &out
	a.Err = &out
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "UniqueZebra Adventure") {
		t.Fatalf("search miss: %s", out.String())
	}
}
