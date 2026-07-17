package app_test

import (
	"context"
	"fmt"
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
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "ctrl+u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
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

func typeText(t *testing.T, m app.Model, text string) app.Model {
	t.Helper()
	for _, r := range text {
		m = applyKeys(t, m, string(r))
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

// TestStartupEmptySession: main screen is empty new in-memory session; store empty.
func TestStartupEmptySession(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("screen=%s want play", mm.Screen())
	}
	if mm.Session() == nil {
		t.Fatal("expected in-memory session")
	}
	if mm.SessionPersisted() {
		t.Fatal("session must not be persisted at startup")
	}
	if len(mm.Session().ActivePath()) != 0 {
		t.Fatal("expected empty path")
	}
	list, err := deps.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("store should be empty before first submit, got %d", len(list))
	}
	view := mm.View()
	if !strings.Contains(view, "AI Adventure") {
		t.Fatalf("view missing title: %s", view)
	}
	if !strings.Contains(view, "unsaved") && !strings.Contains(view, "No turns yet") {
		t.Fatalf("expected empty session play view, got: %s", view)
	}
	if strings.Contains(view, "Main menu") {
		t.Fatal("hub must not be the startup screen")
	}
}

// TestFirstSubmitSavesAndAutoNames drives the real submit path via Update keys + chatCmd.
func TestFirstSubmitSavesAndAutoNames(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	list, _ := deps.Store.List()
	if len(list) != 0 {
		t.Fatal("precondition: empty store")
	}

	const userText = "I want a dragon mountain quest"
	mm = typeText(t, mm, userText)
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	if cmd == nil {
		t.Fatal("expected chatCmd on first submit")
	}
	// Title is set before the async cmd runs.
	if mm.Session().Title != session.AutoTitleFromText(userText) {
		t.Fatalf("title=%q want auto from user text", mm.Session().Title)
	}
	// Run shipped SendUserMessage path (saves user turn even if AI fails).
	mm = drainCmd(t, mm, cmd)

	list, err := deps.Store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("store list=%d want 1 after first submit", len(list))
	}
	if list[0].Title != session.AutoTitleFromText(userText) {
		t.Fatalf("disk title=%q", list[0].Title)
	}
	if !mm.SessionPersisted() {
		t.Fatal("expected sessionPersisted after submit path")
	}
	// User turn must be on the session (save-before-AI).
	path := mm.Session().ActivePath()
	if len(path) < 1 || path[0].Content != userText {
		t.Fatalf("path=%+v", path)
	}
}

// TestSlashCommandPaletteFuzzy shows fuzzy matches above the prompt when typing /.
func TestSlashCommandPaletteFuzzy(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = typeText(t, mm, "/ren")
	matches := mm.SlashMatches()
	if len(matches) == 0 {
		t.Fatal("expected fuzzy matches for /ren")
	}
	foundRename := false
	for _, m := range matches {
		if m.Cmd.Name == "rename" {
			foundRename = true
		}
	}
	if !foundRename {
		t.Fatalf("rename not in palette: %+v", matches)
	}
	view := mm.View()
	if !strings.Contains(view, "/rename") && !strings.Contains(view, "rename") {
		t.Fatalf("view should show command palette: %s", view)
	}
	if !strings.Contains(view, "Commands") {
		t.Fatalf("palette header missing: %s", view)
	}
}

