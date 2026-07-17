package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

type statusMsg struct{ Text string }
type errMsg struct{ Err error }

type deviceCodeStartedMsg struct {
	URL           string
	UserCode      string
	TokenEndpoint string
	Device        xai.DeviceCodeResponse
}

type authDoneMsg struct{}

type sessionsLoadedMsg struct {
	List []session.Summary
}

type sessionOpenedMsg struct {
	Session *session.Session
}

type chatDoneMsg struct {
	Err error
}

type reviseDraftMsg struct {
	Text   string
	Target session.Turn
	Err    error
}

func startAuthCmd(ctx context.Context, deps *Deps) tea.Cmd {
	return func() tea.Msg {
		disc, err := deps.OAuth.Discover(ctx)
		if err != nil {
			return errMsg{Err: err}
		}
		deviceURL := disc.DeviceAuthorizationEndpoint
		if deviceURL == "" {
			deviceURL = strings.TrimRight(xai.DefaultIssuer, "/") + "/oauth2/device/code"
		}
		dc, err := deps.OAuth.RequestDeviceCode(ctx, deviceURL)
		if err != nil {
			return errMsg{Err: err}
		}
		return deviceCodeStartedMsg{
			URL:           dc.VerificationURL(),
			UserCode:      dc.UserCode,
			TokenEndpoint: disc.TokenEndpoint,
			Device:        dc,
		}
	}
}

func pollAuthCmd(ctx context.Context, deps *Deps, tokenEndpoint string, dc xai.DeviceCodeResponse) tea.Cmd {
	return func() tea.Msg {
		tokens, err := deps.OAuth.PollDeviceToken(ctx, tokenEndpoint, dc)
		if err != nil {
			return errMsg{Err: err}
		}
		if err := deps.Tokens.Save(tokens); err != nil {
			return errMsg{Err: err}
		}
		return authDoneMsg{}
	}
}

func loadSessionsCmd(deps *Deps, query string) tea.Cmd {
	return func() tea.Msg {
		var (
			list []session.Summary
			err  error
		)
		if strings.TrimSpace(query) == "" {
			list, err = deps.Store.List()
		} else {
			list, err = deps.Store.Search(query)
		}
		if err != nil {
			return errMsg{Err: err}
		}
		return sessionsLoadedMsg{List: list}
	}
}

func openSessionCmd(deps *Deps, id string) tea.Cmd {
	return func() tea.Msg {
		s, err := deps.Store.Load(id)
		if err != nil {
			return errMsg{Err: err}
		}
		return sessionOpenedMsg{Session: s}
	}
}

func chatCmd(ctx context.Context, deps *Deps, s *session.Session, text string) tea.Cmd {
	return func() tea.Msg {
		err := deps.SendUserMessage(ctx, s, text)
		return chatDoneMsg{Err: err}
	}
}

func reviseCmd(ctx context.Context, deps *Deps, s *session.Session, target session.Turn, instruction string) tea.Cmd {
	return func() tea.Msg {
		text, err := deps.ReviseAssistantTurn(ctx, s, target, instruction)
		return reviseDraftMsg{Text: text, Target: target, Err: err}
	}
}
