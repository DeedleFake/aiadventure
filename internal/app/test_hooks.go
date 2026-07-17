package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ApplyChatDoneForTest injects a chatDoneMsg through the shipped Update path.
// Used to verify error handling refreshes the transcript after a partial save.
func ApplyChatDoneForTest(m Model, err error) Model {
	next, _ := m.Update(chatDoneMsg{Err: err})
	return next.(Model)
}

// RefreshTranscriptForTest rebuilds the play transcript from the current session.
// Used when tests mutate the session tree outside the normal Update path.
func RefreshTranscriptForTest(m Model) Model {
	m.refreshTranscript()
	return m
}

// Ensure tea is referenced if only used for Model assertion in tests elsewhere.
var _ tea.Model = Model{}
