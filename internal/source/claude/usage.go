package claude

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// costEntry mirrors one UUID's entry in ~/.claude/.cost-day-YYYY-MM-DD.json
// and .cost-month-YYYY-MM.json: {"base": <usd before this period>, "current":
// <usd so far>}. Spend for the period is current - base, summed across
// every session UUID present in the file.
type costEntry struct {
	Base    float64 `json:"base"`
	Current float64 `json:"current"`
}

// statuslineCache mirrors ~/.claude/.statusline-cache.json, a side effect of
// the ccstatusline hook (~/cangaco/.ai/claude/statusline-command-v4.sh)
// caching Claude Code's own rate-limit fields so they survive between
// statusLine invocations. Reused here rather than reimplemented — but it
// only exists if the user has ccstatusline (or an equivalent hook)
// configured, and only reflects the LAST time Claude Code rendered a status
// line, so it can be stale. context_used_pct has no equivalent on-disk
// cache (it's pushed to the statusLine hook live, never persisted) — v1
// leaves Claude Code's ContextUsedPct at 0/unavailable; a real fix needs
// aitop to register its own lightweight statusLine sink, which is a v2 idea,
// not a v1 claim.
type statuslineCache struct {
	RL5Pct   float64 `json:"rl5_pct"`
	RL5Reset int64   `json:"rl5_reset"` // unix seconds
	RL7Pct   float64 `json:"rl7_pct"`
	RL7Reset int64   `json:"rl7_reset"`
}

func (a *Adapter) Usage(ctx context.Context) (domain.UsageInfo, error) {
	u := domain.UsageInfo{Tool: Name, Available: true}

	now := time.Now()
	u.CostTodayUSD = a.sumCostFile(costDayPath(a.home, now))
	u.CostMonthUSD = a.sumCostFile(costMonthPath(a.home, now))

	if cache, ok := a.readStatuslineCache(); ok {
		nowSec := now.Unix()
		if cache.RL5Reset > nowSec {
			v := cache.RL5Pct
			u.LimitFiveHour = &v
		}
		if cache.RL7Reset > nowSec {
			v := cache.RL7Pct
			u.LimitWeekly = &v
		}
	}

	return u, nil
}

func costDayPath(home string, t time.Time) string {
	return filepath.Join(home, ".claude", ".cost-day-"+t.Format("2006-01-02")+".json")
}

func costMonthPath(home string, t time.Time) string {
	return filepath.Join(home, ".claude", ".cost-month-"+t.Format("2006-01")+".json")
}

// sumCostFile reads a cost-day/cost-month file and sums current-base across
// every session UUID. A missing file means genuinely zero spend recorded
// for that period, not "unavailable" — that's a real zero, not a fabricated
// one.
func (a *Adapter) sumCostFile(path string) float64 {
	raw, err := reader.ReadFile(path)
	if err != nil {
		return 0
	}
	var entries map[string]costEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return 0
	}
	var total float64
	for _, e := range entries {
		total += e.Current - e.Base
	}
	return total
}

func (a *Adapter) readStatuslineCache() (statuslineCache, bool) {
	raw, err := reader.ReadFile(filepath.Join(a.home, ".claude", ".statusline-cache.json"))
	if err != nil {
		return statuslineCache{}, false
	}
	var c statuslineCache
	if err := json.Unmarshal(raw, &c); err != nil {
		return statuslineCache{}, false
	}
	return c, true
}