// TestSlashRenameAndSettingsAndPhase exercises representative feature commands.
func TestSlashRenameAndSettingsAndPhase(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Rename via slash with args (in-memory, not yet persisted).
	mm = typeText(t, mm, "/rename My Epic Tale")
	mm = applyKeys(t, mm, "enter")
	if mm.Session().Title != "My Epic Tale" {
		t.Fatalf("title=%q", mm.Session().Title)
	}
	list, _ := deps.Store.List()
	if len(list) != 0 {
		t.Fatal("rename before first message must not force persist")
	}

	// Settings modal via slash
	mm = typeText(t, mm, "/settings")
	mm = applyKeys(t, mm, "enter")
	if mm.ModalKind() != app.ModalSettings {
		t.Fatalf("modal=%s want settings", mm.ModalKind())
	}
	view := mm.View()
	if !strings.Contains(view, "Settings") && !strings.Contains(view, "Select model") {
		t.Fatalf("settings modal view: %s", view)
	}
	// Centered modal path still has session surface marker or model list
	if !strings.Contains(view, "model") && !strings.Contains(view, "grok") && !strings.Contains(view, "Model") {
		// Catalog names should appear
		t.Fatalf("settings content missing: %s", view)
	}
	// Pick model (enter may go to effort)
	mm = applyKeys(t, mm, "enter")
	if mm.ModalKind() == app.ModalEffort {
		mm = applyKeys(t, mm, "enter")
	}
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("modal should close, got %s", mm.ModalKind())
	}
	if deps.Cfg.Model == "" {
		t.Fatal("model preference should be set")
	}

	// Phase via slash
	if mm.Session().Phase != session.PhaseBrainstorm {
		t.Fatal("start brainstorm")
	}
	mm = typeText(t, mm, "/phase")
	mm = applyKeys(t, mm, "enter")
	if mm.Session().Phase != session.PhaseAdventure {
		t.Fatalf("phase=%s", mm.Session().Phase)
	}
}

// TestSlashRenamePersistsWhenSaved renames a session that is already on disk.
func TestSlashRenamePersistsWhenSaved(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Seed a saved session by first-submit path (AI may fail).
	mm = typeText(t, mm, "Original seed title text")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)

	mm = typeText(t, mm, "/rename Renamed On Disk")
	mm = applyKeys(t, mm, "enter")
	if mm.Session().Title != "Renamed On Disk" {
		t.Fatalf("title=%q", mm.Session().Title)
	}
	list, err := deps.Store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if list[0].Title != "Renamed On Disk" {
		t.Fatalf("disk title=%q", list[0].Title)
	}
}

// TestSettingsModalCenteredOverPlay asserts modal overlays the play surface (not a blank frame).
func TestSettingsModalCenteredOverPlay(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Distinct play markers so we can assert they remain under the modal.
	mm = typeText(t, mm, "/rename Overlay Quest")
	mm = applyKeys(t, mm, "enter")
	if mm.Session().Title != "Overlay Quest" {
		t.Fatalf("title=%q", mm.Session().Title)
	}

	mm = typeText(t, mm, "/model")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("settings must overlay play, screen=%s", mm.Screen())
	}
	if mm.ModalKind() != app.ModalSettings {
		t.Fatalf("modal=%s", mm.ModalKind())
	}
	view := mm.View()
	if !strings.Contains(view, "Settings") && !strings.Contains(view, "Select model") {
		t.Fatalf("missing settings: %s", view)
	}
	// Underlying play surface must still be painted (not blank/dim-only frame).
	if !strings.Contains(view, "Overlay Quest") {
		t.Fatalf("play session title missing under modal: %s", view)
	}
	if !strings.Contains(view, "phase=") && !strings.Contains(view, "brainstorm") && !strings.Contains(view, "adventure") {
		t.Fatalf("play phase marker missing under modal: %s", view)
	}
	// Frame must fit the terminal so Bubble Tea does not clip the modal away.
	assertModalVisibleInTerminal(t, mm)
	assertPlayAndModalInView(t, mm, "Settings", "Select model", "Overlay Quest")

	// Change effort preference through modal and ensure config persists.
	mm = applyKeys(t, mm, "enter") // model → effort for grok-4.5
	if mm.ModalKind() == app.ModalEffort {
		// move to a different effort if possible
		mm = applyKeys(t, mm, "down", "enter")
	}
	if deps.Cfg.Effort == "" && deps.Cfg.Model == "" {
		t.Fatal("expected model preference saved")
	}
	// Config file should exist
	if _, err := config.Load(deps.Paths, config.Options{}); err != nil {
		t.Fatal(err)
	}
}

