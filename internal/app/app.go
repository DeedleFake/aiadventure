// Package app implements the interactive CLI for AI Adventure.
package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"deedles.dev/aiadventure/internal/config"
	"deedles.dev/aiadventure/internal/prompt"
	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

// App is the top-level interactive application.
type App struct {
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
	Cfg    config.Config
	Paths  config.Paths
	Store  *session.Store
	Tokens xai.TokenStore
	OAuth  *xai.OAuthClient
	HTTP   *xai.Client

	scanner *bufio.Scanner
}

// New constructs an App with stdio defaults.
func New(cfg config.Config, paths config.Paths) *App {
	a := &App{
		In:     os.Stdin,
		Out:    os.Stdout,
		Err:    os.Stderr,
		Cfg:    cfg,
		Paths:  paths,
		Store:  session.NewStore(cfg.SessionsDir),
		Tokens: xai.TokenStore{Path: cfg.AuthPath},
		OAuth:  &xai.OAuthClient{},
	}
	a.HTTP = &xai.Client{
		TokenProvider: a.accessToken,
	}
	return a
}

func (a *App) accessToken(ctx context.Context) (string, error) {
	tok, err := xai.EnsureAccessToken(ctx, a.Tokens, a.OAuth)
	if err != nil {
		return "", err
	}
	if tok.APIBase != "" {
		a.HTTP.APIBase = tok.APIBase
	}
	return tok.AccessToken, nil
}

func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

func (a *App) println(args ...any) {
	fmt.Fprintln(a.Out, args...)
}

func (a *App) scan() *bufio.Scanner {
	if a.scanner == nil {
		a.scanner = bufio.NewScanner(a.In)
		// Allow long pasted edits.
		a.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	}
	return a.scanner
}

func (a *App) readLine(prompt string) (string, error) {
	if prompt != "" {
		a.printf("%s", prompt)
	}
	sc := a.scan()
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return sc.Text(), nil
}

// Run is the main menu loop.
func (a *App) Run(ctx context.Context) error {
	a.println("AI Adventure")
	a.println("============")
	a.printf("Sessions: %s\n", a.Cfg.SessionsDir)
	a.showAuthStatus()
	a.printf("Model: %s", a.Cfg.Model)
	if a.Cfg.Effort != "" {
		a.printf(" (effort: %s)", a.Cfg.Effort)
	}
	a.println()
	a.println()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.println("Main menu")
		a.println("  1) Sign in to xAI (OAuth)")
		a.println("  2) Sign out")
		a.println("  3) Select model / effort")
		a.println("  4) New adventure session")
		a.println("  5) List / open sessions")
		a.println("  6) Search sessions")
		a.println("  7) Quit")
		choice, err := a.readLine("> ")
		if err != nil {
			return err
		}
		switch strings.TrimSpace(choice) {
		case "1":
			if err := a.signIn(ctx); err != nil {
				a.printf("Sign-in failed: %v\n", err)
			}
		case "2":
			if err := a.Tokens.Clear(); err != nil {
				a.printf("Sign-out failed: %v\n", err)
			} else {
				a.println("Signed out.")
			}
		case "3":
			if err := a.selectModel(); err != nil {
				a.printf("Model selection failed: %v\n", err)
			}
		case "4":
			if err := a.newSession(ctx); err != nil {
				a.printf("New session failed: %v\n", err)
			}
		case "5":
			if err := a.listOpenSessions(ctx, ""); err != nil {
				a.printf("Sessions failed: %v\n", err)
			}
		case "6":
			q, err := a.readLine("Search query: ")
			if err != nil {
				return err
			}
			if err := a.listOpenSessions(ctx, q); err != nil {
				a.printf("Search failed: %v\n", err)
			}
		case "7", "q", "quit", "exit":
			a.println("Goodbye.")
			return nil
		default:
			a.println("Unknown choice.")
		}
		a.println()
	}
}

func (a *App) showAuthStatus() {
	tok, err := a.Tokens.Load()
	if err != nil {
		a.printf("Auth: error (%v)\n", err)
		return
	}
	if tok.AccessToken == "" && tok.RefreshToken == "" {
		a.println("Auth: not signed in")
		return
	}
	if tok.Valid(time.Now()) {
		a.println("Auth: signed in (token valid)")
		return
	}
	if tok.RefreshToken != "" {
		a.println("Auth: signed in (token may need refresh)")
		return
	}
	a.println("Auth: signed in (token expired; re-auth may be required)")
}

