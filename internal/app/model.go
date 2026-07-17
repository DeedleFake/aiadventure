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

	// Hub
	hubCursor int

	// Model / effort pickers
	modelCursor  int
	effortCursor int
	pendingModel xai.Model

	// Sessions browser
	sessions    []session.Summary
	sessCursor  int
	searchMode  bool
	searchInput textinput.Model
	filterQuery string
	titleInput  textinput.Model

	// Play
	session     *session.Session
	playInput   textinput.Model
	transcript  viewport.Model
	playMenuCur int
	busy        bool
	busyLabel   string

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

// NewModel constructs the initial TUI model.
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
	pi.Placeholder = "Type a message and press Enter…"
	pi.CharLimit = 4000
	pi.Width = 60

	fa := textarea.New()
	fa.Placeholder = "Enter text… (Ctrl+S to submit)"
	fa.CharLimit = 8000
	fa.SetWidth(60)
	fa.SetHeight(8)
	fa.ShowLineNumbers = false

	vp := viewport.New(80, 20)

	return Model{
		deps:        deps,
		ctx:         ctx,
		screen:      ScreenHub,
		width:       80,
		height:      24,
		searchInput: si,
		titleInput:  ti,
		playInput:   pi,
		formArea:    fa,
		transcript:  vp,
	}
}

// Screen returns the active screen (for tests).
func (m Model) Screen() Screen { return m.screen }

// HubCursor returns the hub selection index (for tests).
func (m Model) HubCursor() int { return m.hubCursor }

// Busy reports whether an async op is running.
func (m Model) Busy() bool { return m.busy }

// Session returns the open play session, if any.
func (m Model) Session() *session.Session { return m.session }

// Sessions returns the browser list (for tests).
func (m Model) Sessions() []session.Summary { return m.sessions }

// FormValue returns the multi-line form buffer (for tests).
func (m Model) FormValue() string { return m.formArea.Value() }

// TranscriptView returns the transcript viewport content (for tests).
func (m Model) TranscriptView() string { return m.transcript.View() }

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
		return m, nil

	case sessionsLoadedMsg:
		m.sessions = msg.List
		if m.sessCursor >= len(m.sessions) {
			m.sessCursor = max(0, len(m.sessions)-1)
		}
		return m, nil

	case sessionOpenedMsg:
		m.session = msg.Session
		m.screen = ScreenPlay
		m.playInput.SetValue("")
		m.playInput.Focus()
		m.refreshTranscript()
		m.status = fmt.Sprintf("Opened %s", msg.Session.Title)
		return m, nil

	case chatDoneMsg:
		m.busy = false
		m.busyLabel = ""
		// Always refresh: SendUserMessage may already have appended+saved the
		// user turn before the AI call failed; the view must match session/disk.
		m.refreshTranscript()
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		} else {
			m.errMsg = ""
			m.status = "AI replied"
		}
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
	case ScreenSessions:
		if m.searchMode {
			m.searchInput, cmd = m.searchInput.Update(msg)
		}
	case ScreenNewSession:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case ScreenPlay:
		if !m.busy {
			m.playInput, cmd = m.playInput.Update(msg)
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
}

func (m *Model) refreshTranscript() {
	if m.session == nil {
		m.transcript.SetContent("(no session)")
		return
	}
	var b strings.Builder
	path := m.session.ActivePath()
	if len(path) == 0 {
		b.WriteString("No turns yet.\n\nDescribe the adventure you want to create (brainstorm),\nor open the action menu (Ctrl+A) to change phase.\n")
	} else {
		for _, t := range path {
			who := "You"
			if t.Role == session.RoleAssistant {
				who = "AI"
			}
			b.WriteString(who)
			b.WriteString(": ")
			b.WriteString(t.Content)
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
	m.transcript.GotoBottom()
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
)

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	var body string
	switch m.screen {
	case ScreenHub:
		body = m.viewHub()
	case ScreenAuth:
		body = m.viewAuth()
	case ScreenModel:
		body = m.viewModel()
	case ScreenEffort:
		body = m.viewEffort()
	case ScreenSessions:
		body = m.viewSessions()
	case ScreenNewSession:
		body = m.viewNewSession()
	case ScreenPlay:
		body = m.viewPlay()
	case ScreenPlayMenu:
		body = m.viewPlayMenu()
	case ScreenPickTurn:
		body = m.viewPickTurn()
	case ScreenTextForm:
		body = m.viewTextForm()
	case ScreenBranches:
		body = m.viewBranches()
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
	return lipgloss.JoinVertical(lipgloss.Left, header, status, body, footer)
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
	switch m.screen {
	case ScreenHub:
		return "↑/↓ move  ·  enter select  ·  q quit"
	case ScreenAuth:
		return "enter start sign-in  ·  esc back"
	case ScreenModel, ScreenEffort:
		return "↑/↓ move  ·  enter select  ·  esc back"
	case ScreenSessions:
		if m.searchMode {
			return "type to filter  ·  enter apply  ·  esc cancel search"
		}
		return "↑/↓ move  ·  enter open  ·  / search  ·  n new  ·  esc back"
	case ScreenNewSession:
		return "type title  ·  enter create  ·  esc cancel"
	case ScreenPlay:
		return "enter send  ·  ctrl+a actions  ·  ctrl+u clear input  ·  esc hub"
	case ScreenPlayMenu:
		return "↑/↓ move  ·  enter select  ·  esc cancel"
	case ScreenPickTurn, ScreenBranches:
		return "↑/↓ move  ·  enter select  ·  esc cancel"
	case ScreenTextForm:
		return "type  ·  enter newline  ·  ctrl+s submit  ·  esc cancel"
	case ScreenRevisePreview:
		return "y apply  ·  n discard  ·  esc discard"
	default:
		return "esc back  ·  q quit"
	}
}

func (m Model) viewHub() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("Sessions: "+m.deps.Cfg.SessionsDir) + "\n\n")
	b.WriteString("Main menu\n\n")
	for i, label := range hubLabels {
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.hubCursor {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor)
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
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

func (m Model) viewModel() string {
	var b strings.Builder
	b.WriteString("Select model\n\n")
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

func (m Model) viewEffort() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Effort for %s\n\n", m.pendingModel.ID))
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

func (m Model) viewSessions() string {
	var b strings.Builder
	b.WriteString("Sessions")
	if m.filterQuery != "" {
		b.WriteString(fmt.Sprintf("  filter=%q", m.filterQuery))
	}
	b.WriteString("\n\n")
	if m.searchMode {
		b.WriteString("Search: " + m.searchInput.View() + "\n\n")
	}
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("No sessions. Press n to create one.") + "\n")
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

func (m Model) viewNewSession() string {
	return "New adventure session\n\nTitle: " + m.titleInput.View() + "\n"
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
	var b strings.Builder
	b.WriteString(titleStyle.Render(head) + "\n")
	b.WriteString(m.transcript.View() + "\n")
	b.WriteString(boxStyle.Render(m.playInput.View()) + "\n")
	return b.String()
}

func (m Model) viewPlayMenu() string {
	var b strings.Builder
	b.WriteString("Session actions\n\n")
	for i, label := range playActLabels {
		cursor := "  "
		line := itemStyle.Render(label)
		if i == m.playMenuCur {
			cursor = "> "
			line = selStyle.Render(" " + label + " ")
		}
		b.WriteString(cursor + line + "\n")
	}
	return b.String()
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
