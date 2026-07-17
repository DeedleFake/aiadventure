package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

// Model is the Bubble Tea root model for the TUI.
type Model struct {
	deps   *Deps
	ctx    context.Context
	screen Screen
	width  int
	height int
	status string
	errMsg string

	// Focus / modal overlays on play
	focus FocusArea
	modal Modal

	// Model / effort pickers (settings modal)
	modelCursor  int
	effortCursor int
	pendingModel xai.Model

	// Sessions browser
	sessions    []session.Summary
	sessCursor  int
	searchMode  bool
	searchInput textinput.Model
	filterQuery string

	// Play
	session          *session.Session
	sessionPersisted bool // true once written to disk (after first user submit)
	playInput        textinput.Model
	transcript       viewport.Model
	busy             bool
	busyLabel        string
	histCursor       int // selected turn index on ActivePath when FocusHistory

	// Slash palette (when play input starts with /)
	slashMatches []SlashMatch
	slashCursor  int

	// Rename modal
	titleInput textinput.Model

	// Pick turn / text form / branches / revise
	pickTurns     []session.Turn
	pickCursor    int
	pickForRevise bool
	formKind      TextFormKind
	formTarget    session.Turn
	// formArea is multi-line so manual edit preserves newlines (textinput flattens them).
	formArea     textarea.Model
	branches     []branchRow
	branchCursor int
	reviseDraft  string
	reviseTarget session.Turn

	// Auth
	authDeviceURL string
	authUserCode  string
	authWaiting   bool
}

type branchRow struct {
	ID      string
	When    time.Time
	Preview string
	Depth   int
	Active  bool
}

// NewModel constructs the initial TUI model on an empty, unsaved session.
func NewModel(deps *Deps, ctx context.Context) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	si := textinput.New()
	si.Placeholder = "Search sessions…"
	si.CharLimit = 200
	si.Width = 40

	ti := textinput.New()
	ti.Placeholder = "Session title"
	ti.CharLimit = 120
	ti.Width = 40

	pi := textinput.New()
	pi.Placeholder = "Message, or / for commands…  (Tab: history)"
	pi.CharLimit = 4000
	pi.Width = 60

	fa := textarea.New()
	fa.Placeholder = "Enter text… (Ctrl+S to submit)"
	fa.CharLimit = 8000
	fa.SetWidth(60)
	fa.SetHeight(8)
	fa.ShowLineNumbers = false

	vp := viewport.New(80, 20)

	m := Model{
		deps:        deps,
		ctx:         ctx,
		screen:      ScreenPlay,
		focus:       FocusInput,
		modal:       ModalNone,
		width:       80,
		height:      24,
		searchInput: si,
		titleInput:  ti,
		playInput:   pi,
		formArea:    fa,
		transcript:  vp,
	}
	m.startNewSession()
	m.playInput.Focus()
	m.refreshTranscript()
	return m
}

func (m *Model) startNewSession() {
	model, effort := "", ""
	if m.deps != nil {
		model, effort = m.deps.Cfg.Model, m.deps.Cfg.Effort
	}
	m.session = session.New("", model, effort)
	m.sessionPersisted = false
	m.histCursor = 0
	m.focus = FocusInput
	m.modal = ModalNone
	m.playInput.SetValue("")
	m.clearSlashPalette()
}

// Screen returns the active screen (for tests).
func (m Model) Screen() Screen { return m.screen }

// Focus returns the play focus area (for tests).
func (m Model) Focus() FocusArea { return m.focus }

// ModalKind returns the active modal (for tests).
func (m Model) ModalKind() Modal { return m.modal }

// PlayInputValue returns the play prompt text (for tests).
func (m Model) PlayInputValue() string { return m.playInput.Value() }

// Size returns the last WindowSize width and height (for tests).
func (m Model) Size() (width, height int) { return m.width, m.height }

// Busy reports whether an async op is running.
func (m Model) Busy() bool { return m.busy }

// Session returns the open play session, if any.
func (m Model) Session() *session.Session { return m.session }

// SessionPersisted reports whether the current session has been saved to disk.
func (m Model) SessionPersisted() bool { return m.sessionPersisted }

// Sessions returns the browser list (for tests).
func (m Model) Sessions() []session.Summary { return m.sessions }

