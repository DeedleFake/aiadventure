// Package app implements the Bubble Tea TUI for AI Adventure.
package app

import (
	"context"
	"fmt"
	"time"

	"deedles.dev/aiadventure/internal/config"
	"deedles.dev/aiadventure/internal/prompt"
	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

// Deps holds pure domain services used by the TUI (no terminal I/O).
type Deps struct {
	Cfg    config.Config
	Paths  config.Paths
	Store  *session.Store
	Tokens xai.TokenStore
	OAuth  *xai.OAuthClient
	HTTP   *xai.Client
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

// NewDeps builds dependencies from config paths.
func NewDeps(cfg config.Config, paths config.Paths) *Deps {
	d := &Deps{
		Cfg:    cfg,
		Paths:  paths,
		Store:  session.NewStore(cfg.SessionsDir),
		Tokens: xai.TokenStore{Path: cfg.AuthPath},
		OAuth:  &xai.OAuthClient{},
		Now:    time.Now,
	}
	d.HTTP = &xai.Client{
		TokenProvider: d.accessToken,
	}
	return d
}

func (d *Deps) accessToken(ctx context.Context) (string, error) {
	tok, err := xai.EnsureAccessToken(ctx, d.Tokens, d.OAuth)
	if err != nil {
		return "", err
	}
	if tok.APIBase != "" {
		d.HTTP.APIBase = tok.APIBase
	}
	return tok.AccessToken, nil
}

func (d *Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// AuthStatus is a short label for the hub header.
func (d *Deps) AuthStatus() string {
	tok, err := d.Tokens.Load()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	if tok.AccessToken == "" && tok.RefreshToken == "" {
		return "not signed in"
	}
	if tok.Valid(d.now()) {
		return "signed in"
	}
	if tok.RefreshToken != "" {
		return "signed in (refresh pending)"
	}
	return "signed in (expired)"
}

// SendUserMessage appends a user turn, calls the AI, and appends the reply.
func (d *Deps) SendUserMessage(ctx context.Context, s *session.Session, text string) error {
	if _, err := s.Append(session.RoleUser, text); err != nil {
		return err
	}
	if err := d.Store.Save(s); err != nil {
		return err
	}
	msgs := prompt.BuildMessages(s)
	model, effort := s.Model, s.Effort
	if model == "" {
		model, effort = d.Cfg.Model, d.Cfg.Effort
		s.Model, s.Effort = model, effort
	}
	req := xai.BuildChatRequest(model, effort, msgs)
	resp, err := d.HTTP.Chat(ctx, req)
	if err != nil {
		return err
	}
	content := trimSpace(resp.AssistantText())
	if content == "" {
		return fmt.Errorf("empty assistant response")
	}
	if _, err := s.Append(session.RoleAssistant, content); err != nil {
		return err
	}
	return d.Store.Save(s)
}

// ReviseAssistantTurn asks the AI to rewrite an assistant turn and returns the draft text.
func (d *Deps) ReviseAssistantTurn(ctx context.Context, s *session.Session, target session.Turn, instruction string) (string, error) {
	msgs := prompt.BuildRevisionMessages(s, target, instruction)
	model, effort := s.Model, s.Effort
	if model == "" {
		model, effort = d.Cfg.Model, d.Cfg.Effort
	}
	req := xai.BuildChatRequest(model, effort, msgs)
	resp, err := d.HTTP.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	content := trimSpace(resp.AssistantText())
	if content == "" {
		return "", fmt.Errorf("empty revision")
	}
	return content, nil
}

// SaveModelPreference persists model/effort into config.
func (d *Deps) SaveModelPreference(model, effort string) error {
	d.Cfg.Model = model
	d.Cfg.Effort = effort
	return config.Save(d.Paths.ConfigPath, d.Cfg)
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		c := s[len(s)-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
