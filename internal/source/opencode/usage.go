package opencode

import (
	"context"

	"github.com/grippado/aitop/internal/domain"
)

// Usage is honestly unavailable in v1. opencode's `session.cost` column IS
// real (confirmed on this machine's data) — but it's a lifetime-cumulative
// total for that one session, not a day/month-scoped figure the way Claude
// Code's .cost-day-*.json files are. Summing it by session.time_created
// would misattribute a long-running session's entire cost to whichever
// day it started, understating every other day it stayed open — a subtly
// wrong number, not an honest one. Available:false here, same call
// Codex/cursor-agent's adapters make for their own reasons; the tokens
// this DOES have a real per-session reading for already live on
// SessionInfo (see Sessions()), same as every other adapter.
func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	return domain.UsageInfo{Tool: Name, Available: false}, nil
}
