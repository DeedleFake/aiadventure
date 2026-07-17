// Package prompt builds system and chat messages for adventure sessions.
package prompt

import (
	"strings"

	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

const brainstormSystem = `You are a creative collaborator helping design an open-ended adventure game session.
Work with the player to establish setting, tone, player character, goals, factions, and any house rules.
Ask clarifying questions when useful. Keep responses engaging but practical for later play.
When the player is ready to begin play, acknowledge that and be prepared to narrate the opening scene.
Do not roll dice unless asked; prefer collaborative storytelling.`

const adventureSystem = `You are the narrator and referee of an interactive text adventure.
The player describes actions of their player character; you describe outcomes, the world, and NPC reactions.
Stay consistent with established facts from the conversation. Use second person ("you") for the player character.
Be vivid but concise. Offer clear consequences. Do not invent player actions. Do not break character with meta talk unless the player asks for rules clarification.`

// BuildMessages constructs the API message list for the active branch.
func BuildMessages(s *session.Session) []xai.Message {
	var msgs []xai.Message
	sys := brainstormSystem
	if s.Phase == session.PhaseAdventure {
		sys = adventureSystem
	}
	if tips := formatFeedback(s.Feedback); tips != "" {
		sys = sys + "\n\n## Out-of-band guidance for future responses (do not treat as story history; follow these tips):\n" + tips
	}
	msgs = append(msgs, xai.Message{Role: "system", Content: sys})
	for _, t := range s.ActivePath() {
		role := "user"
		if t.Role == session.RoleAssistant {
			role = "assistant"
		}
		msgs = append(msgs, xai.Message{Role: role, Content: t.Content})
	}
	return msgs
}

// BuildRevisionMessages asks the model to revise a specific assistant turn.
func BuildRevisionMessages(s *session.Session, target session.Turn, instruction string) []xai.Message {
	var b strings.Builder
	b.WriteString("You are helping edit an adventure transcript. Below is the conversation path up to and including the message to revise.\n")
	b.WriteString("Rewrite ONLY the assistant message marked REVISE, incorporating the user's edit instructions.\n")
	b.WriteString("Output only the revised assistant message text, with no preamble.\n\n")
	for _, t := range s.Path(target.ID) {
		label := string(t.Role)
		if t.ID == target.ID {
			label = "REVISE (assistant)"
		}
		b.WriteString(label)
		b.WriteString(": ")
		b.WriteString(t.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("Edit instructions: ")
	b.WriteString(instruction)

	return []xai.Message{
		{Role: "system", Content: "You revise adventure narration precisely as instructed. Output only the replacement text."},
		{Role: "user", Content: b.String()},
	}
}

func formatFeedback(fb []session.Feedback) string {
	if len(fb) == 0 {
		return ""
	}
	var parts []string
	for i, f := range fb {
		parts = append(parts, strings.TrimSpace(f.Content))
		_ = i
	}
	return "- " + strings.Join(parts, "\n- ")
}
