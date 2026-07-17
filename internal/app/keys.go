package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Modals on play take priority.
	if m.screen == ScreenPlay && m.modal != ModalNone {
		return m.keyModal(msg)
	}

	switch m.screen {
	case ScreenAuth:
		return m.keyAuth(msg)
	case ScreenSessions:
		return m.keySessions(msg)
	case ScreenPlay:
		return m.keyPlay(msg)
	case ScreenPickTurn:
		return m.keyPickTurn(msg)
	case ScreenTextForm:
		return m.keyTextForm(msg)
	case ScreenBranches:
		return m.keyBranches(msg)
	case ScreenRevisePreview:
		return m.keyRevisePreview(msg)
	}
	return m, nil
}

func (m Model) keyModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case ModalSettings:
		return m.keySettings(msg)
	case ModalEffort:
		return m.keyEffort(msg)
	case ModalRename:
		return m.keyRename(msg)
	}
	return m, nil
}

func (m Model) openSettingsModal() Model {
	m.modal = ModalSettings
	m.modelCursor = 0
	for i, mod := range xai.Catalog {
		if mod.ID == m.deps.Cfg.Model {
			m.modelCursor = i
			break
		}
	}
	m.playInput.Blur()
	return m
}

func (m Model) keySettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = ModalNone
		m.playInput.Focus()
		return m, nil
	case "up", "k":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "down", "j":
		if m.modelCursor < len(xai.Catalog)-1 {
			m.modelCursor++
		}
	case "enter":
		mod := xai.Catalog[m.modelCursor]
		m.pendingModel = mod
		if mod.SupportsEffort {
			m.modal = ModalEffort
			m.effortCursor = 0
			for i, e := range mod.EffortOptions {
				if e == mod.DefaultEffort || e == m.deps.Cfg.Effort {
					m.effortCursor = i
				}
			}
			return m, nil
		}
		if err := m.deps.SaveModelPreference(mod.ID, ""); err != nil {
			m.errMsg = err.Error()
		} else {
			m.status = "Model set to " + mod.ID
			if m.session != nil {
				m.session.Model = mod.ID
				m.session.Effort = ""
				m.saveSessionIfPersisted()
			}
		}
		m.modal = ModalNone
		m.playInput.Focus()
	}
	return m, nil
}

func (m Model) keyEffort(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.pendingModel.EffortOptions
	switch msg.String() {
	case "esc":
		m.modal = ModalSettings
	case "up", "k":
		if m.effortCursor > 0 {
			m.effortCursor--
		}
	case "down", "j":
		if m.effortCursor < len(opts)-1 {
			m.effortCursor++
		}
	case "enter":
		effort := opts[m.effortCursor]
		if err := m.deps.SaveModelPreference(m.pendingModel.ID, effort); err != nil {
			m.errMsg = err.Error()
		} else {
			m.status = fmt.Sprintf("Model %s effort=%s", m.pendingModel.ID, effort)
			if m.session != nil {
				m.session.Model = m.pendingModel.ID
				m.session.Effort = effort
				m.saveSessionIfPersisted()
			}
		}
		m.modal = ModalNone
		m.playInput.Focus()
	}
	return m, nil
}

func (m Model) keyRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = ModalNone
		m.titleInput.Blur()
		m.playInput.Focus()
		return m, nil
	case "enter":
		title := strings.TrimSpace(m.titleInput.Value())
		if title == "" {
			m.errMsg = "title cannot be empty"
			return m, nil
		}
		if m.session == nil {
			m.modal = ModalNone
			return m, nil
		}
		m.session.Title = title
		if m.sessionPersisted {
			if err := m.saveSession(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		}
		m.status = "Renamed to " + title
		m.modal = ModalNone
		m.titleInput.Blur()
		m.playInput.Focus()
		return m, nil
	default:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	}
}

func (m Model) keyAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.authWaiting {
			return m, nil // ignore while waiting
		}
		m.screen = ScreenPlay
		m.playInput.Focus()
		return m, nil
	case "enter":
		if m.authWaiting || m.busy {
			return m, nil
		}
		m.errMsg = ""
		m.busy = true
		m.busyLabel = "Requesting device code…"
		return m, startAuthCmd(m.ctx, m.deps)
	}
	return m, nil
}

