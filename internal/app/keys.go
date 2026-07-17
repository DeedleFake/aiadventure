package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit on hub only for plain q; ctrl+c always.
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.screen {
	case ScreenHub:
		return m.keyHub(msg)
	case ScreenAuth:
		return m.keyAuth(msg)
	case ScreenModel:
		return m.keyModel(msg)
	case ScreenEffort:
		return m.keyEffort(msg)
	case ScreenSessions:
		return m.keySessions(msg)
	case ScreenNewSession:
		return m.keyNewSession(msg)
	case ScreenPlay:
		return m.keyPlay(msg)
	case ScreenPlayMenu:
		return m.keyPlayMenu(msg)
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

func (m Model) keyHub(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.hubCursor > 0 {
			m.hubCursor--
		}
	case "down", "j":
		if m.hubCursor < hubItemCount-1 {
			m.hubCursor++
		}
	case "enter":
		return m.hubSelect()
	}
	return m, nil
}

func (m Model) hubSelect() (tea.Model, tea.Cmd) {
	m.errMsg = ""
	switch m.hubCursor {
	case hubSignIn:
		m.screen = ScreenAuth
		m.authWaiting = false
		m.authDeviceURL = ""
		m.authUserCode = ""
		return m, nil
	case hubSignOut:
		if err := m.deps.Tokens.Clear(); err != nil {
			m.errMsg = err.Error()
		} else {
			m.status = "Signed out"
		}
		return m, nil
	case hubModel:
		m.screen = ScreenModel
		m.modelCursor = 0
		for i, mod := range xai.Catalog {
			if mod.ID == m.deps.Cfg.Model {
				m.modelCursor = i
				break
			}
		}
		return m, nil
	case hubNewSession:
		m.screen = ScreenNewSession
		m.titleInput.SetValue("")
		m.titleInput.Focus()
		return m, nil
	case hubSessions:
		m.screen = ScreenSessions
		m.searchMode = false
		m.filterQuery = ""
		m.sessCursor = 0
		return m, loadSessionsCmd(m.deps, "")
	case hubQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) keyAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.authWaiting {
			return m, nil // ignore while waiting
		}
		m.screen = ScreenHub
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

func (m Model) keyModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenHub
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
			m.screen = ScreenEffort
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
				_ = m.deps.Store.Save(m.session)
			}
		}
		m.screen = ScreenHub
	}
	return m, nil
}

func (m Model) keyEffort(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := m.pendingModel.EffortOptions
	switch msg.String() {
	case "esc":
		m.screen = ScreenModel
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
				_ = m.deps.Store.Save(m.session)
			}
		}
		m.screen = ScreenHub
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
		m.screen = ScreenHub
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
		m.screen = ScreenNewSession
		m.titleInput.SetValue("")
		m.titleInput.Focus()
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

func (m Model) keyNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenHub
		m.titleInput.Blur()
		return m, nil
	case "enter":
		title := strings.TrimSpace(m.titleInput.Value())
		s := session.New(title, m.deps.Cfg.Model, m.deps.Cfg.Effort)
		if err := m.deps.Store.Save(s); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.session = s
		m.screen = ScreenPlay
		m.playInput.SetValue("")
		m.playInput.Focus()
		m.refreshTranscript()
		m.status = "Created " + s.Title
		return m, nil
	default:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	}
}

func (m Model) keyPlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		if msg.String() == "esc" {
			// allow leaving? keep session, ignore
		}
		return m, nil
	}
	switch msg.String() {
	case "esc":
		if m.session != nil {
			_ = m.deps.Store.Save(m.session)
		}
		m.screen = ScreenHub
		m.playInput.Blur()
		return m, nil
	case "ctrl+a":
		m.screen = ScreenPlayMenu
		m.playMenuCur = 0
		m.playInput.Blur()
		return m, nil
	case "ctrl+u":
		m.playInput.SetValue("")
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.playInput.Value())
		if text == "" || m.session == nil {
			return m, nil
		}
		m.playInput.SetValue("")
		m.busy = true
		m.busyLabel = "Thinking…"
		m.errMsg = ""
		return m, chatCmd(m.ctx, m.deps, m.session, text)
	case "pgup":
		m.transcript.LineUp(5)
	case "pgdown":
		m.transcript.LineDown(5)
	default:
		var cmd tea.Cmd
		m.playInput, cmd = m.playInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) keyPlayMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = ScreenPlay
		m.playInput.Focus()
	case "up", "k":
		if m.playMenuCur > 0 {
			m.playMenuCur--
		}
	case "down", "j":
		if m.playMenuCur < playActCount-1 {
			m.playMenuCur++
		}
	case "enter":
		return m.playMenuSelect()
	}
	return m, nil
}

func (m Model) playMenuSelect() (tea.Model, tea.Cmd) {
	if m.session == nil {
		m.screen = ScreenHub
		return m, nil
	}
	switch m.playMenuCur {
	case playActPhase:
		if m.session.Phase == session.PhaseBrainstorm {
			_ = m.session.SetPhase(session.PhaseAdventure)
			m.status = "Phase: adventure"
		} else {
			_ = m.session.SetPhase(session.PhaseBrainstorm)
			m.status = "Phase: brainstorm"
		}
		_ = m.deps.Store.Save(m.session)
		m.screen = ScreenPlay
		m.refreshTranscript()
		m.playInput.Focus()
	case playActEdit:
		m.pickTurns = m.session.ActivePath()
		m.pickCursor = max(0, len(m.pickTurns)-1)
		m.pickForRevise = false
		m.screen = ScreenPickTurn
	case playActRevise:
		var asst []session.Turn
		for _, t := range m.session.ActivePath() {
			if t.Role == session.RoleAssistant {
				asst = append(asst, t)
			}
		}
		m.pickTurns = asst
		m.pickCursor = max(0, len(m.pickTurns)-1)
		m.pickForRevise = true
		m.screen = ScreenPickTurn
	case playActFeedback:
		m.openForm(formFeedback, "")
	case playActBranch:
		m.branches = buildBranchRows(m.session)
		m.branchCursor = 0
		m.screen = ScreenBranches
	case playActModel:
		m.screen = ScreenModel
		m.modelCursor = 0
		for i, mod := range xai.Catalog {
			if mod.ID == m.deps.Cfg.Model {
				m.modelCursor = i
				break
			}
		}
	case playActBackHub:
		_ = m.deps.Store.Save(m.session)
		m.screen = ScreenHub
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
			_ = m.deps.Store.Save(m.session)
			m.status = "Feedback added (story unchanged)"
			m.refreshTranscript()
			m.screen = ScreenPlay
			m.playInput.Focus()
		case formEditContent:
			if _, err := m.session.EditTurn(m.formTarget.ID, text); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			_ = m.deps.Store.Save(m.session)
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
		_ = m.deps.Store.Save(m.session)
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
			_ = m.deps.Store.Save(m.session)
			m.status = "Applied AI revision (new branch)"
			m.refreshTranscript()
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