// FormValue returns the multi-line form buffer (for tests).
func (m Model) FormValue() string { return m.formArea.Value() }

// TranscriptView returns the transcript viewport content (for tests).
func (m Model) TranscriptView() string { return m.transcript.View() }

// HistCursor returns the selected history index (for tests).
func (m Model) HistCursor() int { return m.histCursor }

// SlashMatches returns the current fuzzy command palette (for tests).
func (m Model) SlashMatches() []SlashMatch { return m.slashMatches }

// SlashCursor returns the palette selection index (for tests).
func (m Model) SlashCursor() int { return m.slashCursor }

// SelectedHistoryTurn returns the turn under histCursor, if any.
func (m Model) SelectedHistoryTurn() (session.Turn, bool) {
	if m.session == nil {
		return session.Turn{}, false
	}
	path := m.session.ActivePath()
	if m.histCursor < 0 || m.histCursor >= len(path) {
		return session.Turn{}, false
	}
	return path[m.histCursor], true
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case statusMsg:
		m.status = msg.Text
		return m, nil

	case errMsg:
		m.errMsg = msg.Err.Error()
		m.busy = false
		m.busyLabel = ""
		m.authWaiting = false
		return m, nil

	case deviceCodeStartedMsg:
		m.authDeviceURL = msg.URL
		m.authUserCode = msg.UserCode
		m.authWaiting = true
		m.busy = true
		m.busyLabel = "Waiting for browser approval…"
		m.status = "Complete sign-in in your browser"
		return m, pollAuthCmd(m.ctx, m.deps, msg.TokenEndpoint, msg.Device)

	case authDoneMsg:
		m.busy = false
		m.busyLabel = ""
		m.authWaiting = false
		m.status = "Signed in successfully"
		m.errMsg = ""
		m.screen = ScreenPlay
		m.playInput.Focus()
		return m, nil

	case sessionsLoadedMsg:
		m.sessions = msg.List
		if m.sessCursor >= len(m.sessions) {
			m.sessCursor = max(0, len(m.sessions)-1)
		}
		return m, nil

	case sessionOpenedMsg:
		m.session = msg.Session
		m.sessionPersisted = true
		m.screen = ScreenPlay
		m.focus = FocusInput
		m.modal = ModalNone
		m.playInput.SetValue("")
		m.playInput.Focus()
		m.clearSlashPalette()
		path := m.session.ActivePath()
		if len(path) > 0 {
			m.histCursor = len(path) - 1
		} else {
			m.histCursor = 0
		}
		m.refreshTranscript()
		m.status = fmt.Sprintf("Opened %s", msg.Session.Title)
		return m, nil

	case chatDoneMsg:
		m.busy = false
		m.busyLabel = ""
		// Always refresh: SendUserMessage may already have appended+saved the
		// user turn before the AI call failed; the view must match session/disk.
		if m.session != nil && !m.sessionPersisted {
			// First submit path always saves the user turn before the AI call.
			m.sessionPersisted = true
		}
		m.refreshTranscript()
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else {
			m.errMsg = ""
			m.status = "AI replied"
		}
		m.focus = FocusInput
		m.playInput.Focus()
		return m, nil

	case reviseDraftMsg:
		m.busy = false
		m.busyLabel = ""
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
			m.screen = ScreenPlay
			return m, nil
		}
		m.reviseDraft = msg.Text
		m.reviseTarget = msg.Target
		m.screen = ScreenRevisePreview
		return m, nil
	}

	// Forward to focused inputs when appropriate.
	var cmd tea.Cmd
	switch m.screen {
	case ScreenPlay:
		switch m.modal {
		case ModalSessions:
			if m.searchMode {
				m.searchInput, cmd = m.searchInput.Update(msg)
			}
		case ModalRename:
			m.titleInput, cmd = m.titleInput.Update(msg)
		case ModalNone:
			if m.focus == FocusInput && !m.busy {
				m.playInput, cmd = m.playInput.Update(msg)
				m.syncSlashPalette()
			}
		}
	case ScreenTextForm:
		m.formArea, cmd = m.formArea.Update(msg)
	}
	return m, cmd
}

