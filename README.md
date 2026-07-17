AI Adventure
============

AI Adventure is an AI harness for playing AI-powered open-ended adventure games.

## Run

```bash
go run ./cmd/aiadventure
go run ./cmd/aiadventure --help
go run ./cmd/aiadventure --sessions-dir /path/to/sessions
```

## Features (initial version)

- Sign in to xAI via device-code OAuth; choose model and reasoning effort when supported
- Create adventure sessions (auto-saved/loaded under a configurable directory)
- Brainstorming phase for worldbuilding, then adventure play (player actions → AI narration)
- Edit prior AI or user turns (manual or AI-assisted revision); edits fork branches
- All branches remain accessible; loading opens the most recently changed branch
- Searchable session list
- Out-of-band feedback notes that guide future AI replies without rewriting story turns

## Session commands (while playing)

| Command | Action |
|---------|--------|
| `/phase adventure` | Leave brainstorming and begin play |
| `/edit` | Manually edit a prior turn (fork) |
| `/revise` | Ask the AI to rewrite an AI turn (fork) |
| `/feedback` | Add tips for future replies only |
| `/branch` | List/switch branch tips |
| `/menu` | Save and return to the main menu |