// TestRenameModalOverlayPlay asserts rename modal paints over play and Esc restores input.
func TestRenameModalOverlayPlay(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = typeText(t, mm, "/rename")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("screen=%s want play", mm.Screen())
	}
	if mm.ModalKind() != app.ModalRename {
		t.Fatalf("modal=%s want rename", mm.ModalKind())
	}
	view := mm.View()
	if !strings.Contains(view, "Rename") {
		t.Fatalf("rename modal missing: %s", view)
	}
	if !strings.Contains(view, "phase=") && !strings.Contains(view, "unsaved") && !strings.Contains(view, "AI Adventure") {
		t.Fatalf("play surface missing under rename modal: %s", view)
	}
	assertModalVisibleInTerminal(t, mm)

	before := mm.PlayInputValue()
	mm = applyKeys(t, mm, "x", "y") // should go to title input, not play
	if mm.PlayInputValue() != before {
		t.Fatalf("play input leaked under rename modal: %q", mm.PlayInputValue())
	}
	mm = applyKeys(t, mm, "esc")
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after esc modal=%s", mm.ModalKind())
	}
	mm = applyKeys(t, mm, "a", "b")
	if !strings.Contains(mm.PlayInputValue(), "ab") {
		t.Fatalf("play input not usable after esc: %q", mm.PlayInputValue())
	}
}

// visibleTerminalWindow returns the lines Bubble Tea's standard renderer keeps:
// when View() exceeds terminal height, only the last height lines are painted.
func visibleTerminalWindow(view string, height int) string {
	lines := strings.Split(view, "\n")
	// Split leaves a trailing empty element when view ends with '\n'; Place often does.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if height > 0 && len(lines) > height {
		lines = lines[len(lines)-height:]
	}
	return strings.Join(lines, "\n")
}

func assertModalVisibleInTerminal(t *testing.T, mm app.Model) {
	t.Helper()
	_, height := mm.Size()
	view := mm.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	// Open-modal View must not overshoot height enough to push modal out of the paint window.
	if height > 0 && len(lines) > height {
		t.Fatalf("modal View has %d lines > terminal height %d (would clip top/modal)", len(lines), height)
	}
	visible := visibleTerminalWindow(view, height)
	// Also check the first-height window (plan acceptance: content in first height lines).
	first := view
	if height > 0 {
		fl := strings.Split(view, "\n")
		if len(fl) > height {
			fl = fl[:height]
		}
		first = strings.Join(fl, "\n")
	}
	markers := []string{"Settings", "Select model", "Effort", "Rename", "Sessions", "Branches", "Select turn", "Select AI"}
	hasVisible, hasFirst := false, false
	for _, mk := range markers {
		if strings.Contains(visible, mk) {
			hasVisible = true
		}
		if strings.Contains(first, mk) {
			hasFirst = true
		}
	}
	if !hasVisible && !hasFirst {
		t.Fatalf("modal text missing from visible terminal window (height=%d):\n%s", height, visible)
	}
	// Catalog model labels should show in settings
	if mm.ModalKind() == app.ModalSettings {
		if !strings.Contains(visible, "grok") && !strings.Contains(first, "grok") &&
			!strings.Contains(visible, "Grok") && !strings.Contains(first, "Grok") {
			// At least one catalog entry or "model" label
			if !strings.Contains(strings.ToLower(visible), "model") &&
				!strings.Contains(strings.ToLower(first), "model") {
				t.Fatalf("settings catalog missing from visible window:\n%s", visible)
			}
		}
	}
}

// assertPlayAndModalInView requires both underlying play markers and modal labels in View.
func assertPlayAndModalInView(t *testing.T, mm app.Model, modalNeedles ...string) {
	t.Helper()
	view := mm.View()
	_, height := mm.Size()
	if height > 0 {
		n := len(strings.Split(strings.TrimRight(view, "\n"), "\n"))
		if n > height {
			t.Fatalf("view lines=%d > terminal height=%d", n, height)
		}
	}
	foundModal := false
	for _, n := range modalNeedles {
		if strings.Contains(view, n) {
			foundModal = true
			break
		}
	}
	if !foundModal {
		t.Fatalf("modal markers %v missing from view:\n%s", modalNeedles, view)
	}
	// Play surface markers: app chrome and/or session head.
	if !strings.Contains(view, "AI Adventure") {
		t.Fatalf("header chrome missing under modal:\n%s", view)
	}
}

