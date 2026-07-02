# Contributing to aitop

## Development

```sh
go build ./...
CGO_ENABLED=0 go test -race ./...
go vet ./...
```

Every adapter package (`internal/source/<tool>/`) keeps its filesystem/process access behind a `Reader`/`Runner`-style interface defined in a `runner.go` file — that's the only file in the package allowed to touch `os.ReadFile`/`os.ReadDir`/`os.Stat` directly. Tests swap that interface for a fake; never spawn real subprocesses or touch a real `~/.claude`/`~/.codex`/Cursor directory in a test.

Two exceptions read real state from SQLite (`database/sql` + `modernc.org/sqlite`, pure Go — no cgo, keeps `CGO_ENABLED=0` release builds intact) rather than plain files, so that access does NOT go through the `Reader` interface: `internal/source/opencode` (its own `opencode.db`) and `internal/source/cursor`'s `composer.go` (Cursor IDE's `state.vscdb` — a single global VSCode-style store shared with every other extension's data, hundreds of MB on a real machine). Both are tested against a real temporary SQLite file instead of a faked byte-level reader (see `opencode_test.go`'s `openTestDB` / `composer_test.go`'s `openTestComposerDB`), which is more faithful for SQL query behavior. The `Reader` interface in `opencode` still covers everything that IS plain-file access (`Detect()`, the models.json cache); `cursor`'s own `Reader` still covers the process-monitor log.

`composer.go` is also the sharpest example of the security rule below applied to SQL instead of the filesystem: every query is either a single exact-key lookup (`key = 'composer.composerHeaders'`) or a `GLOB` scoped to one composer ID this adapter already resolved itself — never `SELECT *` or an unscoped scan over a store that also holds other extensions' data.

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

## Updating the README demo GIF

`demo.gif` is recorded with [VHS](https://github.com/charmbracelet/vhs) from `vhs.tape`, against `--demo` mode's synthetic sessions (`internal/demo/demo.go`) — never real tool state, so the recording is reproducible on any machine.

```sh
brew install vhs   # pulls in ttyd + ffmpeg
go build -o aitop ./cmd/aitop/
vhs vhs.tape        # writes demo.gif
```

If the card layout changes, re-record rather than hand-editing the GIF. `vhs.tape`'s `Set FontFamily` must name a monospace family that's actually installed (check with `system_profiler SPFontsDataType | grep -i <name>` on macOS) — a proportional fallback silently breaks every box-drawing/bar-alignment in the recording without erroring.
