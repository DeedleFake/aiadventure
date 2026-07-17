package session_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"deedles.dev/aiadventure/internal/session"
)

func TestNewSessionBrainstormPhase(t *testing.T) {
	s := session.New("Test world", "grok-4.5", "high")
	if s.Phase != session.PhaseBrainstorm {
		t.Fatalf("phase = %q, want brainstorm", s.Phase)
	}
	if s.Title != "Test world" {
		t.Fatalf("title = %q", s.Title)
	}
	if s.ID == "" {
		t.Fatal("expected non-empty id")
	}
}

func TestAutoTitleFromText(t *testing.T) {
	if got := session.AutoTitleFromText(""); got != session.DefaultTitle {
		t.Fatalf("empty=%q", got)
	}
	if got := session.AutoTitleFromText("  hello world  "); got != "hello world" {
		t.Fatalf("got=%q", got)
	}
	if got := session.AutoTitleFromText("line one\nline two"); got != "line one" {
		t.Fatalf("first line=%q", got)
	}
	long := strings.Repeat("a", 80)
	got := session.AutoTitleFromText(long)
	if len([]rune(got)) > 60 {
		t.Fatalf("too long: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis: %q", got)
	}
}

func TestAppendPathAndPhaseTransition(t *testing.T) {
	s := session.New("Quest", "grok-4.5", "")
	u1, err := s.Append(session.RoleUser, "I want a fantasy setting")
	if err != nil {
		t.Fatal(err)
	}
	a1, err := s.Append(session.RoleAssistant, "How about a coastal kingdom?")
	if err != nil {
		t.Fatal(err)
	}
	path := s.ActivePath()
	if len(path) != 2 {
		t.Fatalf("path len = %d, want 2", len(path))
	}
	if path[0].ID != u1 || path[1].ID != a1 {
		t.Fatalf("path ids = %v, %v", path[0].ID, path[1].ID)
	}
	if err := s.SetPhase(session.PhaseAdventure); err != nil {
		t.Fatal(err)
	}
	if s.Phase != session.PhaseAdventure {
		t.Fatalf("phase = %q", s.Phase)
	}
	_, err = s.Append(session.RoleUser, "I open the wooden door")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ActivePath()) != 3 {
		t.Fatalf("path after adventure turn = %d", len(s.ActivePath()))
	}
}

func TestEditForksBranchAndMostRecentTipOnLoad(t *testing.T) {
	dir := t.TempDir()
	st := session.NewStore(dir)

	s := session.New("Branchy", "grok-4.5", "medium")
	u1, _ := s.Append(session.RoleUser, "Hello")
	a1, _ := s.Append(session.RoleAssistant, "Welcome to the inn")
	u2, _ := s.Append(session.RoleUser, "I order ale")
	_, _ = s.Append(session.RoleAssistant, "The barkeep nods")

	// Edit the first AI message — forks away from later turns.
	time.Sleep(2 * time.Millisecond) // ensure distinct timestamps
	newAI, err := s.EditTurn(a1, "Welcome to the haunted inn")
	if err != nil {
		t.Fatal(err)
	}
	if s.ActiveTipID != newAI {
		t.Fatalf("active tip after edit = %q, want %q", s.ActiveTipID, newAI)
	}
	// Original branch still present.
	if _, ok := s.Turns[u2]; !ok {
		t.Fatal("original child turn should still exist")
	}
	tips := s.LeafTips()
	if len(tips) < 2 {
		t.Fatalf("want at least 2 leaf tips, got %v", tips)
	}
	// Path on new branch is user + revised assistant only.
	path := s.ActivePath()
	if len(path) != 2 || path[0].ID != u1 || path[1].Content != "Welcome to the haunted inn" {
		t.Fatalf("active path after edit: %+v", path)
	}

	// Edit user message on original line: need to select original tip first.
	// Original tip is the last assistant on the first branch.
	var origTip string
	for _, id := range tips {
		if id != newAI {
			// find a tip that still has u2 on path
			for _, t := range s.Path(id) {
				if t.ID == u2 {
					origTip = id
					break
				}
			}
		}
	}
	if origTip == "" {
		t.Fatalf("could not find original branch tip among %v", tips)
	}
	if err := s.SelectTip(origTip); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	newUser, err := s.EditTurn(u2, "I order wine")
	if err != nil {
		t.Fatal(err)
	}
	if s.ActiveTipID != newUser {
		t.Fatalf("tip after user edit = %q", s.ActiveTipID)
	}

	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Load should surface most recently changed tip (user edit).
	if loaded.ActiveTipID != newUser {
		// MostRecentlyChangedTip should prefer newest CreatedAt branch.
		want := loaded.MostRecentlyChangedTip()
		if want != newUser {
			t.Fatalf("most recent tip = %q, want %q (loaded active=%q)", want, newUser, loaded.ActiveTipID)
		}
		if loaded.ActiveTipID != want {
			t.Fatalf("load active tip = %q, want most recent %q", loaded.ActiveTipID, want)
		}
	}

	// All branches still listable.
	if len(loaded.LeafTips()) < 2 {
		t.Fatalf("loaded leaf tips = %v", loaded.LeafTips())
	}
}