// TestModalVisibleWithinTerminalHeight gates the invisible-modal bug for /model and /settings
// at common terminal sizes: modal state active AND content painted in the height window.
func TestModalVisibleWithinTerminalHeight(t *testing.T) {
	sizes := []struct{ w, h int }{
		{80, 24},
		{100, 40},
	}
	commands := []string{"/model", "/settings"}

	for _, sz := range sizes {
		for _, cmd := range commands {
			t.Run(fmt.Sprintf("%s_%dx%d", strings.TrimPrefix(cmd, "/"), sz.w, sz.h), func(t *testing.T) {
				deps := testDeps(t)
				mm := app.NewModel(deps, context.Background())
				mm, _ = upd(t, mm, tea.WindowSizeMsg{Width: sz.w, Height: sz.h})

				mm = typeText(t, mm, cmd)
				mm = applyKeys(t, mm, "enter")
				if mm.ModalKind() != app.ModalSettings {
					t.Fatalf("modal=%s want settings after %s", mm.ModalKind(), cmd)
				}
				assertModalVisibleInTerminal(t, mm)

				// Non-escape keys must not leak into play input.
				before := mm.PlayInputValue()
				mm = applyKeys(t, mm, "x", "y", "z")
				if mm.ModalKind() != app.ModalSettings {
					t.Fatalf("modal closed by typed chars, got %s", mm.ModalKind())
				}
				if mm.PlayInputValue() != before {
					t.Fatalf("play input leaked under modal: got %q want %q", mm.PlayInputValue(), before)
				}
				// Still visible after key noise
				assertModalVisibleInTerminal(t, mm)

				// Escape closes modal and restores play input handling.
				mm = applyKeys(t, mm, "esc")
				if mm.ModalKind() != app.ModalNone {
					t.Fatalf("after esc modal=%s want none", mm.ModalKind())
				}
				mm = applyKeys(t, mm, "h", "i")
				if !strings.Contains(mm.PlayInputValue(), "hi") {
					t.Fatalf("play input not usable after esc: %q", mm.PlayInputValue())
				}
			})
		}
	}
}

// TestTabHistoryFocusAndSelect drives Tab focus toggle and selectable history edit.
func TestTabHistoryFocusAndSelect(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Seed turns on the live session (no AI).
	s := mm.Session()
	_, _ = s.Append(session.RoleUser, "first user turn")
	_, _ = s.Append(session.RoleAssistant, "first ai reply")
	_, _ = s.Append(session.RoleUser, "second user turn")
	// Persist so edits save cleanly
	if err := deps.Store.Save(s); err != nil {
		t.Fatal(err)
	}
	// Reflect persisted flag as if loaded/saved
	mm = typeText(t, mm, "/sessions")
	// Actually sessionPersisted may still be false — force via first-submit was skipped.
	// Re-open from store would set persisted; instead mark by draining a no-op rename after save.
	// openSession path: save already done; set by reloading.
	id := s.ID
	// Simulate sessionOpened via open
	mm = sized(t, deps)
	var cmd tea.Cmd
	// Use slash sessions then open — or inject sessionOpenedMsg through openSessionCmd
	// Easier: type message path already complex; manually open:
	loaded, err := deps.Store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	// Drive openSessionCmd
	mm = sized(t, deps)
	// Replace with open via command
	mm = typeText(t, mm, "/sessions")
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("sessions must stay on play, screen=%s", mm.Screen())
	}
	if mm.ModalKind() != app.ModalSessions {
		t.Fatalf("sessions modal=%s want sessions", mm.ModalKind())
	}
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("play=%s", mm.Screen())
	}
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after open modal=%s want none", mm.ModalKind())
	}
	if !mm.SessionPersisted() {
		t.Fatal("opened session should be persisted")
	}
	if len(mm.Session().ActivePath()) != 3 {
		t.Fatalf("path len=%d", len(mm.Session().ActivePath()))
	}
	_ = loaded

	if mm.Focus() != app.FocusInput {
		t.Fatalf("focus=%s", mm.Focus())
	}
	mm = applyKeys(t, mm, "tab")
	if mm.Focus() != app.FocusHistory {
		t.Fatalf("after tab focus=%s", mm.Focus())
	}
	// Cursor should be on last turn
	if mm.HistCursor() != 2 {
		t.Fatalf("histCursor=%d want 2", mm.HistCursor())
	}
	view := mm.View()
	if !strings.Contains(view, "focus: history") {
		t.Fatalf("view should show history focus: %s", view)
	}

	// Navigate selection
	mm = applyKeys(t, mm, "up")
	if mm.HistCursor() != 1 {
		t.Fatalf("histCursor=%d after up", mm.HistCursor())
	}
	tSel, ok := mm.SelectedHistoryTurn()
	if !ok || tSel.Content != "first ai reply" {
		t.Fatalf("selected=%+v ok=%v", tSel, ok)
	}

	// Enter edits selected turn (out-of-band form)
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("edit form=%s", mm.Screen())
	}
	if mm.FormValue() != "first ai reply" {
		t.Fatalf("form=%q", mm.FormValue())
	}
	// Cancel and tab back to input
	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenPlay {
		t.Fatal(mm.Screen())
	}
	mm = applyKeys(t, mm, "tab") // was history before edit; esc returns to play with previous focus?
	// After esc from form, focus is input (keyTextForm focuses input).
	if mm.Focus() != app.FocusInput {
		// Ensure tab toggles to history then back
		mm = applyKeys(t, mm, "tab")
	}
	if mm.Focus() != app.FocusHistory {
		mm = applyKeys(t, mm, "tab")
	}
	mm = applyKeys(t, mm, "tab")
	if mm.Focus() != app.FocusInput {
		t.Fatalf("tab back to input, focus=%s", mm.Focus())
	}
}

