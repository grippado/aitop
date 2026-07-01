// Package source defines the adapter contract every AI-tool integration
// implements, and the registry that discovers which ones apply on this
// machine.
package source

import (
	"context"

	"github.com/grippado/aitop/internal/domain"
)

// Source is a read-only adapter for one AI coding tool (Claude Code, Codex,
// Cursor, ...). Implementations must never mutate the tool's state and must
// never fabricate data they can't actually observe — return
// UsageInfo.Available=false instead of a zero value when a metric doesn't
// exist for that tool.
type Source interface {
	// Name identifies the tool, e.g. "claude-code", "codex", "cursor".
	Name() string

	// Detect reports whether this tool is installed/configured on this
	// machine at all, independent of whether it's currently running.
	Detect(ctx context.Context) bool

	// Processes returns the OS processes currently attributable to this
	// tool, with real CPU/mem figures.
	Processes(ctx context.Context) ([]domain.ProcessInfo, error)

	// Sessions returns known logical sessions for this tool, live or
	// recent, each flagged Alive based on actual process verification.
	Sessions(ctx context.Context) ([]domain.SessionInfo, error)

	// Usage returns cost/token/rate-limit data for this tool. Returns
	// Available=false when the tool has no such data (e.g. Cursor).
	Usage(ctx context.Context) (domain.UsageInfo, error)
}
