package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// costEntry mirrors one UUID's entry in <configDir>/.cost-day-YYYY-MM-DD.json
// and .cost-month-YYYY-MM.json: {"base": <usd before this period>, "current":
// <usd so far>}. Spend for the period is current - base, summed across
// every session UUID present in the file.
type costEntry struct {
	Base    float64 `json:"base"`
	Current float64 `json:"current"`
}

// ccstatuslineUsage mirrors ~/.cache/ccstatusline/usage.json — the LIVE
// cache written by the actual npm `ccstatusline` package (v2), which is
// what this user's statusLine hook (`~/.claude/settings.json` ->
// statusLine.command -> `$HOME/.claude/bin/ccstatusline`, symlinked to
// `~/cangaco/.ai/claude/bin/ccstatusline`) really invokes today.
//
// This supersedes an earlier, wrong assumption: `~/.claude/.statusline-
// cache.json` (written by a legacy `statusline-command-v4.sh` bash script)
// stopped being updated in May 2026 and is dead. So did the
// `.cost-day-*.json` / `.cost-month-*.json` files this adapter also reads
// below — no file for the current day/month exists anywhere searched
// (`~/.claude/`, `~/cangaco/.ai/claude/`), which is why cost fields are
// genuinely, honestly zero right now, not a parsing bug.
//
// sessionUsage/sessionResetAt map to Claude's 5-hour rolling window;
// weeklyUsage/weeklyResetAt map to the 7-day window (naming is
// ccstatusline's own; inferred from the two-tier reset cadence matching
// Claude's known 5h/weekly limit model, not confirmed against
// ccstatusline's own source).
type ccstatuslineUsage struct {
	SessionUsage   float64 `json:"sessionUsage"`
	SessionResetAt string  `json:"sessionResetAt"`
	WeeklyUsage    float64 `json:"weeklyUsage"`
	WeeklyResetAt  string  `json:"weeklyResetAt"`
}

// Usage reports tool-wide cost and rate limits only. Tokens/context% used
// to be approximated here from an arbitrarily-picked "best" session and
// applied uniformly to every session's card — which looked like a bug
// (and was reported as one): two different sessions showing identical
// token counts. That data now lives on each SessionInfo directly (see
// Sessions(), which already tails every session's own transcript for
// Title/LastAction and grabs its token reading at the same time) —
// genuinely per-session, not a tool-wide stand-in.
func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	u := domain.UsageInfo{Tool: Name}

	now := time.Now()
	costFound := false
	if v, ok := a.sumCostFile(costDayPath(a.configDir, now)); ok {
		u.CostTodayUSD = v
		costFound = true
	}
	if v, ok := a.sumCostFile(costMonthPath(a.configDir, now)); ok {
		u.CostMonthUSD = v
		costFound = true
	}

	limitsFound := false
	if cache, ok := a.readCcstatuslineUsage(); ok {
		if resetAt, err := time.Parse(time.RFC3339Nano, cache.SessionResetAt); err == nil && resetAt.After(now) {
			v := cache.SessionUsage
			u.LimitFiveHour = &v
			limitsFound = true
		}
		if resetAt, err := time.Parse(time.RFC3339Nano, cache.WeeklyResetAt); err == nil && resetAt.After(now) {
			v := cache.WeeklyUsage
			u.LimitWeekly = &v
			limitsFound = true
		}
	}

	// Available only when at least one field above is a genuine reading —
	// never true with every field left at its zero value, which would
	// read as "confirmed $0" when really nothing was found at all.
	u.Available = costFound || limitsFound

	return u, nil
}

func costDayPath(configDir string, t time.Time) string {
	return filepath.Join(configDir, ".cost-day-"+t.Format("2006-01-02")+".json")
}

func costMonthPath(configDir string, t time.Time) string {
	return filepath.Join(configDir, ".cost-month-"+t.Format("2006-01")+".json")
}

// sumCostFile reads a cost-day/cost-month file and sums current-base
// across every session UUID. ok=false means the file doesn't exist / isn't
// parseable — genuinely "no reading," not "zero spend": callers must not
// treat that as a confirmed $0.
func (a *Adapter) sumCostFile(path string) (float64, bool) {
	raw, err := reader.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var entries map[string]costEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return 0, false
	}
	var total float64
	for _, e := range entries {
		total += e.Current - e.Base
	}
	return total, true
}

func (a *Adapter) readCcstatuslineUsage() (ccstatuslineUsage, bool) {
	raw, err := reader.ReadFile(filepath.Join(ccstatuslineCacheDir(), "usage.json"))
	if err != nil {
		return ccstatuslineUsage{}, false
	}
	var c ccstatuslineUsage
	if err := json.Unmarshal(raw, &c); err != nil {
		return ccstatuslineUsage{}, false
	}
	return c, true
}

// ccstatuslineCacheDir follows XDG_CACHE_HOME when set, matching how the
// real ccstatusline npm package resolves its own cache location.
func ccstatuslineCacheDir() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return filepath.Join(v, "ccstatusline")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "ccstatusline")
}