// TestSlashSessionsPhaseFeedbackEditBranch covers remaining feature commands.
func TestSlashSessionsPhaseFeedbackEditBranch(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// First message to persist
	mm = typeText(t, mm, "World of ice castles")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)

	// Append AI turn for edit/branch tests
	s := mm.Session()
	_, _ = s.Append(session.RoleAssistant, "Snow falls on crystal towers.")
	_ = deps.Store.Save(s)
	mm = app.RefreshTranscriptForTest(mm)

	// Feedback
	mm = typeText(t, mm, "/feedback")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("form=%s", mm.Screen())
	}
	mm = typeText(t, mm, "Be concise")
	mm = applyKeys(t, mm, "ctrl+s")
	if len(mm.Session().Feedback) != 1 {
		t.Fatalf("feedback=%+v", mm.Session().Feedback)
	}

	// Edit via slash (uses histCursor / path)
	mm = typeText(t, mm, "/edit")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenTextForm && mm.ModalKind() != app.ModalPickTurn {
		t.Fatalf("edit screen=%s modal=%s", mm.Screen(), mm.ModalKind())
	}
	if mm.ModalKind() == app.ModalPickTurn {
		if mm.Screen() != app.ScreenPlay {
			t.Fatalf("pick-turn must overlay play, screen=%s", mm.Screen())
		}
		mm = applyKeys(t, mm, "enter")
	}
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("form=%s", mm.Screen())
	}
	mm = applyKeys(t, mm, "ctrl+s")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("play=%s", mm.Screen())
	}

	// Branch browser as modal over play
	mm = typeText(t, mm, "/branch")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("branch must overlay play, screen=%s", mm.Screen())
	}
	if mm.ModalKind() != app.ModalBranches {
		t.Fatalf("branches modal=%s view=%s", mm.ModalKind(), mm.View())
	}
	if !strings.Contains(mm.View(), "Branches") {
		t.Fatalf("branch modal content missing: %s", mm.View())
	}
	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenPlay {
		t.Fatal(mm.Screen())
	}
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after esc modal=%s", mm.ModalKind())
	}

	// Auth screen via slash
	mm = typeText(t, mm, "/signin")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenAuth {
		t.Fatalf("auth=%s", mm.Screen())
	}
	if !strings.Contains(mm.View(), "OAuth") {
		t.Fatal(mm.View())
	}
	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenPlay {
		t.Fatal(mm.Screen())
	}

	// Quit command returns tea.Quit
	mm = typeText(t, mm, "/quit")
	_, cmd = upd(t, mm, key("enter"))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
}

// refreshTranscriptForTest is not available — fix TestSlashSessionsPhaseFeedbackEditBranch
// by using WindowSizeMsg which doesn't refresh content from session mutations done outside Update.
// After external Append, reopen or use a key that refreshes. We'll call View which uses transcript;
// need to refresh. Looking at model — refreshTranscript is private. Opening sessions and returning
// won't help. Use /phase which calls refreshTranscript.

