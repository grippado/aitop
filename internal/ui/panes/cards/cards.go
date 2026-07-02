// Package cards builds and renders aitop's primary surface: one card per
// live/recent agent session. This is a pure presentation join over
// domain.Snapshot — it does not talk to any Source, it only reshapes data
// the backend already produced.
//
// Known honesty gaps, inherited from what adapters currently populate (not
// this package's decision to make, and not fixed here since the backend
// is out of scope for this refactor): SessionInfo.Branch/Dirty are not set
// by any adapter yet, so the card's footer context-line falls back to "—"
// on real data today (Model, unlike those two, IS populated by most
// adapters now). Tokens and context% come straight from each session
// (SessionInfo.TokensIn/TokensOut/ContextUsedPct) when the adapter has a
// per-session source. Cost/rate-limits have no per-session source and
// stay tool-wide (every session's card shows the same cost/limit reading
// for that tool) — process-count/CPU-sum is attributed to one
// "representative" session per tool (the alive one with the most recent
// activity) so it isn't double-counted across cards.
//
// One cross-tool concern lives here too: a cursor-agent CLI run and its
// mirror inside Cursor IDE's own composer store share the same session ID
// (confirmed — see internal/source/cursor's enrichWithComposer), so
// BuildCards drops the "cursor" (IDE) card for any session ID a
// cursor-agent session already claims, rather than showing the identical
// real task twice.
package cards