func (m *Model) layout() {
	header := 6
	footer := 3
	h := max(5, m.height-header-footer)
	w := max(20, m.width-4)
	m.transcript.Width = w
	m.transcript.Height = h
	m.playInput.Width = max(10, w-2)
	m.searchInput.Width = max(10, w/2)
	m.titleInput.Width = max(10, w/2)
	m.formArea.SetWidth(max(10, w-2))
	formH := max(4, min(12, h/2))
	m.formArea.SetHeight(formH)
}

// openForm prepares the multi-line form screen with initial content.
func (m *Model) openForm(kind TextFormKind, initial string) {
	m.formKind = kind
	m.formArea.SetValue(initial)
	m.formArea.Focus()
	m.formArea.CursorEnd()
	m.screen = ScreenTextForm
	m.modal = ModalNone
}

func (m *Model) clearSlashPalette() {
	m.slashMatches = nil
	m.slashCursor = 0
}

func (m *Model) syncSlashPalette() {
	val := m.playInput.Value()
	if !strings.HasPrefix(val, "/") {
		m.clearSlashPalette()
		return
	}
	name, _, _ := ParseSlashInput(val)
	// While typing the command token only, filter on name; if args started, still show match for name.
	m.slashMatches = FuzzyFilterSlash(name)
	if m.slashCursor >= len(m.slashMatches) {
		m.slashCursor = max(0, len(m.slashMatches)-1)
	}
}

func (m *Model) slashPaletteActive() bool {
	return m.screen == ScreenPlay && m.modal == ModalNone && m.focus == FocusInput &&
		strings.HasPrefix(m.playInput.Value(), "/")
}

// saveSession writes the session when it has already been persisted (or force first write).
func (m *Model) saveSession() error {
	if m.session == nil {
		return nil
	}
	if err := m.deps.Store.Save(m.session); err != nil {
		return err
	}
	m.sessionPersisted = true
	return nil
}

// saveSessionIfPersisted saves only when the session is already on disk.
func (m *Model) saveSessionIfPersisted() {
	if m.session == nil || !m.sessionPersisted {
		return
	}
	_ = m.deps.Store.Save(m.session)
}