func TestOOBFeedbackDoesNotMutateTurns(t *testing.T) {
	s := session.New("FB", "grok-4.5", "")
	_, _ = s.Append(session.RoleUser, "Setup")
	_, _ = s.Append(session.RoleAssistant, "World ready")
	before := s.TurnIDs()
	beforeContents := map[string]string{}
	for id, t := range s.Turns {
		beforeContents[id] = t.Content
	}
	tip := s.ActiveTipID

	f := s.AddFeedback("Prefer terse narration and second person.")
	if f.Content == "" || f.ID == "" {
		t.Fatal("expected feedback id and content")
	}
	after := s.TurnIDs()
	if !slices.Equal(before, after) {
		t.Fatalf("turn ids changed: before=%v after=%v", before, after)
	}
	for id, c := range beforeContents {
		if s.Turns[id].Content != c {
			t.Fatalf("turn %s content mutated", id)
		}
	}
	if s.ActiveTipID != tip {
		t.Fatal("active tip changed after feedback")
	}
	if len(s.Feedback) != 1 {
		t.Fatalf("feedback len = %d", len(s.Feedback))
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := session.NewStore(dir)

	s := session.New("Round trip", "grok-4.5", "low")
	_, _ = s.Append(session.RoleUser, "Brainstorm pirates")
	_, _ = s.Append(session.RoleAssistant, "Caribbean ghost ship?")
	s.AddFeedback("Keep humor light")
	_ = s.SetPhase(session.PhaseAdventure)
	_, _ = s.Append(session.RoleUser, "I climb the rigging")

	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	// Ensure file exists under configured location.
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("session files: %v %v", matches, err)
	}

	loaded, err := st.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != s.Title || loaded.Phase != session.PhaseAdventure {
		t.Fatalf("loaded meta: title=%q phase=%q", loaded.Title, loaded.Phase)
	}
	if len(loaded.ActivePath()) != 3 {
		t.Fatalf("path len = %d", len(loaded.ActivePath()))
	}
	if len(loaded.Feedback) != 1 || loaded.Feedback[0].Content != "Keep humor light" {
		t.Fatalf("feedback = %+v", loaded.Feedback)
	}
	if loaded.Model != "grok-4.5" || loaded.Effort != "low" {
		t.Fatalf("model/effort = %s/%s", loaded.Model, loaded.Effort)
	}
}

func TestSearchByTitleAndContent(t *testing.T) {
	dir := t.TempDir()
	st := session.NewStore(dir)

	a := session.New("Dragon Mountains", "grok-4.5", "")
	_, _ = a.Append(session.RoleUser, "I seek the crystal cave")
	_ = st.Save(a)

	b := session.New("Ocean Voyage", "grok-4.5", "")
	_, _ = b.Append(session.RoleUser, "Sail west at dawn")
	_ = st.Save(b)

	found, err := st.Search("dragon")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != a.ID {
		t.Fatalf("search dragon: %+v", found)
	}

	found, err = st.Search("crystal")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != a.ID {
		t.Fatalf("search crystal: %+v", found)
	}

	found, err = st.Search("sail")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != b.ID {
		t.Fatalf("search sail: %+v", found)
	}

	all, err := st.Search("")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("empty query should list all, got %d", len(all))
	}
}

func TestStoreList(t *testing.T) {
	dir := t.TempDir()
	st := session.NewStore(dir)
	s := session.New("Listed", "m", "")
	if err := st.Save(s); err != nil {
		t.Fatal(err)
	}
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Title != "Listed" {
		t.Fatalf("list = %+v", list)
	}
	// Empty dir.
	empty := session.NewStore(filepath.Join(dir, "missing"))
	list, err = empty.List()
	if err != nil || list != nil && len(list) != 0 {
		t.Fatalf("empty list: %v %v", list, err)
	}
	_ = os.RemoveAll(filepath.Join(dir, "x")) // silence unused if any
}