func (a *App) signIn(ctx context.Context) error {
	a.println("Starting xAI device-code OAuth…")
	disc, err := a.OAuth.Discover(ctx)
	if err != nil {
		return err
	}
	deviceURL := disc.DeviceAuthorizationEndpoint
	if deviceURL == "" {
		deviceURL = strings.TrimRight(xai.DefaultIssuer, "/") + "/oauth2/device/code"
	}
	dc, err := a.OAuth.RequestDeviceCode(ctx, deviceURL)
	if err != nil {
		return err
	}
	a.println()
	a.println("To continue:")
	a.printf("  1. Open: %s\n", dc.VerificationURL())
	a.printf("  2. If prompted, enter code: %s\n", dc.UserCode)
	a.println("Waiting for approval…")
	tokens, err := a.OAuth.PollDeviceToken(ctx, disc.TokenEndpoint, dc)
	if err != nil {
		return err
	}
	if err := a.Tokens.Save(tokens); err != nil {
		return err
	}
	a.println("Signed in successfully.")
	return nil
}

func (a *App) selectModel() error {
	a.println("Available models:")
	for i, m := range xai.Catalog {
		effort := ""
		if m.SupportsEffort {
			effort = fmt.Sprintf(" [effort: %s]", strings.Join(m.EffortOptions, "/"))
		}
		a.printf("  %d) %s (%s)%s\n      %s\n", i+1, m.Name, m.ID, effort, m.Description)
	}
	line, err := a.readLine("Select model number: ")
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(xai.Catalog) {
		return fmt.Errorf("invalid selection")
	}
	m := xai.Catalog[n-1]
	a.Cfg.Model = m.ID
	a.Cfg.Effort = ""
	if m.SupportsEffort {
		a.println("Effort levels:")
		for i, e := range m.EffortOptions {
			def := ""
			if e == m.DefaultEffort {
				def = " (default)"
			}
			a.printf("  %d) %s%s\n", i+1, e, def)
		}
		eline, err := a.readLine("Select effort number (empty for default): ")
		if err != nil {
			return err
		}
		eline = strings.TrimSpace(eline)
		if eline == "" {
			a.Cfg.Effort = m.DefaultEffort
		} else {
			en, err := strconv.Atoi(eline)
			if err != nil || en < 1 || en > len(m.EffortOptions) {
				return fmt.Errorf("invalid effort")
			}
			a.Cfg.Effort = m.EffortOptions[en-1]
		}
	}
	if err := config.Save(a.Paths.ConfigPath, a.Cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	a.printf("Selected model %s", a.Cfg.Model)
	if a.Cfg.Effort != "" {
		a.printf(" effort=%s", a.Cfg.Effort)
	}
	a.println()
	return nil
}

func (a *App) newSession(ctx context.Context) error {
	title, err := a.readLine("Session title: ")
	if err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	s := session.New(title, a.Cfg.Model, a.Cfg.Effort)
	if err := a.Store.Save(s); err != nil {
		return err
	}
	a.printf("Created session %s\n", s.ID)
	return a.playSession(ctx, s)
}

func (a *App) listOpenSessions(ctx context.Context, query string) error {
	var (
		list []session.Summary
		err  error
	)
	if strings.TrimSpace(query) == "" {
		list, err = a.Store.List()
	} else {
		list, err = a.Store.Search(query)
	}
	if err != nil {
		return err
	}
	if len(list) == 0 {
		a.println("No sessions found.")
		return nil
	}
	for i, s := range list {
		a.printf("  %d) %s [%s] updated %s\n     id=%s\n", i+1, s.Title, s.Phase, s.UpdatedAt.Local().Format(time.RFC822), s.ID)
	}
	line, err := a.readLine("Open number (empty to cancel): ")
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(list) {
		return fmt.Errorf("invalid selection")
	}
	s, err := a.Store.Load(list[n-1].ID)
	if err != nil {
		return err
	}
	return a.playSession(ctx, s)
}

func (a *App) playSession(ctx context.Context, s *session.Session) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		a.println()
		a.printf("── %s ── phase=%s model=%s", s.Title, s.Phase, s.Model)
		if s.Effort != "" {
			a.printf(" effort=%s", s.Effort)
		}
		a.println()
		a.printTranscript(s)
		a.println("Commands: /help /send /edit /revise /feedback /branch /phase /title /model /menu")
		line, err := a.readLine(fmt.Sprintf("[%s] > ", s.Phase))
		if err != nil {
			_ = a.Store.Save(s)
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			cmd, arg, _ := strings.Cut(line, " ")
			arg = strings.TrimSpace(arg)
			switch strings.ToLower(cmd) {
			case "/help", "/h", "/?":
				a.printPlayHelp()
			case "/menu", "/quit", "/q", "/exit":
				if err := a.Store.Save(s); err != nil {
					return err
				}
				return nil
			case "/phase":
				if err := a.cmdPhase(s, arg); err != nil {
					a.printf("%v\n", err)
				}
			case "/title":
				if arg == "" {
					a.println("Usage: /title <new title>")
					continue
				}
				s.Title = arg
				s.UpdatedAt = time.Now().UTC()
			case "/model":
				if err := a.selectModel(); err != nil {
					a.printf("%v\n", err)
					continue
				}
				s.Model = a.Cfg.Model
				s.Effort = a.Cfg.Effort
			case "/feedback":
				if err := a.cmdFeedback(s, arg); err != nil {
					a.printf("%v\n", err)
				}
			case "/edit":
				if err := a.cmdEdit(s, arg); err != nil {
					a.printf("%v\n", err)
				}
			case "/revise":
				if err := a.cmdRevise(ctx, s, arg); err != nil {
					a.printf("%v\n", err)
				}
			case "/branch":
				if err := a.cmdBranch(s, arg); err != nil {
					a.printf("%v\n", err)
				}
			case "/send":
				// fall through to treat arg as message
				if arg == "" {
					a.println("Usage: /send <message>")
					continue
				}
				if err := a.sendUserMessage(ctx, s, arg); err != nil {
					a.printf("AI error: %v\n", err)
				}
			default:
				a.printf("Unknown command %q (try /help)\n", cmd)
			}
			if err := a.Store.Save(s); err != nil {
				a.printf("save: %v\n", err)
			}
			continue
		}
		if err := a.sendUserMessage(ctx, s, line); err != nil {
			a.printf("AI error: %v\n", err)
		}
		if err := a.Store.Save(s); err != nil {
			a.printf("save: %v\n", err)
		}
	}
}

