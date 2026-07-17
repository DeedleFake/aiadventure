package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpFlag(t *testing.T) {
	exe := buildBinary(t)
	out, err := exec.Command(exe, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("help: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "AI Adventure") {
		t.Fatalf("help missing app name: %s", s)
	}
	if !strings.Contains(s, "sessions-dir") {
		t.Fatalf("help missing sessions-dir: %s", s)
	}
}

func TestVersionFlag(t *testing.T) {
	exe := buildBinary(t)
	out, err := exec.Command(exe, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "aiadventure") {
		t.Fatalf("version: %s", out)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "aiadventure")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = filepath.Join(findModuleRoot(t), "cmd", "aiadventure")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return exe
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Test runs from cmd/aiadventure or module root depending on go test path.
	if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
		return wd
	}
	root := filepath.Join(wd, "../..")
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		return root
	}
	t.Fatalf("cannot find go.mod from %s", wd)
	return ""
}
