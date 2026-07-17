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

- Sign in to xAI via device-code OAuth; choose model and reasoning effort when supported
- Create adventure sessions (auto-saved/loaded under a configurable directory)
- Brainstorming phase for worldbuilding, then adventure play (player actions → AI narration)
- Edit prior AI or user turns (manual or AI-assisted revision); edits fork branches
- All branches remain accessible; loading opens the most recently changed branch
- Searchable session list
- Out-of-band feedback notes that guide future AI replies without rewriting story turns

## TUI keyboard map

### Hub
| Key | Action |
|-----|--------|
| ↑/↓ or j/k | Move |
| Enter | Select |
| q / Esc | Quit |

### Session browser
| Key | Action |
|-----|--------|
| ↑/↓ | Move |
| Enter | Open session |
| `/` | Search/filter |
| n | New session |
| Esc | Back to hub |

### Play
| Key | Action |
|-----|--------|
| Enter | Send message to AI |
| Ctrl+A | Session actions (phase, edit, revise, feedback, branch, model) |
| Ctrl+U | Clear input |
| Esc | Save and return to hub |
| PgUp/PgDn | Scroll transcript |

### Multi-line forms (edit / feedback / revise instruction)
| Key | Action |
|-----|--------|
| Enter | Insert newline |
| Ctrl+S | Submit |
| Esc | Cancel |