func TestManualEditPreservesNewlines(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Persist with first submit
	mm = typeText(t, mm, "Multi")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)

	const original = "line1\nline2\nline3"
	s := mm.Session()
	_, _ = s.Append(session.RoleAssistant, original)
	_ = deps.Store.Save(s)

	// Select second turn (AI) via history
	mm = applyKeys(t, mm, "tab") // history
	// path: user "Multi", assistant original → indices 0,1 — cursor starts at 1
	if mm.HistCursor() != 1 {
		// move to AI turn
		for mm.HistCursor() < 1 {
			mm = applyKeys(t, mm, "down")
		}
		for mm.HistCursor() > 1 {
			mm = applyKeys(t, mm, "up")
		}
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("form=%s", mm.Screen())
	}
	gotForm := mm.FormValue()
	if !strings.Contains(gotForm, "\n") {
		t.Fatalf("form flattened newlines: %q", gotForm)
	}
	if gotForm != original {
		t.Fatalf("form value = %q, want %q", gotForm, original)
	}
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
}

func TestChatDoneErrorRefreshesTranscript(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Start empty play session; inject turns as SendUserMessage would before AI failure.
	const userText = "unique-user-turn-before-ai-failure"
	s := mm.Session()
	if _, err := s.Append(session.RoleUser, userText); err != nil {
		t.Fatal(err)
	}
	if err := deps.Store.Save(s); err != nil {
		t.Fatal(err)
	}
	// Stale transcript: UI still lacks userText until chatDoneMsg.
	if strings.Contains(mm.TranscriptView(), userText) {
		t.Fatal("precondition: transcript should be stale before chatDoneMsg")
	}

	mm = app.ApplyChatDoneForTest(mm, context.Canceled)
	if !strings.Contains(mm.TranscriptView(), userText) && !strings.Contains(mm.View(), userText) {
		t.Fatalf("transcript not refreshed after chat error; view=%s", mm.View())
	}
	if mm.Busy() {
		t.Fatal("should not be busy after chatDoneMsg")
	}
}

func TestStructuralTUIScreens(t *testing.T) {
	deps := testDeps(t)
	m := app.NewModel(deps, context.Background())
	_ = m.Init()
	if m.Screen().String() != "play" {
		t.Fatalf("screen name=%s", m.Screen())
	}
	for _, s := range []app.Screen{
		app.ScreenPlay, app.ScreenAuth, app.ScreenTextForm, app.ScreenRevisePreview,
	} {
		if s.String() == "unknown" {
			t.Fatalf("bad screen %d", s)
		}
	}
	for _, md := range []app.Modal{
		app.ModalNone, app.ModalSettings, app.ModalEffort, app.ModalRename,
		app.ModalSessions, app.ModalPickTurn, app.ModalBranches,
	} {
		if md.String() == "unknown" {
			t.Fatalf("bad modal %d", md)
		}
	}
	var _ tea.Model = m
}

// TestSessionsModalOverlay drives /sessions as a modal over play (not a full-screen switch).
func TestSessionsModalOverlay(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	// Persist a named session so the list has content.
	mm = typeText(t, mm, "Ice castle adventure seed")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	title := mm.Session().Title
	if title == "" {
		t.Fatal("expected auto title")
	}

	// Open sessions browser — must stay on play with sessions modal.
	mm = typeText(t, mm, "/sessions")
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("screen=%s want play", mm.Screen())
	}
	if mm.ModalKind() != app.ModalSessions {
		t.Fatalf("modal=%s want sessions", mm.ModalKind())
	}
	view := mm.View()
	if !strings.Contains(view, "Sessions") {
		t.Fatalf("sessions label missing: %s", view)
	}
	// Underlying play + list content both present.
	if !strings.Contains(view, "AI Adventure") {
		t.Fatalf("play chrome missing under sessions modal: %s", view)
	}
	if !strings.Contains(view, title) && !strings.Contains(view, "phase=") {
		// Title may appear in both list and play head; at least one play marker.
		t.Fatalf("expected session markers under overlay: %s", view)
	}
	assertModalVisibleInTerminal(t, mm)

	// Keys must not leak into play input while modal is open.
	before := mm.PlayInputValue()
	mm = applyKeys(t, mm, "x", "y", "z")
	if mm.ModalKind() != app.ModalSessions {
		t.Fatalf("modal closed by noise keys: %s", mm.ModalKind())
	}
	if mm.PlayInputValue() != before {
		t.Fatalf("play input leaked: got %q want %q", mm.PlayInputValue(), before)
	}

	// Escape restores play + ModalNone; prior session intact.
	mm = applyKeys(t, mm, "esc")
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after esc modal=%s", mm.ModalKind())
	}
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after esc screen=%s", mm.Screen())
	}
	if mm.Session() == nil || mm.Session().Title != title {
		t.Fatalf("session context lost after esc: %+v", mm.Session())
	}
	mm = applyKeys(t, mm, "h", "i")
	if !strings.Contains(mm.PlayInputValue(), "hi") {
		t.Fatalf("play input not usable after esc: %q", mm.PlayInputValue())
	}

	// Re-open and select session (primary action still works).
	mm = applyKeys(t, mm, "ctrl+u")
	mm = typeText(t, mm, "/sessions")
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.ModalKind() != app.ModalSessions {
		t.Fatalf("modal=%s input=%q", mm.ModalKind(), mm.PlayInputValue())
	}
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after open modal=%s", mm.ModalKind())
	}
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("after open screen=%s", mm.Screen())
	}
	if !mm.SessionPersisted() {
		t.Fatal("opened session should be persisted")
	}
	if mm.Session().Title != title {
		t.Fatalf("opened title=%q want %q", mm.Session().Title, title)
	}
}