func (m Model) keySessions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchMode {
		switch msg.String() {
		case "esc":
			m.searchMode = false
			m.searchInput.Blur()
			return m, nil
		case "enter":
			m.filterQuery = strings.TrimSpace(m.searchInput.Value())
			m.searchMode = false
			m.searchInput.Blur()
			m.sessCursor = 0
			return m, loadSessionsCmd(m.deps, m.filterQuery)
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
	}
	switch msg.String() {
	case "esc":
		m.screen = ScreenPlay
		m.playInput.Focus()
	case "up", "k":
		if m.sessCursor > 0 {
			m.sessCursor--
		}
	case "down", "j":
		if m.sessCursor < len(m.sessions)-1 {
			m.sessCursor++
		}
	case "/":
		m.searchMode = true
		m.searchInput.SetValue(m.filterQuery)
		m.searchInput.Focus()
		return m, nil
	case "n":
		m.startNewSession()
		m.screen = ScreenPlay
		m.playInput.Focus()
		m.refreshTranscript()
		m.status = "New session (unsaved until first message)"
		return m, nil
	case "enter":
		if len(m.sessions) == 0 {
			return m, nil
		}
		id := m.sessions[m.sessCursor].ID
		return m, openSessionCmd(m.deps, id)
	}
	return m, nil
}

func (m Model) keyPlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}

	// History focus navigation
	if m.focus == FocusHistory {
		return m.keyPlayHistory(msg)
	}

	// Slash palette navigation when active
	if m.slashPaletteActive() {
		switch msg.String() {
		case "up":
			if m.slashCursor > 0 {
				m.slashCursor--
			}
			return m, nil
		case "down":
			if m.slashCursor < len(m.slashMatches)-1 {
				m.slashCursor++
			}
			return m, nil
		case "esc":
			m.playInput.SetValue("")
			m.clearSlashPalette()
			return m, nil
		case "enter":
			return m.executeSlashFromPaletteOrInput()
		case "tab":
			return m.toggleFocus()
		}
	}

	switch msg.String() {
	case "tab":
		return m.toggleFocus()
	case "esc":
		// Clear input / palette; stay on play.
		if m.playInput.Value() != "" {
			m.playInput.SetValue("")
			m.clearSlashPalette()
			return m, nil
		}
		return m, nil
	case "ctrl+u":
		m.playInput.SetValue("")
		m.clearSlashPalette()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.playInput.Value())
		if text == "" || m.session == nil {
			return m, nil
		}
		if strings.HasPrefix(text, "/") {
			return m.executeSlashLine(text)
		}
		return m.submitUserMessage(text)
	case "pgup":
		m.transcript.LineUp(5)
	case "pgdown":
		m.transcript.LineDown(5)
	default:
		var cmd tea.Cmd
		m.playInput, cmd = m.playInput.Update(msg)
		m.syncSlashPalette()
		return m, cmd
	}
	return m, nil
}

