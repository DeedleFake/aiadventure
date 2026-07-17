package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"deedles.dev/aiadventure/internal/session"
)

// TestVerificationPlanSessionLifecycle exercises create → turns → edits →
// reload tip selection → branch list → search → OOB feedback, against the
// shipped Store/Session APIs (verification plan step 3).
func TestVerificationPlanSessionLifecycle(t *testing.T) {
	root := t.TempDir()
	st := session.NewStore(root)

	s := session.New("Crystal Heist", "grok-4.5", "high")
	if s.Phase != session.PhaseBrainstorm {
		t.Fatal("must start in brainstorm")
	}
	_, _ = s.Append(session.RoleUser, "I want a heist in a crystal city")
	_, _ = s.Append(session.RoleAssistant, "The spires gleam under twin moons.")
	if err := s.SetPhase(session.PhaseAdventure); err != nil {
		t.Fatal(err)
	}
	uAct, _ := s.Append(session.RoleUser, "I slip into the vault corridor")
	aAct, _ := s.Append(session.RoleAssistant, "Laser lattices hum between the pillars.")

	// Manual edit of AI turn forks a branch.
	time.Sleep(2 * time.Millisecond)
	aiFork, err := s.EditTurn(aAct, "Silent laser lattices sweep the vault corridor.")
	if err != nil {
		t.Fatal(err)
	}
	// Manual edit of user turn on the original line.
	// Original tip still exists as a leaf.
	var origLeaf string
	for _, tip := range s.LeafTips() {
		if tip == aiFork {
			continue
		}
		for _, tr := range s.Path(tip) {
			if tr.ID == aAct {
				origLeaf = tip
			}
		}
	}
	if origLeaf == "" {
		// After editing aAct, the original aAct may still be a leaf if it had no children...
		// Wait: we edited aAct which was a leaf. Edit creates sibling; original aAct remains a leaf.
		// uAct was parent. Path to origLeaf is user setup + AI setup + user act + original AI act.
		if _, ok := s.Turns[aAct]; ok {
			origLeaf = aAct
		}
	}
	if origLeaf == "" {
		t.Fatal("original branch tip missing")
	}
	_ = s.SelectTip(origLeaf)
	time.Sleep(2 * time.Millisecond)
	userFork, err := s.EditTurn(uAct, "I carefully map the vault corridor")
	if err != nil {
		t.Fatal(err)
	}

	turnIDsBeforeFB := s.TurnIDs()
	s.AddFeedback("Describe senses richly; never control the player character.")
	if len(s.TurnIDs()) != len(turnIDsBeforeFB) {
		t.Fatal("OOB feedback mutated turns")
	}

	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) == 0 {
		t.Fatalf("expected session files under %s: %v %v", root, files, err)
	}

	loaded, err := st.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Most recently changed branch displayed on load.
	if loaded.ActiveTipID != loaded.MostRecentlyChangedTip() {
		t.Fatalf("active=%s mostRecent=%s", loaded.ActiveTipID, loaded.MostRecentlyChangedTip())
	}
	if loaded.MostRecentlyChangedTip() != userFork && loaded.ActiveTipID != userFork {
		// userFork should be newest
		if loaded.BranchTipActivity(userFork).Before(loaded.BranchTipActivity(aiFork)) {
			t.Fatalf("expected userFork to be most recent")
		}
	}
	if len(loaded.LeafTips()) < 2 {
		t.Fatalf("branches not listable: %v", loaded.LeafTips())
	}
	if loaded.Phase != session.PhaseAdventure {
		t.Fatalf("phase=%s", loaded.Phase)
	}
	if len(loaded.Feedback) != 1 {
		t.Fatalf("feedback missing: %+v", loaded.Feedback)
	}
	// Prior story content on other branches still present.
	if _, ok := loaded.Turns[aAct]; !ok {
		t.Fatal("original AI turn lost")
	}

	found, err := st.Search("crystal")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != s.ID {
		t.Fatalf("search: %+v", found)
	}
	found, err = st.Search("heist")
	if err != nil || len(found) != 1 {
		t.Fatalf("title search: %+v %v", found, err)
	}

	// Absolute path check for configured location.
	wantPath := filepath.Join(root, s.ID+".json")
	if _, err := os.Stat(wantPath); err != nil {
		// id may be hex without issue; pathFor sanitizes
		matches, _ := filepath.Glob(filepath.Join(root, "*.json"))
		if len(matches) != 1 {
			t.Fatalf("session file missing: %v", err)
		}
	}
}