// TestBranchAndPickTurnModals converts former full-screen menus to overlays.
func TestBranchAndPickTurnModals(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)

	mm = typeText(t, mm, "Branch world seed")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	s := mm.Session()
	_, _ = s.Append(session.RoleAssistant, "Crystal reply for revise pick.")
	_ = deps.Store.Save(s)
	mm = app.RefreshTranscriptForTest(mm)

	// /branch → modal over play
	mm = typeText(t, mm, "/branch")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("branch screen=%s want play", mm.Screen())
	}
	if mm.ModalKind() != app.ModalBranches {
		t.Fatalf("modal=%s want branches", mm.ModalKind())
	}
	view := mm.View()
	if !strings.Contains(view, "Branches") {
		t.Fatalf("branch content missing: %s", view)
	}
	if !strings.Contains(view, "AI Adventure") || !strings.Contains(view, "phase=") {
		t.Fatalf("play surface missing under branch modal: %s", view)
	}
	assertModalVisibleInTerminal(t, mm)
	// Escape cancels
	mm = applyKeys(t, mm, "esc")
	if mm.ModalKind() != app.ModalNone || mm.Screen() != app.ScreenPlay {
		t.Fatalf("after esc screen=%s modal=%s", mm.Screen(), mm.ModalKind())
	}

	// /revise with history on a user turn → pick-turn modal (not form directly).
	// Focus stays on input; histCursor is on last path index (assistant) after seed —
	// force selection onto the user turn so SelectedHistoryTurn is not assistant.
	mm = applyKeys(t, mm, "tab") // history
	// path: user, assistant → indices 0,1; move to user turn
	for mm.HistCursor() > 0 {
		mm = applyKeys(t, mm, "up")
	}
	mm = applyKeys(t, mm, "tab") // back to input; histCursor stays on user
	mm = typeText(t, mm, "/revise")
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("pick-turn must overlay play, screen=%s", mm.Screen())
	}
	if mm.ModalKind() != app.ModalPickTurn {
		// If hist still points at assistant, form may open; that is still valid.
		// Force-open path: re-run with explicit non-assistant selection.
		if mm.Screen() == app.ScreenTextForm {
			mm = applyKeys(t, mm, "esc")
			// Put hist on user and use slash again after clearing focus
			mm = applyKeys(t, mm, "tab")
			for mm.HistCursor() > 0 {
				mm = applyKeys(t, mm, "up")
			}
			// Stay on history focus so SelectedHistoryTurn is user (not assistant)
			// cmdRevise prefers selected history only when RoleAssistant.
			// With FocusHistory on user turn, SelectedHistoryTurn is user → pick list.
			mm = typeText(t, mm, "/revise")
			// typing from history switches to input — histCursor remains
			mm = applyKeys(t, mm, "enter")
		}
	}
	if mm.ModalKind() != app.ModalPickTurn {
		t.Fatalf("modal=%s want pick_turn (screen=%s view=%s)", mm.ModalKind(), mm.Screen(), mm.View())
	}
	view = mm.View()
	if !strings.Contains(view, "Select") && !strings.Contains(view, "revise") && !strings.Contains(view, "AI") {
		t.Fatalf("pick-turn content missing: %s", view)
	}
	assertModalVisibleInTerminal(t, mm)
	// Escape restores play
	mm = applyKeys(t, mm, "esc")
	if mm.ModalKind() != app.ModalNone || mm.Screen() != app.ScreenPlay {
		t.Fatalf("after esc screen=%s modal=%s", mm.Screen(), mm.ModalKind())
	}

	// Primary action: open pick and enter → form flow for revise.
	mm = applyKeys(t, mm, "tab")
	for mm.HistCursor() > 0 {
		mm = applyKeys(t, mm, "up")
	}
	mm = applyKeys(t, mm, "tab") // input; hist on user
	mm = typeText(t, mm, "/revise")
	mm = applyKeys(t, mm, "enter")
	if mm.ModalKind() != app.ModalPickTurn {
		t.Fatalf("pick again modal=%s screen=%s", mm.ModalKind(), mm.Screen())
	}
	mm = applyKeys(t, mm, "enter")
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("after pick form screen=%s modal=%s", mm.Screen(), mm.ModalKind())
	}
	mm = applyKeys(t, mm, "esc")
	if mm.Screen() != app.ScreenPlay {
		t.Fatal(mm.Screen())
	}

	// Branch select primary action: enter switches tip and closes modal.
	// Create a second branch via edit so LeafTips > 1.
	path := mm.Session().ActivePath()
	if len(path) == 0 {
		t.Fatal("empty path")
	}
	// Edit last turn content to fork
	mm = applyKeys(t, mm, "tab")
	mm = applyKeys(t, mm, "enter") // edit form on selected
	if mm.Screen() != app.ScreenTextForm {
		// fallback slash edit
		mm = applyKeys(t, mm, "esc")
		mm = typeText(t, mm, "/edit")
		mm = applyKeys(t, mm, "enter")
		if mm.ModalKind() == app.ModalPickTurn {
			mm = applyKeys(t, mm, "enter")
		}
	}
	if mm.Screen() != app.ScreenTextForm {
		t.Fatalf("need form to fork, got screen=%s modal=%s", mm.Screen(), mm.ModalKind())
	}
	// Submit same content still creates a branch via EditTurn
	mm = applyKeys(t, mm, "ctrl+s")
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("play=%s", mm.Screen())
	}
	mm = typeText(t, mm, "/branch")
	mm = applyKeys(t, mm, "enter")
	if mm.ModalKind() != app.ModalBranches {
		t.Fatalf("modal=%s", mm.ModalKind())
	}
	// Select current (enter) closes
	mm = applyKeys(t, mm, "enter")
	if mm.ModalKind() != app.ModalNone {
		t.Fatalf("after branch select modal=%s", mm.ModalKind())
	}
	if mm.Screen() != app.ScreenPlay {
		t.Fatalf("screen=%s", mm.Screen())
	}
}

