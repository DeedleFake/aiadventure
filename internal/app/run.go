package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"deedles.dev/aiadventure/internal/config"
)

// App is the TUI application entry wrapper.
type App struct {
	Deps *Deps
}

// New constructs an App from config.
func New(cfg config.Config, paths config.Paths) *App {
	return &App{Deps: NewDeps(cfg, paths)}
}

// Run starts the Bubble Tea program until quit or context cancel.
func (a *App) Run(ctx context.Context) error {
	if a.Deps == nil {
		return fmt.Errorf("nil deps")
	}
	m := NewModel(a.Deps, ctx)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
