# Contributing to aitop

## Development

```sh
go build ./...
CGO_ENABLED=0 go test -race ./...
go vet ./...
```

Every adapter package (`internal/source/<tool>/`) keeps its filesystem/process access behind a `Reader`/`Runner`-style interface defined in a `runner.go` file — that's the only file in the package allowed to touch `os.ReadFile`/`os.ReadDir`/`os.Stat` directly. Tests swap that interface for a fake; never spawn real subprocesses or touch a real `~/.claude`/`~/.codex`/Cursor directory in a test.

## Adding a new theme

v1 ships exactly one theme (`btop-classic`, in `internal/ui/theme/theme.go`). All of aitop's colors live in that single `Theme` struct on purpose — adding a new one is meant to be a small, self-contained PR:

1. Add a new `Theme` value alongside `BtopClassic` in `internal/ui/theme/theme.go`.
2. That's it for the color definition. Wiring a `--theme`/cycle-key selector back into the UI is tracked as a v2 item — a theme PR doesn't need to solve that part too, just contribute the palette.

Theme PRs are genuinely welcome — this is one of the easiest ways to contribute without touching adapter logic.

## Adding a new Source adapter

Implement `source.Source` (`internal/source/source.go`) for the new tool under `internal/source/<tool>/`, following the pattern in `internal/source/claude/` or `internal/source/codex/`:

- `Detect` should be cheap and side-effect-free.
- Never fabricate a value the tool can't actually provide — return `Available: false` (for `UsageInfo`) or omit the field, and let the UI render `—`.
- If the tool has anything remotely like an API key or credential file, name it explicitly as excluded (see `internal/source/codex/allowlist.go` for the pattern) rather than relying on "just don't read it."

## Security-sensitive adapters

If your adapter touches a directory that could contain credentials (API keys, tokens, cookies), do not use `filepath.Walk`/`filepath.Glob` over that directory. Use an explicit named-path allowlist instead, and add a test asserting the credential file is never in that allowlist — see `internal/source/codex/allowlist_test.go`.