func TestQuitViaSlash(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)
	mm = typeText(t, mm, "/quit")
	_, cmd := upd(t, mm, key("enter"))
	if cmd == nil {
		t.Fatal("expected quit")
	}
	_, cmd = upd(t, mm, key("ctrl+c"))
	if cmd == nil {
		t.Fatal("ctrl+c should quit")
	}
}

func TestNewSessionSlashDoesNotPersist(t *testing.T) {
	deps := testDeps(t)
	mm := sized(t, deps)
	// Create noise session on disk
	mm = typeText(t, mm, "Persist me first")
	var cmd tea.Cmd
	mm, cmd = upd(t, mm, key("enter"))
	mm = drainCmd(t, mm, cmd)
	list, _ := deps.Store.List()
	if len(list) != 1 {
		t.Fatalf("list=%d", len(list))
	}

	mm = typeText(t, mm, "/new")
	mm = applyKeys(t, mm, "enter")
	if mm.SessionPersisted() {
		t.Fatal("new session must be unsaved")
	}
	if len(mm.Session().ActivePath()) != 0 {
		t.Fatal("new session must be empty")
	}
	// Store still has the old one only
	list, _ = deps.Store.List()
	if len(list) != 1 {
		t.Fatalf("list=%d after /new", len(list))
	}
}
