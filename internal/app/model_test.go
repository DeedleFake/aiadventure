package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"deedles.dev/aiadventure/internal/app"
	"deedles.dev/aiadventure/internal/config"
	"deedles.dev/aiadventure/internal/session"
)

func testDeps(t *testing.T) *app.Deps {
	t.Helper()
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
	if err := config.EnsureDirs(cfg, paths); err != nil {
		t.Fatal(err)
	}
	return app.NewDeps(cfg, paths)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func upd(t *testing.T, m app.Model, msg tea.Msg) (app.Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(app.Model), cmd
}

func applyKeys(t *testing.T, m app.Model, keys ...string) app.Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = upd(t, m, key(k))
		_ = cmd
	}
	return m
}

func drainCmd(t *testing.T, m app.Model, cmd tea.Cmd) app.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	m, _ = upd(t, m, msg)
	return m
}

func sized(t *testing.T, deps *app.Deps) app.Model {
	t.Helper()
	m := app.NewModel(deps, context.Background())
	m, _ = upd(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return m
}

func TestHubNavigationAndScreens(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	if mm.Screen() != app.ScreenHub {
		t.Fatalf("screen=%s", mm.Screen())
	}
	view := mm.View()
	if !strings.Contains(view, "AI Adventure") || !strings.Contains(view, "Main menu") {
		t.Fatalf("hub view missing title: %s", view)
	}
	if !strings.Contains(view, "Sign in to xAI") {
		t.Fatalf("hub missing sign-in: %s", view)
	}

	mm = applyKeys(t, mm, "down", "down")
	if mm.HubCursor() != 2 {
		t.Fatalf("cursor=%d", mm.HubCursor())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenModel {
		t.Fatalf("want model screen, got %s\n%s", mm.Screen(), mm.View())
	}
	if !strings.Contains(mm.View(), "Select model") {
		t.Fatalf("model view: %s", mm.View())
	}

	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenHub {
		t.Fatalf("esc hub got %s", mm.Screen())
	}

	for mm.HubCursor() > 0 {
		mm = applyKeys(t, mm, "up")
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenAuth {
		t.Fatalf("auth screen=%s", mm.Screen())
	}
	if !strings.Contains(mm.View(), "OAuth") {
		t.Fatalf("auth view: %s", mm.View())
	}
	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenHub {
		t.Fatalf("back hub=%s", mm.Screen())
	}
}

func TestModelEffortSelection(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = applyKeys(t, mm, "down", "down", "enter")
	if mm.Screen() != app.ScreenModel {
		t.Fatal(mm.Screen())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenEffort {
		t.Fatalf("want effort, got %s", mm.Screen())
	}
	if !strings.Contains(mm.View(), "Effort") {
		t.Fatalf("view=%s", mm.View())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenHub {
		t.Fatalf("after effort hub=%s", mm.Screen())
	}
	if deps.Cfg.Model != "grok-4.5" {
		t.Fatalf("model=%s", deps.Cfg.Model)
	}
	if deps.Cfg.Effort == "" {
		t.Fatal("effort should be set")
	}
}

func TestSessionCreateOpenSearchPlayPhaseEditFeedbackBranch(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// New session via hub (cursor 3)
	mm = applyKeys(t, mm, "down", "down", "down", "enter")
	if mm.Screen() != app.ScreenNewSession {
		t.Fatalf("new session screen=%s view=%s", mm.Screen(), mm.View())
	}
	for _, r := range "Dragon Quest" {
		mm = applyKeys(t, mm, string(r))
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("play=%s", mm.Screen())
	}
	if mm.Session() == nil || mm.Session().Title != "Dragon Quest" {
		t.Fatalf("session=%+v", mm.Session())
	}
	if mm.Session().Phase != session.PhaseBrainstorm {
		t.Fatal("expected brainstorm")
	}
	list, err := deps.Store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	s := mm.Session()
	_, _ = s.Append(session.RoleUser, "I want dragons")
	_, _ = s.Append(session.RoleAssistant, "The mountains hold ancient wyrms.")
	_ = deps.Store.Save(s)
	mm, _ = upd(t, mm, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Play menu + phase toggle
	mm = applyKeys(t, mm, "ctrl+a")
	if mm.Screen() != app.ScreenPlayMenu {
		t.Fatalf("play menu=%s", mm.Screen())
	}
	if !strings.Contains(mm.View(), "Toggle phase") {
		t.Fatalf("menu view=%s", mm.View())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after phase play=%s", mm.Screen())
	}
	if mm.Session().Phase != session.PhaseAdventure {
		t.Fatalf("phase=%s", mm.Session().Phase)
	}

	// Feedback (Ctrl+S submits multi-line form)
	mm = applyKeys(t, mm, "ctrl+a", "down", "down", "down", "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("form=%s view=%s", mm.Screen(), mm.View())
	}
	beforeTurns := len(mm.Session().TurnIDs())
	for _, r := range "Be concise" {
		mm = applyKeys(t, mm, string(r))
	}
	mm = applyKeys(t, mm, "ctrl+s")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after feedback=%s", mm.Screen())
	}
	if len(mm.Session().Feedback) != 1 {
		t.Fatalf("feedback=%+v", mm.Session().Feedback)
	}
	if len(mm.Session().TurnIDs()) != beforeTurns {
		t.Fatal("feedback mutated turns")
	}

	// Manual edit
	mm = applyKeys(t, mm, "ctrl+a", "down", "enter")
	if mm.Screen() != app.ScreenPickTurn {
		t.Fatalf("pick=%s", mm.Screen())
	}
	mm = applyKeys(t, mm, "down", "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("edit form=%s", mm.Screen())
	}
	for i := 0; i < 80; i++ {
		mm, _ = upd(t, mm, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "Revised wyrm text" {
		mm = applyKeys(t, mm, string(r))
	}
	mm = applyKeys(t, mm, "ctrl+s")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after edit=%s", mm.Screen())
	}
	if len(mm.Session().LeafTips()) < 2 {
		t.Fatalf("expected branch fork, tips=%v", mm.Session().LeafTips())
	}

	// Branches
	mm = applyKeys(t, mm, "ctrl+a", "down", "down", "down", "down", "enter")
	if mm.Screen() != app.ScreenBranches {
		t.Fatalf("branches=%s view=%s", mm.Screen(), mm.View())
	}
	if !strings.Contains(mm.View(), "Branches") {
		t.Fatalf("branch view=%s", mm.View())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after branch=%s", mm.Screen())
	}

	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenHub {
		t.Fatalf("hub=%s", mm.Screen())
	}

	// Browse sessions
	for mm.HubCursor() < 4 {
		mm = applyKeys(t, mm, "down")
	}
	for mm.HubCursor() > 4 {
		mm = applyKeys(t, mm, "up")
	}
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	if mm.Screen() != app.ScreenSessions {
		t.Fatalf("sessions screen=%s", mm.Screen())
	}
	mm = drainCmd(t, mm, cmd)
	if len(mm.Sessions()) != 1 {
		t.Fatalf("sessions=%d", len(mm.Sessions()))
	}

	// Search
	mm, _ = upd(t, mm, key("/"))
	for _, r := range "Dragon" {
		mm, _ = upd(t, mm, key(string(r)))
	}
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if len(mm.Sessions()) != 1 {
		t.Fatalf("search results=%d", len(mm.Sessions()))
	}

	// Open
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("reopen play=%s", mm.Screen())
	}
	if mm.Session() == nil {
		t.Fatal("nil session")
	}
}

func TestQuitFromHub(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)
	for mm.HubCursor() < 5 {
		mm = applyKeys(t, mm, "down")
	}
	_, cmd := upd(t, mm, key("enter"))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	_, cmd = upd(t, mm, key("q"))
	if cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestViewRendersMainSurfaces(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)
	if !strings.Contains(mm.View(), "Main menu") {
		t.Fatal(mm.View())
	}
	mm = applyKeys(t, mm, "enter")
	if !strings.Contains(mm.View(), "OAuth") {
		t.Fatal(mm.View())
	}
}

func TestStructuralTUINotLineMenus(t *testing.T) {
	deps := testDeps(t)
	m := app.NewModel(deps, context.Background())
	_ = m.Init()
	if m.Screen().String() != "hub" {
		t.Fatalf("screen name=%s", m.Screen())
	}
	for _, s := range []app.Screen{
		app.ScreenHub, app.ScreenSessions, app.ScreenPlay,
		app.ScreenAuth, app.ScreenModel, app.ScreenPlayMenu,
	} {
		if s.String() == "unknown" {
			t.Fatalf("bad screen %d", s)
		}
	}
	// Init implements tea.Model; Update/View are the TUI path.
	var _ tea.Model = m
}

// TestManualEditPreservesNewlines would fail if edit used single-line textinput
// (SetValue flattens "\n" to spaces). Drives the real pick→form→submit path.
func TestManualEditPreservesNewlines(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = applyKeys(t, mm, "down", "down", "down", "enter")
	for _, r := range "Multi" {
		mm = applyKeys(t, mm, string(r))
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatal(mm.Screen())
	}

	const original = "line1\nline2\nline3"
	s := mm.Session()
	_, _ = s.Append(session.RoleUser, "look")
	_, _ = s.Append(session.RoleAssistant, original)
	_ = deps.Store.Save(s)

	// Actions → Edit → select AI turn (second on path: index 1)
	mm = applyKeys(t, mm, "ctrl+a", "down", "enter")
	if mm.Screen() != app.ScreenPickTurn {
		t.Fatalf("pick=%s", mm.Screen())
	}
	mm = applyKeys(t, mm, "down", "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("form=%s", mm.Screen())
	}
	// Form buffer must retain newlines before any edit.
	gotForm := mm.FormValue()
	if !strings.Contains(gotForm, "\n") {
		t.Fatalf("form flattened newlines: %q", gotForm)
	}
	if gotForm != original {
		t.Fatalf("form value = %q, want %q", gotForm, original)
	}

	// Submit unchanged — must not corrupt multi-line content on the new branch tip.
	mm = applyKeys(t, mm, "ctrl+s")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("play=%s", mm.Screen())
	}
	path := mm.Session().ActivePath()
	if len(path) == 0 {
		t.Fatal("empty path")
	}
	tip := path[len(path)-1]
	if tip.Content != original {
		t.Fatalf("after submit content = %q, want original multi-line %q", tip.Content, original)
	}
	if !strings.Contains(tip.Content, "\n") {
		t.Fatal("newlines lost after edit submit")
	}
}

// TestChatDoneErrorRefreshesTranscript drives the shipped chatDoneMsg handler:
// after a user turn is already on the session (as SendUserMessage does before the
// HTTP call), an error result must still refresh the transcript so the UI matches disk.
func TestChatDoneErrorRefreshesTranscript(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = applyKeys(t, mm, "down", "down", "down", "enter")
	for _, r := range "ErrSess" {
		mm = applyKeys(t, mm, string(r))
	}
	mm = applyKeys(t, mm, "enter")

	s := mm.Session()
	// Simulate what SendUserMessage does before a failed AI call.
	const userText = "unique-user-turn-before-ai-failure"
	if _, err := s.Append(session.RoleUser, userText); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.Save(s); err != nil {
		t.Fatal(err)
	}
	// Stale transcript: do not call refreshTranscript; UI still lacks userText.
	if strings.Contains(mm.TranscriptView(), userText) {
		t.Fatal("precondition: transcript should be stale before chatDoneMsg")
	}

	// Inject the real message type via helper that uses the shipped Update path.
	mm = app.ApplyChatDoneForTest(mm, context.Canceled)
	if !strings.Contains(mm.TranscriptView(), userText) && !strings.Contains(mm.View(), userText) {
		t.Fatalf("transcript not refreshed after chat error; view=%s", mm.View())
	}
	if mm.Busy() {
		t.Fatal("should not be busy after chatDoneMsg")
	}
}