func (m *Model) refreshTranscript() {
	if m.session == nil {
		m.transcript.SetContent("(no session)")
		return
	}
	var b strings.Builder
	path := m.session.ActivePath()
	if len(path) == 0 {
		b.WriteString("No turns yet.\n\nDescribe the adventure you want to create,\nor type / for commands (e.g. /settings, /phase).\n")
	} else {
		for i, t := range path {
			who := "You"
			if t.Role == session.RoleAssistant {
				who = "AI"
			}
			cursor := "  "
			if m.focus == FocusHistory && i == m.histCursor {
				cursor = "> "
			} else if m.focus == FocusHistory {
				cursor = "  "
			}
			preview := t.Content
			// Keep multi-line content; indent continuation lines lightly in selection mode.
			b.WriteString(cursor)
			b.WriteString(who)
			b.WriteString(": ")
			if m.focus == FocusHistory && i == m.histCursor {
				b.WriteString(selStyle.Render(preview))
			} else {
				b.WriteString(preview)
			}
			b.WriteString("\n\n")
		}
	}
	if n := len(m.session.Feedback); n > 0 {
		fmt.Fprintf(&b, "— %d OOB feedback note(s) for future replies —\n", n)
	}
	if tips := m.session.LeafTips(); len(tips) > 1 {
		fmt.Fprintf(&b, "— %d branches (tip %s) —\n", len(tips), shortID(m.session.ActiveTipID))
	}
	m.transcript.SetContent(b.String())
	if m.focus != FocusHistory {
		m.transcript.GotoBottom()
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// Styles
var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
	itemStyle   = lipgloss.NewStyle().PaddingLeft(2)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	headerStyle = lipgloss.NewStyle().BorderBottom(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(0, 1)
	modalStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212")).Padding(1, 2).Background(lipgloss.Color("235"))
)

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	var body string
	switch m.screen {
	case ScreenAuth:
		body = m.viewAuth()
	case ScreenPlay:
		// List menus (sessions/branch/pick) are modals; body stays the play surface.
		body = m.viewPlay()
	case ScreenTextForm:
		body = m.viewTextForm()
	case ScreenRevisePreview:
		body = m.viewRevisePreview()
	default:
		body = "Unknown screen"
	}

	header := m.viewHeader()
	footer := m.viewFooter()
	status := ""
	if m.errMsg != "" {
		status = errStyle.Render("Error: "+m.errMsg) + "\n"
	} else if m.status != "" {
		status = okStyle.Render(m.status) + "\n"
	}
	if m.busy {
		status += dimStyle.Render("… "+m.busyLabel) + "\n"
	}

	main := lipgloss.JoinVertical(lipgloss.Left, header, status, body, footer)

	if m.screen == ScreenPlay && m.modal != ModalNone {
		return m.renderWithCenteredModal(main)
	}
	return main
}

func (m Model) renderWithCenteredModal(base string) string {
	var content string
	switch m.modal {
	case ModalSettings:
		content = m.viewSettingsModal()
	case ModalEffort:
		content = m.viewEffortModal()
	case ModalRename:
		content = m.viewRenameModal()
	case ModalSessions:
		content = m.viewSessions()
	case ModalPickTurn:
		content = m.viewPickTurn()
	case ModalBranches:
		content = m.viewBranches()
	default:
		return base
	}
	boxW := min(64, max(30, m.width-6))
	// Cap modal height so the composite frame stays within the terminal.
	maxBoxH := max(6, m.height-2)
	rawH := lipgloss.Height(content)
	if rawH > maxBoxH-4 { // account for modal padding/border
		lines := strings.Split(content, "\n")
		limit := max(1, maxBoxH-4)
		if len(lines) > limit {
			lines = lines[:limit]
			lines[limit-1] = dimStyle.Render("…")
			content = strings.Join(lines, "\n")
		}
	}
	box := modalStyle.Width(boxW).Render(content)
	w, h := m.width, m.height
	if w <= 0 {
		w = boxW + 4
	}
	if h <= 0 {
		h = lipgloss.Height(box)
	}
	// Composite: play (base) remains visible under a centered modal.
	// Height is clamped to the terminal so Bubble Tea does not clip the modal away.
	fitted := fitViewToTerminal(base, w, h)
	return placeOverlay(fitted, box, w, h)
}

// fitViewToTerminal pads or clips a multi-line view to exactly width×height cells.
func fitViewToTerminal(s string, width, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if width > 0 {
		for i, line := range lines {
			lw := lipgloss.Width(line)
			if lw > width {
				lines[i] = ansi.Truncate(line, width, "")
			} else if lw < width {
				lines[i] = line + strings.Repeat(" ", width-lw)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// placeOverlay centers fg over bg within a width×height frame, keeping bg cells
// outside the modal. ANSI sequences in bg lines are preserved via ansi.Cut.
func placeOverlay(bg, fg string, width, height int) string {
	if width <= 0 {
		width = max(lipgloss.Width(bg), lipgloss.Width(fg))
	}
	if height <= 0 {
		height = max(lipgloss.Height(bg), lipgloss.Height(fg))
	}

	bgLines := strings.Split(bg, "\n")
	if len(bgLines) > 0 && bgLines[len(bgLines)-1] == "" {
		bgLines = bgLines[:len(bgLines)-1]
	}
	for len(bgLines) < height {
		bgLines = append(bgLines, strings.Repeat(" ", width))
	}
	if len(bgLines) > height {
		bgLines = bgLines[:height]
	}

	fgW := lipgloss.Width(fg)
	fgH := lipgloss.Height(fg)
	if fgW > width {
		fgW = width
	}
	fgLines := strings.Split(fg, "\n")
	if len(fgLines) > 0 && fgLines[len(fgLines)-1] == "" {
		fgLines = fgLines[:len(fgLines)-1]
	}
	if len(fgLines) > height {
		fgLines = fgLines[:height]
		fgH = height
	}

	x := max(0, (width-fgW)/2)
	y := max(0, (height-fgH)/2)

	out := make([]string, height)
	copy(out, bgLines)
	for i, fl := range fgLines {
		row := y + i
		if row < 0 || row >= height {
			continue
		}
		line := bgLines[row]
		// Ensure fg line spans fgW for a solid modal block.
		pad := fgW - lipgloss.Width(fl)
		if pad > 0 {
			fl = fl + strings.Repeat(" ", pad)
		} else if pad < 0 {
			fl = ansi.Truncate(fl, fgW, "")
		}
		left := ansi.Cut(line, 0, x)
		rightStart := x + fgW
		var right string
		if rightStart < lipgloss.Width(line) {
			right = ansi.TruncateLeft(line, rightStart, "")
		}
		// Reset after the modal so residual bg SGR does not bleed into the right strip.
		out[row] = left + fl + "\x1b[0m" + right
	}
	return strings.Join(out, "\n")
}

func (m Model) viewHeader() string {
	auth := "Auth: " + m.deps.AuthStatus()
	model := "Model: " + m.deps.Cfg.Model
	if m.deps.Cfg.Effort != "" {
		model += " / " + m.deps.Cfg.Effort
	}
	left := titleStyle.Render("AI Adventure")
	right := dimStyle.Render(auth + "  ·  " + model)
	line := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)), right)
	return headerStyle.Width(m.width).Render(line)
}

func (m Model) viewFooter() string {
	help := m.footerHelp()
	return footerStyle.Width(m.width).Render(help)
}

func (m Model) footerHelp() string {
	if m.screen == ScreenPlay {
		switch m.modal {
		case ModalSettings, ModalEffort:
			return "↑/↓ move  ·  enter select  ·  esc close"
		case ModalRename:
			return "type title  ·  enter save  ·  esc cancel"
		case ModalSessions:
			if m.searchMode {
				return "type to filter  ·  enter apply  ·  esc cancel search"
			}
			return "↑/↓ move  ·  enter open  ·  / search  ·  n new  ·  esc close"
		case ModalPickTurn, ModalBranches:
			return "↑/↓ move  ·  enter select  ·  esc cancel"
		}
		if m.focus == FocusHistory {
			return "↑/↓ select turn  ·  enter edit  ·  tab input  ·  /cmd  ·  esc input"
		}
		if m.slashPaletteActive() {
			return "↑/↓ commands  ·  enter run  ·  tab history  ·  esc clear"
		}
		return "enter send  ·  / commands  ·  tab history  ·  ctrl+u clear"
	}
	switch m.screen {
	case ScreenAuth:
		return "enter start sign-in  ·  esc back"
	case ScreenTextForm:
		return "type  ·  enter newline  ·  ctrl+s submit  ·  esc cancel"
	case ScreenRevisePreview:
		return "y apply  ·  n discard  ·  esc discard"
	default:
		return "esc back  ·  ctrl+c quit"
	}
}

func (m Model) viewAuth() string {
	var b strings.Builder
	b.WriteString("xAI OAuth\n\n")
	b.WriteString("Status: " + m.deps.AuthStatus() + "\n\n")
	if m.authWaiting {
		b.WriteString("Open this URL in a browser:\n")
		b.WriteString(boxStyle.Render(m.authDeviceURL) + "\n\n")
		b.WriteString("Code: " + titleStyle.Render(m.authUserCode) + "\n")
		b.WriteString(dimStyle.Render("Waiting for approval…") + "\n")
	} else {
		b.WriteString("Press enter to start device-code sign-in.\n")
		b.WriteString(dimStyle.Render("Requires SuperGrok / eligible X Premium account.") + "\n")
	}
	return b.String()
}

func (m Model) viewSettingsModal() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Settings — Select model") + "\n\n")
	for i, mod := range xai.Catalog {
		label := fmt.Sprintf("%s (%s)", mod.Name, mod.ID)
		if mod.SupportsEffort {
			label += "  [effort]"
		}
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.modelCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
		if i == m.modelCursor {
			b.WriteString(dimStyle.Render("    "+mod.Description) + "\n")
		}
	}
	return b.String()
}

func (m Model) viewEffortModal() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Settings — Effort for %s", m.pendingModel.ID)) + "\n\n")
	for i, e := range m.pendingModel.EffortOptions {
		label := e
		if e == m.pendingModel.DefaultEffort {
			label += " (default)"
		}
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.effortCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
	}
	return b.String()
}

