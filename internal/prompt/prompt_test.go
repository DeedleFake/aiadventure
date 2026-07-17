package prompt_test

import (
	"strings"
	"testing"

	"deedles.dev/aiadventure/internal/prompt"
	"deedles.dev/aiadventure/internal/session"
)

func TestBuildMessagesIncludesFeedbackNotAsTurns(t *testing.T) {
	s := session.New("T", "grok-4.5", "high")
	_, _ = s.Append(session.RoleUser, "Fantasy please")
	_, _ = s.Append(session.RoleAssistant, "Coastal realm?")
	s.AddFeedback("Keep it dark fantasy")
	msgs := prompt.BuildMessages(s)
	if len(msgs) != 3 {
		t.Fatalf("want system+2 turns, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatal("first should be system")
	}
	if !strings.Contains(msgs[0].Content, "Keep it dark fantasy") {
		t.Fatalf("system should include feedback: %s", msgs[0].Content)
	}
	if msgs[1].Role != "user" || msgs[2].Role != "assistant" {
		t.Fatalf("roles: %s %s", msgs[1].Role, msgs[2].Role)
	}
}

func TestBuildMessagesAdventurePhase(t *testing.T) {
	s := session.New("T", "m", "")
	_ = s.SetPhase(session.PhaseAdventure)
	msgs := prompt.BuildMessages(s)
	if !strings.Contains(msgs[0].Content, "narrator") {
		t.Fatalf("adventure system prompt missing: %s", msgs[0].Content)
	}
}

func TestBuildRevisionMessages(t *testing.T) {
	s := session.New("T", "m", "")
	_, _ = s.Append(session.RoleUser, "Look around")
	aid, _ := s.Append(session.RoleAssistant, "You see a door")
	target := s.Turns[aid]
	msgs := prompt.BuildRevisionMessages(s, target, "Make it spookier")
	if len(msgs) != 2 {
		t.Fatalf("len=%d", len(msgs))
	}
	if !strings.Contains(msgs[1].Content, "REVISE") || !strings.Contains(msgs[1].Content, "spookier") {
		t.Fatalf("revision body: %s", msgs[1].Content)
	}
}