func (m Model) keyPlayHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pathLen := 0
	if m.session != nil {
		pathLen = len(m.session.ActivePath())
	}
	switch msg.String() {
	case "tab", "esc":
		return m.focusInput()
	case "up", "k":
		if m.histCursor > 0 {
			m.histCursor--
			m.refreshTranscript()
		}
		return m, nil
	case "down", "j":
		if m.histCursor < pathLen-1 {
			m.histCursor++
			m.refreshTranscript()
		}
		return m, nil
	case "enter":
		// Edit selected turn out-of-band (manual fork form).
		t, ok := m.SelectedHistoryTurn()
		if !ok {
			return m, nil
		}
		m.formTarget = t
		m.openForm(formEditContent, t.Content)
		return m, nil
	case "pgup":
		m.transcript.LineUp(5)
	case "pgdown":
		m.transcript.LineDown(5)
	default:
		// Typing switches back to input and applies the key.
		m.focus = FocusInput
		m.playInput.Focus()
		m.refreshTranscript()
		if msg.String() == "/" || msg.Type == tea.KeyRunes {
			var cmd tea.Cmd
			m.playInput, cmd = m.playInput.Update(msg)
			m.syncSlashPalette()
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) toggleFocus() (tea.Model, tea.Cmd) {
	if m.focus == FocusInput {
		return m.focusHistory()
	}
	return m.focusInput()
}

func (m Model) focusHistory() (tea.Model, tea.Cmd) {
	m.focus = FocusHistory
	m.playInput.Blur()
	if m.session != nil {
		path := m.session.ActivePath()
		if len(path) == 0 {
			m.histCursor = 0
		} else if m.histCursor < 0 || m.histCursor >= len(path) {
			m.histCursor = len(path) - 1
		}
	}
	m.refreshTranscript()
	return m, nil
}

func (m Model) focusInput() (tea.Model, tea.Cmd) {
	m.focus = FocusInput
	m.playInput.Focus()
	m.refreshTranscript()
	return m, nil
}

func (m Model) submitUserMessage(text string) (tea.Model, tea.Cmd) {
	// First successful submit path: auto-name and mark for persistence (SendUserMessage saves).
	if !m.sessionPersisted {
		m.session.Title = session.AutoTitleFromText(text)
	}
	m.playInput.SetValue("")
	m.clearSlashPalette()
	m.busy = true
	m.busyLabel = "Thinking…"
	m.errMsg = ""
	// Optimistically mark persisted; chatDoneMsg also ensures this after the save-before-AI path.
	// If Append/Save fails inside chatCmd, session file may be absent — tests assert via Store.List.
	m.sessionPersisted = true
	return m, chatCmd(m.ctx, m.deps, m.session, text)
}

func (m Model) executeSlashFromPaletteOrInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.playInput.Value())
	name, args, ok := ParseSlashInput(text)
	if !ok {
		return m, nil
	}
	// If user has typed a full known command name, use it; else use palette selection.
	if name != "" {
		if _, found := ResolveSlashCommand(name); found {
			return m.executeSlashLine(text)
		}
	}
	if len(m.slashMatches) == 0 {
		m.errMsg = "unknown command"
		return m, nil
	}
	sel := m.slashMatches[m.slashCursor].Cmd
	// Replace input with selected command and run (preserve args if any).
	line := "/" + sel.Name
	if args != "" {
		line += " " + args
	}
	return m.executeSlashLine(line)
}

func (m Model) executeSlashLine(text string) (tea.Model, tea.Cmd) {
	name, args, ok := ParseSlashInput(text)
	if !ok {
		return m, nil
	}
	m.playInput.SetValue("")
	m.clearSlashPalette()

	if name == "" {
		// Bare "/" — show all commands as help status.
		m.status = "Type a command, e.g. /settings, /rename, /sessions"
		m.syncSlashPalette()
		// Re-show palette with empty filter by putting / back? Keep empty; user can type again.
		return m, nil
	}

	cmd, found := ResolveSlashCommand(name)
	if !found {
		// Try fuzzy unique match.
		matches := FuzzyFilterSlash(name)
		if len(matches) == 1 {
			cmd = matches[0].Cmd
			found = true
		} else if len(matches) > 1 && m.slashCursor < len(matches) {
			// Should have been handled by palette; fall through.
		}
	}
	if !found {
		m.errMsg = "unknown command: /" + name
		return m, nil
	}
	return m.runSlashCommand(cmd, args)
}