func (a *App) printPlayHelp() {
	a.println(`Session commands:
  <text>           Send as your message (brainstorm chat or player action)
  /send <text>     Same as plain text
  /phase adventure Begin adventure phase (or /phase brainstorm)
  /edit            Manually edit a prior turn (forks a branch)
  /revise          OOB AI conversation to revise an assistant turn (forks)
  /feedback [text] Add future-only tips for the AI (does not rewrite story)
  /branch          List or switch branch tips
  /title <name>    Rename session
  /model           Change model/effort for this session
  /menu            Save and return to main menu`)
}

func (a *App) printTranscript(s *session.Session) {
	path := s.ActivePath()
	if len(path) == 0 {
		a.println("(no turns yet — describe the adventure you want to create)")
		return
	}
	// Show last few turns fully; summarize older if long.
	start := 0
	if len(path) > 12 {
		start = len(path) - 12
		a.printf("… %d earlier turns omitted …\n", start)
	}
	for i := start; i < len(path); i++ {
		t := path[i]
		who := "You"
		if t.Role == session.RoleAssistant {
			who = "AI"
		}
		a.printf("%s: %s\n\n", who, t.Content)
	}
	if n := len(s.Feedback); n > 0 {
		a.printf("(%d out-of-band feedback note(s) active for future replies)\n", n)
	}
	tips := s.LeafTips()
	if len(tips) > 1 {
		a.printf("(%d branches; active tip %s)\n", len(tips), shortID(s.ActiveTipID))
	}
}

func (a *App) sendUserMessage(ctx context.Context, s *session.Session, text string) error {
	if _, err := s.Append(session.RoleUser, text); err != nil {
		return err
	}
	if err := a.Store.Save(s); err != nil {
		return err
	}
	a.println("…thinking…")
	msgs := prompt.BuildMessages(s)
	req := xai.BuildChatRequest(s.Model, s.Effort, msgs)
	if s.Model == "" {
		req = xai.BuildChatRequest(a.Cfg.Model, a.Cfg.Effort, msgs)
		s.Model = a.Cfg.Model
		s.Effort = a.Cfg.Effort
	}
	resp, err := a.HTTP.Chat(ctx, req)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(resp.AssistantText())
	if content == "" {
		return fmt.Errorf("empty assistant response")
	}
	if _, err := s.Append(session.RoleAssistant, content); err != nil {
		return err
	}
	a.printf("\nAI: %s\n", content)
	return nil
}

func (a *App) cmdPhase(s *session.Session, arg string) error {
	arg = strings.ToLower(strings.TrimSpace(arg))
	switch arg {
	case "adventure", "play", "start":
		return s.SetPhase(session.PhaseAdventure)
	case "brainstorm", "setup":
		return s.SetPhase(session.PhaseBrainstorm)
	case "":
		// toggle
		if s.Phase == session.PhaseBrainstorm {
			return s.SetPhase(session.PhaseAdventure)
		}
		return s.SetPhase(session.PhaseBrainstorm)
	default:
		return fmt.Errorf("usage: /phase [brainstorm|adventure]")
	}
}

