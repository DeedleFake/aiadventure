# AGENTS.md

Instructions for AI coding agents working in this repository.

## Project overview

**AI Adventure** is an AI harness for playing AI-powered open-ended adventure games.

## Technology stack

| Layer | Choice |
|-------|--------|
| Language | Go — see `go.mod` for the required toolchain |
| Module | `deedles.dev/aiadventure` |

Do not pin toolchain or dependency versions in this file (they go stale). Prefer “as specified in `go.mod`” or unversioned names.

## Development commands

```bash
go mod download
go test ./...
go vet ./...
go fmt ./...
```

`go test` already compiles packages; do not run a separate `go build` only to check that the project compiles.

## Code style and conventions

- **Logging** — `log/slog` with structured key-value fields.
- **Context** — pass `context.Context` as the first argument for cancelable / long-running work.
- **Errors** — handle explicitly; wrap with `fmt.Errorf("...: %w", err)` when adding context.
- **Modern Go** — match current stdlib helpers (`slices`, `maps`, `cmp`, `iter`, etc.) as used with the toolchain in `go.mod`.
- **Imports** — goimports-style groups: standard library, third-party, then `deedles.dev/...`.
- **Scope** — prefer small, focused changes. Do not reformat unrelated files or drive-by refactors.

## Agent guidelines

1. **Git is read-only under all circumstances.** Never create commits, amend, rebase, merge, cherry-pick, stash, checkout branches, reset, clean, tag, push, or otherwise mutate the git repository or index. Read-only commands (`status`, `diff`, `log`, `show`, `blame`, etc.) are fine. Leave all commits and branch management to the user.
2. **Do not pin versions in this file** — refer to `go.mod` or unversioned names so these instructions stay valid as versions change.
3. **Verify** with `go test ./...` and `go vet ./...` before considering work done.
4. **Secrets** — do not commit tokens, API keys, or machine-specific paths.
