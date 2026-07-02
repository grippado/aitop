package cursoragent

import (
	"context"

	"github.com/grippado/aitop/internal/domain"
)

// Usage is honestly unavailable in v1. cursor-agent's transcript format
// (~/.cursor/projects/*/agent-transcripts/*/*.jsonl) carries no per-turn
// usage block the way Claude Code's transcript does — confirmed on this
// machine's real session data, role/message/content only, no token
// counts — and no local cost ledger comparable to Claude Code's
// .cost-day-*.json files was found either. The debug log cursor-agent
// writes per run (under $TMPDIR) does carry a rough "estimated_tokens"
// figure per turn, but that file's location and lifetime aren't stable
// enough to build on for v1. Returning Available:false is the honest
// answer, not a fabricated zero — same call Codex's adapter makes for the
// same reason.
func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	return domain.UsageInfo{Tool: Name, Available: false}, nil
}