func (m Model) viewRenameModal() string {
	return titleStyle.Render("Rename session") + "\n\nTitle: " + m.titleInput.View() + "\n"
}

func (m Model) viewSessions() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Sessions"))
	if m.filterQuery != "" {
		b.WriteString(fmt.Sprintf("  filter=%q", m.filterQuery))
	}
	b.WriteString("\n\n")
	if m.searchMode {
		b.WriteString("Search: " + m.searchInput.View() + "\n\n")
	}
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("No sessions. Press n for a new empty session.") + "\n")
		return b.String()
	}
	for i, s := range m.sessions {
		label := fmt.Sprintf("%s  [%s]  %s", s.Title, s.Phase, s.UpdatedAt.Local().Format("2006-01-02 15:04"))
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.sessCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
	}
	return b.String()
}

func (m Model) viewPlay() string {
	if m.session == nil {
		return "No session open"
	}
	s := m.session
	head := fmt.Sprintf("%s  ·  phase=%s  ·  model=%s", s.Title, s.Phase, s.Model)
	if s.Effort != "" {
		head += " / " + s.Effort
	}
	if !m.sessionPersisted {
		head += "  ·  unsaved"
	}
	focusHint := "input"
	if m.focus == FocusHistory {
		focusHint = "history"
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(head) + "\n")
	b.WriteString(dimStyle.Render("focus: "+focusHint) + "\n")
	b.WriteString(m.transcript.View() + "\n")
	if m.slashPaletteActive() && len(m.slashMatches) > 0 {
		b.WriteString(m.viewSlashPalette() + "\n")
	}
	b.WriteString(boxStyle.Render(m.playInput.View()) + "\n")
	return b.String()
}

