package app_test

import (
	"testing"

	"deedles.dev/aiadventure/internal/app"
)

func TestParseSlashInput(t *testing.T) {
	name, args, ok := app.ParseSlashInput("hello")
	if ok {
		t.Fatal("expected not slash")
	}
	name, args, ok = app.ParseSlashInput("/")
	if !ok || name != "" || args != "" {
		t.Fatalf("bare slash: name=%q args=%q ok=%v", name, args, ok)
	}
	name, args, ok = app.ParseSlashInput("/rename Dragon Quest")
	if !ok || name != "rename" || args != "Dragon Quest" {
		t.Fatalf("got name=%q args=%q", name, args)
	}
	name, args, ok = app.ParseSlashInput("/PHASE")
	if !ok || name != "phase" || args != "" {
		t.Fatalf("got name=%q args=%q", name, args)
	}
}

func TestFuzzyFilterSlash(t *testing.T) {
	all := app.FuzzyFilterSlash("")
	if len(all) != len(app.AllSlashCommands) {
		t.Fatalf("empty query should return all: %d vs %d", len(all), len(app.AllSlashCommands))
	}
	matches := app.FuzzyFilterSlash("ren")
	if len(matches) == 0 {
		t.Fatal("expected rename match")
	}
	if matches[0].Cmd.Name != "rename" && matches[0].Cmd.ID != app.AllSlashCommands[0].ID {
		// rename should rank highly for "ren"
		found := false
		for _, m := range matches {
			if m.Cmd.Name == "rename" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("rename not in matches for ren: %+v", matches)
		}
	}
	// settings via partial
	sm := app.FuzzyFilterSlash("set")
	found := false
	for _, m := range sm {
		if m.Cmd.Name == "settings" {
			found = true
		}
	}
	if !found {
		t.Fatal("settings not matched by set")
	}
}

func TestResolveSlashCommand(t *testing.T) {
	c, ok := app.ResolveSlashCommand("rename")
	if !ok || c.Name != "rename" {
		t.Fatal(c, ok)
	}
	c, ok = app.ResolveSlashCommand("title") // alias
	if !ok || c.ID != "rename" {
		t.Fatalf("alias: %+v ok=%v", c, ok)
	}
	_, ok = app.ResolveSlashCommand("nope")
	if ok {
		t.Fatal("expected miss")
	}
}

func TestAllFeaturesRegistered(t *testing.T) {
	// Every app feature from the goal must be reachable via slash registry.
	need := []string{
		"rename", "settings", "signin", "signout", "sessions", "new",
		"phase", "edit", "revise", "feedback", "branch", "model", "quit", "help",
	}
	have := map[string]bool{}
	for _, c := range app.AllSlashCommands {
		have[c.Name] = true
	}
	for _, n := range need {
		if !have[n] {
			t.Errorf("missing slash command %q", n)
		}
	}
}
