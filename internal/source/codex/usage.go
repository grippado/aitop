package codex

import (
	"context"

	"github.com/grippado/aitop/internal/domain"
)

// Usage is honestly unavailable in v1. Two things ruled it out during
// design, both confirmed on a real machine, not assumed:
//
//  1. Cost: Codex bills through the user's own OpenAI API key
//     (~/.codex/auth.json, which this adapter must never open) — there is
//     no local per-session USD ledger comparable to Claude Code's
//     .cost-day-*.json files.
//  2. Tokens/context%/rate-limits: ~/.codex/config.toml's [tui].status_line
//     array (e.g. "context-used", "five-hour-limit", "total-input-tokens")
//     is a list of FIELD NAMES describing what Codex's own status line
//     template displays — not live numeric values. Codex computes those
//     numbers itself at render time; nothing observed on disk persists
//     them the way ccstatusline's cache file does for Claude Code.
//
// Getting real numbers here is a v2 investigation (likely needs either
// reverse-engineering the state_5.sqlite/logs_2.sqlite schema, both opened
// read-only and enrichment-only per the design doc, or Codex shipping its
// own equivalent of a statusline cache file). Returning Available:false is
// the honest v1 answer, not a fabricated zero.
func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	return domain.UsageInfo{Tool: Name, Available: false}, nil
}
