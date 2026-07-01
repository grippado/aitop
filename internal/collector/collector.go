// Package collector runs the system-wide resource poll (always) and every
// registered Source adapter (concurrently, timeout-bounded) and fans the
// results into one domain.Snapshot per tick.
package collector

import (
	"context"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/source"
)

// Collector polls system stats and every Source, bounding each adapter to
// its own timeout so a hung one (e.g. sqlite lock contention) never blocks
// the others or the render loop.
type Collector struct {
	sources []source.Source
	timeout time.Duration

	mu        sync.Mutex
	warm      bool
	lastNet   gnet.IOCountersStat
	lastNetAt time.Time
}

// New builds a Collector over the given Sources (already filtered by
// source.Resolve). refresh is only used to size the per-adapter timeout
// (capped at 500ms regardless, per the design doc) — it does not drive a
// loop itself; callers tick it (e.g. from the Bubble Tea model or --once).
func New(sources []source.Source, refresh time.Duration) *Collector {
	return &Collector{sources: sources, timeout: 500 * time.Millisecond}
}

// Snapshot performs one poll cycle. Safe to call repeatedly and
// concurrently is not required — the Bubble Tea model calls it from a
// single tea.Cmd goroutine at a time.
func (c *Collector) Snapshot() domain.Snapshot {
	ctx := context.Background()
	sys := c.systemStats()

	type result struct {
		tool  domain.ToolStatus
		procs []domain.ProcessInfo
		sess  []domain.SessionInfo
		usage domain.UsageInfo
	}

	results := make([]result, len(c.sources))
	var wg sync.WaitGroup
	for i, s := range c.sources {
		wg.Add(1)
		go func(i int, s source.Source) {
			defer wg.Done()
			tctx, cancel := context.WithTimeout(ctx, c.timeout)
			defer cancel()

			procs, procErr := s.Processes(tctx)
			sess, _ := s.Sessions(tctx)
			usage, _ := s.Usage(tctx)

			// Running reflects actual liveness (an adapter's Sessions can
			// mark Alive from a plain ps check independent of whether its
			// richer per-process data parsed) — never gated on procs being
			// non-empty, so a degraded-but-alive tool (e.g. Cursor with an
			// unparseable log) still shows as running, just annotated.
			//
			// SessionCount/OldestSessionSec are computed over ALIVE
			// sessions only. A dead session (Alive:false — e.g. a Codex
			// rollout from weeks ago found via history.jsonl) is a known
			// historical record, not a running agent: it must not inflate
			// the session count or make "oldest session" report a session
			// that ended long ago as if it were still open.
			running := len(procs) > 0
			var oldest float64
			aliveCount := 0
			for _, se := range sess {
				if !se.Alive {
					continue
				}
				running = true
				aliveCount++
				if !se.UpdatedAt.IsZero() {
					if age := time.Since(se.UpdatedAt).Seconds(); age > oldest {
						oldest = age
					}
				}
			}

			note := ""
			if procErr != nil {
				note = procErr.Error()
			}

			results[i] = result{
				tool: domain.ToolStatus{
					Tool:             s.Name(),
					Installed:        true,
					Running:          running,
					SessionCount:     aliveCount,
					OldestSessionSec: oldest,
					Note:             note,
				},
				procs: procs,
				sess:  sess,
				usage: usage,
			}
		}(i, s)
	}
	wg.Wait()

	warming := !c.warm
	c.warm = true

	snap := domain.Snapshot{TakenAt: time.Now(), Warming: warming, System: sys}
	for _, r := range results {
		snap.Tools = append(snap.Tools, r.tool)
		snap.Processes = append(snap.Processes, r.procs...)
		snap.Sessions = append(snap.Sessions, r.sess...)
		snap.Usage = append(snap.Usage, r.usage)
	}
	return snap
}

// systemStats reads real, whole-machine CPU/mem/net — independent of which
// Sources are registered. cpu.Percent is called with a 200ms sampling
// interval so even the very first call (--once, cold start) yields a real
// delta instead of a meaningless first-ever-call value.
func (c *Collector) systemStats() domain.SystemStats {
	perCore, _ := cpu.Percent(200*time.Millisecond, true)
	vm, _ := mem.VirtualMemory()

	var memUsed, memTotal float64
	if vm != nil {
		memUsed = float64(vm.Used) / 1024 / 1024
		memTotal = float64(vm.Total) / 1024 / 1024
	}

	var up, down float64
	if counters, err := gnet.IOCounters(false); err == nil && len(counters) > 0 {
		now := time.Now()
		c.mu.Lock()
		if !c.lastNetAt.IsZero() {
			if dt := now.Sub(c.lastNetAt).Seconds(); dt > 0 {
				up = float64(counters[0].BytesSent-c.lastNet.BytesSent) / dt
				down = float64(counters[0].BytesRecv-c.lastNet.BytesRecv) / dt
			}
		}
		c.lastNet = counters[0]
		c.lastNetAt = now
		c.mu.Unlock()
	}

	return domain.SystemStats{
		PerCoreCPUPct: perCore,
		MemUsedMB:     memUsed,
		MemTotalMB:    memTotal,
		NetUpBps:      up,
		NetDownBps:    down,
	}
}
