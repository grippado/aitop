// Package domain holds the shared, JSON-pure data types that every Source
// adapter produces and every UI pane consumes. Types here must stay plain
// data: no context.Context, no runtime pointers to live OS handles, nothing
// that can't round-trip through encoding/json. That's what keeps the v2
// external-plugin path (subprocess + JSON-RPC) open without a refactor.
package domain

import "time"

// ProcessInfo describes a single OS process attributed to an AI tool.
type ProcessInfo struct {
	PID       int       `json:"pid"`
	PPID      int       `json:"ppid"`
	Tool      string    `json:"tool"`
	Label     string    `json:"label"`
	CPUPct    float64   `json:"cpu_pct"`
	MemMB     float64   `json:"mem_mb"`
	StartedAt time.Time `json:"started_at"`
}

// SessionInfo describes a logical agent session, which may or may not have a
// currently-live PID attached to it.
type SessionInfo struct {
	Tool      string    `json:"tool"`
	ID        string    `json:"id"`
	PID       int       `json:"pid,omitempty"`
	Alive     bool      `json:"alive"`
	CWD       string    `json:"cwd,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Dirty     bool      `json:"dirty,omitempty"`
	Model     string    `json:"model,omitempty"`
	Status    string    `json:"status"` // "busy" | "idle" | "unknown"
	UpdatedAt time.Time `json:"updated_at"`
	// LastAction is a short, human-readable summary of the most recent
	// tool call or thinking snippet found in the session's own transcript
	// (e.g. "🔧 Bash: go test ./..."), when the adapter can read one.
	// Empty when unavailable — never fabricated.
	LastAction string `json:"last_action,omitempty"`
	// Title is a short, descriptive label for what this session is
	// working on — e.g. Claude Code's own auto-generated "ai-title" for
	// the conversation. Empty when the adapter has no such source.
	Title string `json:"title,omitempty"`

	// TokensIn/TokensOut/ContextUsedPct are THIS session's own token
	// usage, when the adapter can read a per-session source (Claude
	// Code's own transcript today) — deliberately separate from
	// UsageInfo, which is tool-wide by contract and would otherwise show
	// the same numbers on every session's card. Zero/0 means
	// unavailable, not a real zero reading.
	TokensIn       int64   `json:"tokens_in,omitempty"`
	TokensOut      int64   `json:"tokens_out,omitempty"`
	ContextUsedPct float64 `json:"context_used_pct,omitempty"`
}

// UsageInfo describes cost/token/rate-limit usage for a tool. Available=false
// means the tool genuinely has no usage data (e.g. Cursor) — callers must
// render that as "—", never as a fabricated zero.
type UsageInfo struct {
	Tool           string   `json:"tool"`
	Available      bool     `json:"available"`
	CostTodayUSD   float64  `json:"cost_today_usd,omitempty"`
	CostMonthUSD   float64  `json:"cost_month_usd,omitempty"`
	TokensIn       int64    `json:"tokens_in,omitempty"`
	TokensOut      int64    `json:"tokens_out,omitempty"`
	ContextUsedPct float64  `json:"context_used_pct,omitempty"`
	LimitFiveHour  *float64 `json:"limit_five_hour,omitempty"`
	LimitWeekly    *float64 `json:"limit_weekly,omitempty"`
}

// ToolStatus is the at-a-glance per-tool summary for the status-strip pane.
type ToolStatus struct {
	Tool             string  `json:"tool"`
	Installed        bool    `json:"installed"`
	Running          bool    `json:"running"`
	SessionCount     int     `json:"session_count"`
	OldestSessionSec float64 `json:"oldest_session_sec,omitempty"`
	// Note surfaces best-effort/degraded states honestly, e.g.
	// "detectado, log não parseável" — never fail silently.
	Note string `json:"note,omitempty"`
}

// SystemStats is real, whole-machine resource usage — the same category of
// numbers a real btop shows — independent of which processes are AI tools.
// Panes overlay the AI-process share on top of these for the "AI-total: N%
// of system" annotation.
type SystemStats struct {
	PerCoreCPUPct []float64 `json:"per_core_cpu_pct"`
	MemUsedMB     float64   `json:"mem_used_mb"`
	MemTotalMB    float64   `json:"mem_total_mb"`
	NetUpBps      float64   `json:"net_up_bps"`
	NetDownBps    float64   `json:"net_down_bps"`
}

// Snapshot is the full state emitted by the collector on each tick, and the
// exact shape `aitop --once --json` prints.
type Snapshot struct {
	TakenAt   time.Time     `json:"taken_at"`
	Warming   bool          `json:"warming"` // true on the first live tick / --once, CPU% has no baseline yet
	System    SystemStats   `json:"system"`
	Tools     []ToolStatus  `json:"tools"`
	Processes []ProcessInfo `json:"processes"`
	Sessions  []SessionInfo `json:"sessions"`
	Usage     []UsageInfo   `json:"usage"`
}
