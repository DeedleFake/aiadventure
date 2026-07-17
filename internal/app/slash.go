package app

import (
	"sort"
	"strings"
	"unicode"
)

// SlashCmdID identifies a registered slash command.
type SlashCmdID string

const (
	cmdRename   SlashCmdID = "rename"
	cmdSettings SlashCmdID = "settings"
	cmdSignIn   SlashCmdID = "signin"
	cmdSignOut  SlashCmdID = "signout"
	cmdSessions SlashCmdID = "sessions"
	cmdNew      SlashCmdID = "new"
	cmdPhase    SlashCmdID = "phase"
	cmdEdit     SlashCmdID = "edit"
	cmdRevise   SlashCmdID = "revise"
	cmdFeedback SlashCmdID = "feedback"
	cmdBranch   SlashCmdID = "branch"
	cmdModel    SlashCmdID = "model"
	cmdQuit     SlashCmdID = "quit"
	cmdHelp     SlashCmdID = "help"
)

// SlashCommand is one entry in the command palette.
type SlashCommand struct {
	ID          SlashCmdID
	Name        string   // primary name without leading slash
	Aliases     []string // alternate names without leading slash
	Description string
}

// AllSlashCommands is the full registry of app features reachable via slash.
var AllSlashCommands = []SlashCommand{
	{ID: cmdRename, Name: "rename", Aliases: []string{"title"}, Description: "Rename the current session"},
	{ID: cmdSettings, Name: "settings", Aliases: []string{"prefs", "preferences"}, Description: "Open settings (model / effort)"},
	{ID: cmdModel, Name: "model", Aliases: []string{"effort"}, Description: "Change model / effort (settings)"},
	{ID: cmdSignIn, Name: "signin", Aliases: []string{"login", "auth"}, Description: "Sign in to xAI (OAuth)"},
	{ID: cmdSignOut, Name: "signout", Aliases: []string{"logout"}, Description: "Sign out"},
	{ID: cmdSessions, Name: "sessions", Aliases: []string{"list", "open"}, Description: "Browse / search sessions"},
	{ID: cmdNew, Name: "new", Aliases: []string{"newsession"}, Description: "Start a new empty session"},
	{ID: cmdPhase, Name: "phase", Aliases: []string{"togglephase"}, Description: "Toggle brainstorm ↔ adventure phase"},
	{ID: cmdEdit, Name: "edit", Aliases: []string{"fork"}, Description: "Edit selected (or pick) turn"},
	{ID: cmdRevise, Name: "revise", Aliases: []string{"airevise"}, Description: "Revise selected AI turn with AI"},
	{ID: cmdFeedback, Name: "feedback", Aliases: []string{"oob"}, Description: "Add out-of-band feedback"},
	{ID: cmdBranch, Name: "branch", Aliases: []string{"branches", "tips"}, Description: "Switch branch"},
	{ID: cmdHelp, Name: "help", Aliases: []string{"commands", "?"}, Description: "List slash commands"},
	{ID: cmdQuit, Name: "quit", Aliases: []string{"exit", "q"}, Description: "Quit the application"},
}

// SlashMatch is a fuzzy-ranked command hit.
type SlashMatch struct {
	Cmd   SlashCommand
	Score int
}

// ParseSlashInput splits a leading-slash line into command name and remaining args.
// Returns ok=false when input does not start with '/'.
func ParseSlashInput(input string) (name, args string, ok bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	rest := strings.TrimSpace(input[1:])
	if rest == "" {
		return "", "", true
	}
	// First token is the command name; remainder is args.
	sp := strings.IndexFunc(rest, unicode.IsSpace)
	if sp < 0 {
		return strings.ToLower(rest), "", true
	}
	return strings.ToLower(rest[:sp]), strings.TrimSpace(rest[sp+1:]), true
}

// FuzzyFilterSlash returns commands matching query (name/alias/description), best first.
// Empty query returns all commands in registry order with equal score.
func FuzzyFilterSlash(query string) []SlashMatch {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		out := make([]SlashMatch, len(AllSlashCommands))
		for i, c := range AllSlashCommands {
			out[i] = SlashMatch{Cmd: c, Score: 0}
		}
		return out
	}
	var out []SlashMatch
	for _, c := range AllSlashCommands {
		best := -1
		for _, cand := range slashCandidates(c) {
			if sc, ok := fuzzyScore(query, cand); ok && sc > best {
				best = sc
			}
		}
		// Also allow matching description words lightly.
		if sc, ok := fuzzyScore(query, strings.ToLower(c.Description)); ok && sc/2 > best {
			best = sc / 2
		}
		if best >= 0 {
			out = append(out, SlashMatch{Cmd: c, Score: best})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Cmd.Name < out[j].Cmd.Name
	})
	return out
}

func slashCandidates(c SlashCommand) []string {
	cands := []string{strings.ToLower(c.Name)}
	for _, a := range c.Aliases {
		cands = append(cands, strings.ToLower(a))
	}
	return cands
}

// fuzzyScore returns a higher score for better matches. ok=false if no match.
func fuzzyScore(query, candidate string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if candidate == "" {
		return 0, false
	}
	if candidate == query {
		return 1000, true
	}
	if strings.HasPrefix(candidate, query) {
		return 900 - len(candidate) + len(query), true
	}
	if idx := strings.Index(candidate, query); idx >= 0 {
		return 700 - idx, true
	}
	// Subsequence: all query runes appear in order.
	qi := 0
	gaps := 0
	last := -1
	for i, r := range candidate {
		if qi < len(query) && r == rune(query[qi]) {
			if last >= 0 {
				gaps += i - last - 1
			}
			last = i
			qi++
		}
	}
	if qi < len(query) {
		return 0, false
	}
	return 400 - gaps, true
}

// ResolveSlashCommand finds a command by exact name or alias (case-insensitive).
func ResolveSlashCommand(name string) (SlashCommand, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return SlashCommand{}, false
	}
	for _, c := range AllSlashCommands {
		if c.Name == name {
			return c, true
		}
		for _, a := range c.Aliases {
			if strings.ToLower(a) == name {
				return c, true
			}
		}
	}
	return SlashCommand{}, false
}