func (m Model) runSlashCommand(cmd SlashCommand, args string) (tea.Model, tea.Cmd) {
	m.errMsg = ""
	switch cmd.ID {
	case cmdQuit:
		return m, tea.Quit

	case cmdHelp:
		var names []string
		for _, c := range AllSlashCommands {
			names = append(names, "/"+c.Name)
		}
		m.status = "Commands: " + strings.Join(names, " ")
		return m, nil

	case cmdRename:
		if args != "" {
			if m.session == nil {
				return m, nil
			}
			m.session.Title = strings.TrimSpace(args)
			if m.sessionPersisted {
				if err := m.saveSession(); err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
			}
			m.status = "Renamed to " + m.session.Title
			return m, nil
		}
		// Open rename modal
		m.modal = ModalRename
		cur := ""
		if m.session != nil {
			cur = m.session.Title
		}
		m.titleInput.SetValue(cur)
		m.titleInput.Focus()
		m.playInput.Blur()
		return m, nil

	case cmdSettings, cmdModel:
		m = m.openSettingsModal()
		return m, nil

	case cmdSignIn:
		m.screen = ScreenAuth
		m.authWaiting = false
		m.authDeviceURL = ""
		m.authUserCode = ""
		m.playInput.Blur()
		return m, nil

	case cmdSignOut:
		if err := m.deps.Tokens.Clear(); err != nil {
			m.errMsg = err.Error()
		} else {
			m.status = "Signed out"
		}
		return m, nil

	case cmdSessions:
		m.screen = ScreenSessions
		m.searchMode = false
		m.filterQuery = ""
		m.sessCursor = 0
		m.playInput.Blur()
		return m, loadSessionsCmd(m.deps, "")

	case cmdNew:
		m.startNewSession()
		m.playInput.Focus()
		m.refreshTranscript()
		m.status = "New session (unsaved until first message)"
		return m, nil

	case cmdPhase:
		if m.session == nil {
			return m, nil
		}
		if m.session.Phase == session.PhaseBrainstorm {
			_ = m.session.SetPhase(session.PhaseAdventure)
			m.status = "Phase: adventure"
		} else {
			_ = m.session.SetPhase(session.PhaseBrainstorm)
			m.status = "Phase: brainstorm"
		}
		m.saveSessionIfPersisted()
		m.refreshTranscript()
		return m, nil

	case cmdEdit:
		if m.session == nil {
			return m, nil
		}
		// Prefer history selection when set and path non-empty.
		if t, ok := m.SelectedHistoryTurn(); ok && m.focus == FocusHistory {
			m.formTarget = t
			m.openForm(formEditContent, t.Content)
			return m, nil
		}
		// If a history cursor is valid even after focusing input, use it when path non-empty.
		if t, ok := m.SelectedHistoryTurn(); ok && len(m.session.ActivePath()) > 0 {
			// Use selection if user had navigated history; otherwise pick list.
			_ = t
		}
		path := m.session.ActivePath()
		if len(path) == 0 {
			m.errMsg = "no turns to edit"
			return m, nil
		}
		// If histCursor is valid, edit that turn directly for out-of-band ease.
		if m.histCursor >= 0 && m.histCursor < len(path) {
			t := path[m.histCursor]
			m.formTarget = t
			m.openForm(formEditContent, t.Content)
			return m, nil
		}
		m.pickTurns = path
		m.pickCursor = max(0, len(m.pickTurns)-1)
		m.pickForRevise = false
		m.screen = ScreenPickTurn
		m.playInput.Blur()
		return m, nil

	case cmdRevise:
		if m.session == nil {
			return m, nil
		}
		// Prefer selected history turn if it is assistant.
		if t, ok := m.SelectedHistoryTurn(); ok && t.Role == session.RoleAssistant {
			m.formTarget = t
			m.openForm(formReviseInstruction, "")
			return m, nil
		}
		var asst []session.Turn
		for _, t := range m.session.ActivePath() {
			if t.Role == session.RoleAssistant {
				asst = append(asst, t)
			}
		}
		if len(asst) == 0 {
			m.errMsg = "no AI turns to revise"
			return m, nil
		}
		m.pickTurns = asst
		m.pickCursor = max(0, len(m.pickTurns)-1)
		m.pickForRevise = true
		m.screen = ScreenPickTurn
		m.playInput.Blur()
		return m, nil

	case cmdFeedback:
		if m.session == nil {
			return m, nil
		}
		m.openForm(formFeedback, "")
		return m, nil

	case cmdBranch:
		if m.session == nil {
			return m, nil
		}
		m.branches = buildBranchRows(m.session)
		m.branchCursor = 0
		m.screen = ScreenBranches
		m.playInput.Blur()
		return m, nil
	}
	return m, nil
}