import (
	"sort"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// SortColumn selects card ordering, cycled via 'o'.
type SortColumn int

const (
	SortContext SortColumn = iota
	SortTokens
	SortAge
	SortTool
)

func (c SortColumn) String() string {
	switch c {
	case SortTokens:
		return "tokens"
	case SortAge:
		return "age"
	case SortTool:
		return "tool"
	default:
		return "ctx"
	}
}

// Card is one agent/session, denormalized for rendering.
type Card struct {
	Tool       string
	SessionID  string
	PID        int
	Status     string // "busy" | "idle" | "unknown" (raw session status)
	Alive      bool
	CWD        string
	Branch     string
	Dirty      bool
	Model      string
	Title      string  // e.g. Claude Code's own auto-generated session title
	LastAction string  // e.g. "🔧 Bash: go test ./..." — "" when unavailable
	AgeSec     float64 // seconds since last activity

	HasContext bool
	ContextPct float64

	HasTokens bool
	TokensIn  int64
	TokensOut int64

	HasCost      bool
	CostTodayUSD float64
	CostMonthUSD float64

	LimitFiveHour *float64
	LimitWeekly   *float64

	ProcCount  int
	ProcCPUSum float64
}

// BuildCards joins Sessions/Processes/Usage into cards, applying an
// optional tool filter ("" = all).
func BuildCards(snap domain.Snapshot, toolFilter string) []Card {
	usageByTool := map[string]domain.UsageInfo{}
	for _, u := range snap.Usage {
		usageByTool[u.Tool] = u
	}

	procByPID := map[int]domain.ProcessInfo{}
	for _, p := range snap.Processes {
		procByPID[p.PID] = p
	}

	// One representative session per tool absorbs processes that can't be
	// tied to a specific session PID (e.g. Cursor's many helper processes,
	// or a Claude daemon PID with no session file of its own) — prefer the
	// alive session with the most recent activity, so it isn't split
	// arbitrarily or duplicated across every card of that tool.
	representative := map[string]domain.SessionInfo{}
	for _, s := range snap.Sessions {
		if toolFilter != "" && s.Tool != toolFilter {
			continue
		}
		cur, ok := representative[s.Tool]
		if !ok || isBetterRepresentative(s, cur) {
			representative[s.Tool] = s
		}
	}

	matchedPID := map[int]bool{}
	for _, s := range snap.Sessions {
		if s.PID != 0 {
			if _, ok := procByPID[s.PID]; ok {
				matchedPID[s.PID] = true
			}
		}
	}
	leftoverCount := map[string]int{}
	leftoverCPU := map[string]float64{}
	for _, p := range snap.Processes {
		if matchedPID[p.PID] {
			continue
		}
		leftoverCount[p.Tool]++
		leftoverCPU[p.Tool] += p.CPUPct
	}

	cursorAgentIDs := map[string]bool{}
	for _, s := range snap.Sessions {
		if s.Tool == "cursor-agent" && s.ID != "" {
			cursorAgentIDs[s.ID] = true
		}
	}

	now := time.Now()
	var out []Card
	for _, s := range snap.Sessions {
		if toolFilter != "" && s.Tool != toolFilter {
			continue
		}
		if !s.Alive {
			// A known-but-dead session (e.g. a Codex rollout from weeks
			// ago, surfaced by Sessions() for historical visibility) is
			// not a running agent — it doesn't get a card. See
			// isBetterRepresentative/leftover attribution below, which
			// already prefer alive sessions for the same reason.
			continue
		}
		if s.Tool == "cursor" && s.ID != "" && cursorAgentIDs[s.ID] {
			// The exact same real task, observed twice: cursor-agent CLI
			// runs share their composerId with Cursor IDE's own composer
			// store (confirmed — a cursor-agent transcript file is
			// literally named after the same composerId), so without this
			// the identical task gets two cards. cursor-agent wins: its
			// own transcript is the more precise per-session source (real
			// tokens, more granular last action) than the composer-store
			// enrichment the cursor (IDE) adapter falls back to for the
			// same underlying data.
			continue
		}
		c := Card{
			Tool: s.Tool, SessionID: s.ID, PID: s.PID, Status: s.Status,
			Alive: s.Alive, CWD: s.CWD, Branch: s.Branch, Dirty: s.Dirty, Model: s.Model,
			Title: s.Title, LastAction: s.LastAction,
		}
		if !s.UpdatedAt.IsZero() {
			c.AgeSec = now.Sub(s.UpdatedAt).Seconds()
		}

		if s.PID != 0 {
			if p, ok := procByPID[s.PID]; ok {
				c.ProcCount++
				c.ProcCPUSum += p.CPUPct
			}
		}
		if rep, ok := representative[s.Tool]; ok && rep.ID == s.ID && rep.PID == s.PID {
			c.ProcCount += leftoverCount[s.Tool]
			c.ProcCPUSum += leftoverCPU[s.Tool]
		}

		// Tokens/context% come straight from THIS session (s.TokensIn/
		// TokensOut/ContextUsedPct) when the adapter has a per-session
		// source (Claude Code's own transcript) — never the tool-wide
		// usage below, which used to be applied identically to every
		// session's card and looked like a bug (two different sessions
		// showing the same token count, because it was the same number).
		if s.TokensIn > 0 || s.TokensOut > 0 {
			c.HasTokens = true
			c.TokensIn, c.TokensOut = s.TokensIn, s.TokensOut
		}
		if s.ContextUsedPct > 0 {
			c.HasContext = true
			c.ContextPct = s.ContextUsedPct
		}

		// Cost/rate-limits genuinely have no per-session source (the
		// cost-day file is a UUID-keyed sum across the whole tool, and
		// ccstatusline's cache is one rolling-window reading, not per
		// conversation) — tool-wide is the honest answer here, unlike
		// tokens above. A zero value means "this adapter has never
		// populated this field," not "confirmed zero," so it's treated
		// as unavailable — except cost, where a missing cost-day file
		// really does mean zero spend recorded.
		if u, ok := usageByTool[s.Tool]; ok && u.Available {
			c.HasCost = true
			c.CostTodayUSD = u.CostTodayUSD
			c.CostMonthUSD = u.CostMonthUSD
			c.LimitFiveHour = u.LimitFiveHour
			c.LimitWeekly = u.LimitWeekly
		}

		out = append(out, c)
	}
	return out
}

func isBetterRepresentative(a, b domain.SessionInfo) bool {
	if a.Alive != b.Alive {
		return a.Alive
	}
	return a.UpdatedAt.After(b.UpdatedAt)
}

// Sort orders cards in place by the given column.
func Sort(cs []Card, col SortColumn) {
	sort.SliceStable(cs, func(i, j int) bool {
		switch col {
		case SortTokens:
			return cs[i].TokensIn+cs[i].TokensOut > cs[j].TokensIn+cs[j].TokensOut
		case SortAge:
			return cs[i].AgeSec > cs[j].AgeSec
		case SortTool:
			return cs[i].Tool < cs[j].Tool
		default: // SortContext
			return cs[i].ContextPct > cs[j].ContextPct
		}
	})
}
