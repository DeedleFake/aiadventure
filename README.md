AI Adventure
============

> **Warning:** This project is in early development and should not be used.

AI Adventure is an AI harness for playing AI-powered open-ended adventure games.

## Run

```bash
go run ./cmd/aiadventure
go run ./cmd/aiadventure --help
go run ./cmd/aiadventure --sessions-dir /path/to/sessions
```

The interactive app is a full-screen, keyboard-driven TUI (Bubble Tea).

## Features (initial version)

- Starts on an empty new session (nothing is saved until the first message)
- Sessions auto-name from the first user message; rename anytime with `/rename`
- Slash commands with fuzzy search for every app feature (settings, sessions, phase, …)
- Settings (model / effort) open as a centered modal over the play screen
- Tab switches focus between the message input and selectable history turns
- Sign in to xAI via device-code OAuth
- Brainstorming phase for worldbuilding, then adventure play
- Edit prior turns (manual or AI-assisted); edits fork branches
- Searchable session list; out-of-band feedback for future AI replies

## TUI keyboard map

### Play (main screen)
| Key | Action |
|-----|--------|
| Enter | Send message, or run slash command |
| `/…` | Slash commands; fuzzy list appears above the prompt |
| Tab | Toggle focus: input ↔ history |
| ↑/↓ | Navigate slash palette or history selection |
| Ctrl+U | Clear input |
| Esc | Clear input / close palette; cancel overlays |
| PgUp/PgDn | Scroll transcript |

### History focus
| Key | Action |
|-----|--------|
| ↑/↓ | Select a turn |
| Enter | Edit selected turn (manual fork form) |
| Tab / Esc | Return to input |

### Slash commands
| Command | Action |
|---------|--------|
| `/rename [title]` | Rename session (modal if no title given) |
| `/settings` / `/model` | Settings modal (model / effort) |
| `/sessions` | Browse / search / open sessions |
| `/new` | Start a new empty unsaved session |
| `/phase` | Toggle brainstorm ↔ adventure |
| `/edit` | Edit selected (or pick) turn |
| `/revise` | AI-revise selected assistant turn |
| `/feedback` | Add out-of-band feedback |
| `/branch` | Switch branch |
| `/signin` / `/signout` | OAuth auth |
| `/help` | List commands |
| `/quit` | Exit |

### Multi-line forms (edit / feedback / revise instruction)
| Key | Action |
|-----|--------|
| Enter | Insert newline |
| Ctrl+S | Submit |
| Esc | Cancel |