func (m Model) keyPickTurn(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenPlay
		m.playInput.Focus()
	case "up", "k":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
	case "down", "j":
		if m.pickCursor < len(m.pickTurns)-1 {
			m.pickCursor++
		}
	case "enter":
		if len(m.pickTurns) == 0 {
			m.screen = ScreenPlay
			return m, nil
		}
		t := m.pickTurns[m.pickCursor]
		m.formTarget = t
		if m.pickForRevise {
			m.openForm(formReviseInstruction, "")
		} else {
			// Preserve full multi-line content; must not round-trip through single-line textinput.
			m.openForm(formEditContent, t.Content)
		}
	}
	return m, nil
}

func (m Model) keyTextForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenPlay
		m.formArea.Blur()
		m.playInput.Focus()
		return m, nil
	case "ctrl+s":
		// Enter inserts newlines in the textarea; Ctrl+S submits.
		text := strings.TrimSpace(m.formArea.Value())
		if text == "" {
			m.errMsg = "empty input"
			return m, nil
		}
		switch m.formKind {
		case formFeedback:
			m.session.AddFeedback(text)
			m.saveSessionIfPersisted()
			// Feedback on never-saved session: keep in memory only until first message.
			m.status = "Feedback added (story unchanged)"
			m.refreshTranscript()
			m.screen = ScreenPlay
			m.playInput.Focus()
		case formEditContent:
			if _, err := m.session.EditTurn(m.formTarget.ID, text); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			if err := m.saveSession(); err != nil {
				// Edit implies content exists; force save so branch is not lost.
				m.errMsg = err.Error()
				return m, nil
			}
			m.status = "Edited turn (new branch)"
			m.refreshTranscript()
			m.screen = ScreenPlay
			m.playInput.Focus()
		case formReviseInstruction:
			m.busy = true
			m.busyLabel = "Revising…"
			m.screen = ScreenPlay
			return m, reviseCmd(m.ctx, m.deps, m.session, m.formTarget, text)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.formArea, cmd = m.formArea.Update(msg)
		return m, cmd
	}
}

func (m Model) keyBranches(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenPlay
		m.playInput.Focus()
	case "up", "k":
		if m.branchCursor > 0 {
			m.branchCursor--
		}
	case "down", "j":
		if m.branchCursor < len(m.branches)-1 {
			m.branchCursor++
		}
	case "enter":
		if len(m.branches) == 0 {
			return m, nil
		}
		id := m.branches[m.branchCursor].ID
		if err := m.session.SelectTip(id); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.saveSessionIfPersisted()
		m.status = "Switched branch " + shortID(id)
		m.refreshTranscript()
		m.screen = ScreenPlay
		m.playInput.Focus()
	}
	return m, nil
}

func (m Model) keyRevisePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		if _, err := m.session.EditTurn(m.reviseTarget.ID, m.reviseDraft); err != nil {
			m.errMsg = err.Error()
		} else {
			if err := m.saveSession(); err != nil {
				m.errMsg = err.Error()
			} else {
				m.status = "Applied AI revision (new branch)"
				m.refreshTranscript()
			}
		}
		m.screen = ScreenPlay
		m.playInput.Focus()
	case "n", "N", "esc":
		m.status = "Revision discarded"
		m.screen = ScreenPlay
		m.playInput.Focus()
	}
	return m, nil
}

func buildBranchRows(s *session.Session) []branchRow {
	tips := s.LeafTips()
	rows := make([]branchRow, 0, len(tips))
	for _, id := range tips {
		path := s.Path(id)
		preview := ""
		if len(path) > 0 {
			preview = path[len(path)-1].Content
			if len(preview) > 60 {
				preview = preview[:60] + "…"
			}
		}
		rows = append(rows, branchRow{
			ID:      id,
			When:    s.BranchTipActivity(id),
			Preview: preview,
			Depth:   len(path),
			Active:  id == s.ActiveTipID,
		})
	}
	// newest first
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].When.After(rows[i].When) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	return rows
}