func (a *App) cmdFeedback(s *session.Session, arg string) error {
	text := arg
	var err error
	if text == "" {
		text, err = a.readLine("Feedback for future AI replies: ")
		if err != nil {
			return err
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty feedback")
	}
	f := s.AddFeedback(text)
	a.printf("Added feedback %s (story turns unchanged).\n", shortID(f.ID))
	return nil
}

func (a *App) cmdEdit(s *session.Session, arg string) error {
	path := s.ActivePath()
	if len(path) == 0 {
		return fmt.Errorf("no turns to edit")
	}
	a.println("Turns on active branch:")
	for i, t := range path {
		preview := t.Content
		if len(preview) > 60 {
			preview = preview[:60] + "…"
		}
		a.printf("  %d) [%s] %s\n", i+1, t.Role, preview)
	}
	line := arg
	var err error
	if line == "" {
		line, err = a.readLine("Edit turn number: ")
		if err != nil {
			return err
		}
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(path) {
		return fmt.Errorf("invalid turn number")
	}
	target := path[n-1]
	a.printf("Current text:\n%s\n", target.Content)
	a.println("Enter new text (end with a line containing only .)")
	newText, err := a.readMultiline()
	if err != nil {
		return err
	}
	if strings.TrimSpace(newText) == "" {
		return fmt.Errorf("empty replacement")
	}
	id, err := s.EditTurn(target.ID, newText)
	if err != nil {
		return err
	}
	a.printf("Forked branch at tip %s\n", shortID(id))
	return nil
}

func (a *App) cmdRevise(ctx context.Context, s *session.Session, arg string) error {
	path := s.ActivePath()
	var assistants []session.Turn
	for _, t := range path {
		if t.Role == session.RoleAssistant {
			assistants = append(assistants, t)
		}
	}
	if len(assistants) == 0 {
		return fmt.Errorf("no AI turns to revise")
	}
	a.println("AI turns on active branch:")
	for i, t := range assistants {
		preview := t.Content
		if len(preview) > 60 {
			preview = preview[:60] + "…"
		}
		a.printf("  %d) %s\n", i+1, preview)
	}
	line := arg
	var err error
	if line == "" {
		line, err = a.readLine("Revise AI turn number: ")
		if err != nil {
			return err
		}
	}
	// line may be "N instruction..."
	parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 1 || n > len(assistants) {
		return fmt.Errorf("invalid turn number")
	}
	target := assistants[n-1]
	instruction := ""
	if len(parts) > 1 {
		instruction = parts[1]
	}
	if instruction == "" {
		instruction, err = a.readLine("How should the AI change this response? ")
		if err != nil {
			return err
		}
	}
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return fmt.Errorf("empty instruction")
	}
	a.println("…revising…")
	msgs := prompt.BuildRevisionMessages(s, target, instruction)
	req := xai.BuildChatRequest(s.Model, s.Effort, msgs)
	if s.Model == "" {
		req = xai.BuildChatRequest(a.Cfg.Model, a.Cfg.Effort, msgs)
	}
	resp, err := a.HTTP.Chat(ctx, req)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(resp.AssistantText())
	if content == "" {
		return fmt.Errorf("empty revision")
	}
	a.printf("Revised text:\n%s\n", content)
	ok, err := a.readLine("Apply this revision? [Y/n]: ")
	if err != nil {
		return err
	}
	ok = strings.ToLower(strings.TrimSpace(ok))
	if ok == "n" || ok == "no" {
		a.println("Cancelled.")
		return nil
	}
	id, err := s.EditTurn(target.ID, content)
	if err != nil {
		return err
	}
	a.printf("Forked branch at tip %s\n", shortID(id))
	return nil
}

func (a *App) cmdBranch(s *session.Session, arg string) error {
	tips := s.LeafTips()
	if len(tips) == 0 {
		a.println("No branches.")
		return nil
	}
	a.println("Branch tips (most recent activity first):")
	type row struct {
		id   string
		when time.Time
	}
	rows := make([]row, 0, len(tips))
	for _, id := range tips {
		rows = append(rows, row{id: id, when: s.BranchTipActivity(id)})
	}
	// sort newest first
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].when.After(rows[i].when) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
	for i, r := range rows {
		path := s.Path(r.id)
		preview := ""
		if len(path) > 0 {
			preview = path[len(path)-1].Content
			if len(preview) > 50 {
				preview = preview[:50] + "…"
			}
		}
		mark := ""
		if r.id == s.ActiveTipID {
			mark = " *"
		}
		a.printf("  %d) %s depth=%d updated=%s%s\n     %s\n", i+1, shortID(r.id), len(path), r.when.Local().Format(time.RFC822), mark, preview)
	}
	line := arg
	var err error
	if line == "" {
		line, err = a.readLine("Switch to number (empty to cancel): ")
		if err != nil {
			return err
		}
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(rows) {
		return fmt.Errorf("invalid selection")
	}
	return s.SelectTip(rows[n-1].id)
}

func (a *App) readMultiline() (string, error) {
	var lines []string
	for {
		line, err := a.readLine("")
		if err != nil {
			return "", err
		}
		if line == "." {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