func (m Model) viewSlashPalette() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("Commands") + "\n")
	limit := min(8, len(m.slashMatches))
	for i := 0; i < limit; i++ {
		sm := m.slashMatches[i]
		label := "/" + sm.Cmd.Name + "  " + sm.Cmd.Description
		if i == m.slashCursor {
			b.WriteString(selStyle.Render(" "+label+" ") + "\n")
		} else {
			b.WriteString(itemStyle.Render(label) + "\n")
		}
	}
	if len(m.slashMatches) > limit {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more", len(m.slashMatches)-limit)) + "\n")
	}
	return boxStyle.Render(strings.TrimRight(b.String(), "\n"))
}

func (m Model) viewPickTurn() string {
	title := "Select turn to edit"
	if m.pickForRevise {
		title = "Select AI turn to revise"
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	if len(m.pickTurns) == 0 {
		b.WriteString(dimStyle.Render("No turns available.") + "\n")
		return b.String()
	}
	for i, t := range m.pickTurns {
		preview := t.Content
		if len(preview) > 70 {
			preview = preview[:70] + "…"
		}
		label := fmt.Sprintf("[%s] %s", t.Role, preview)
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.pickCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
	}
	return b.String()
}

func (m Model) viewTextForm() string {
	var title string
	switch m.formKind {
	case formFeedback:
		title = "Out-of-band feedback (future replies only)"
	case formEditContent:
		title = "Edit turn content (creates a new branch)"
	case formReviseInstruction:
		title = "How should the AI change this response?"
	}
	return title + "\n\n" + boxStyle.Render(m.formArea.View()) + "\n"
}

func (m Model) viewBranches() string {
	var b strings.Builder
	b.WriteString("Branches (newest first)\n\n")
	if len(m.branches) == 0 {
		b.WriteString(dimStyle.Render("No branches.") + "\n")
		return b.String()
	}
	for i, r := range m.branches {
		mark := ""
		if r.Active {
			mark = " *"
		}
		label := fmt.Sprintf("%s  depth=%d  %s%s", shortID(r.ID), r.Depth, r.When.Local().Format("15:04:05"), mark)
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.branchCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
		b.WriteString(dimStyle.Render("    "+r.Preview) + "\n")
	}
	return b.String()
}

func (m Model) viewRevisePreview() string {
	var b strings.Builder
	b.WriteString("Revised AI text preview\n\n")
	b.WriteString(boxStyle.Width(max(20, m.width-4)).Render(m.reviseDraft) + "\n\n")
	b.WriteString("Apply this revision? (y/n)\n")
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
