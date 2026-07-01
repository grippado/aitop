// Package procstat caches gopsutil process handles per PID across polling
// ticks so CPU% is a real delta between ticks instead of re-priming a
// fresh sample from zero on every call — the "cache the baseline, halve
// the syscalls" rule from the design doc. Used by every per-tool adapter
// that reports live process CPU/mem.
package procstat

import (
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

// Cache keeps one *process.Process per PID alive across ticks.
type Cache struct {
	mu    sync.Mutex
	procs map[int32]*process.Process
}

func NewCache() *Cache {
	return &Cache{procs: map[int32]*process.Process{}}
}

// Stat returns CPU% and resident memory in MB for pid. The first call after
// a PID is first seen has no baseline yet and returns cpuPct=0 — callers on
// the first live tick should treat that as "warming", not "idle".
func (c *Cache) Stat(pid int32) (cpuPct, memMB float64, ok bool) {
	c.mu.Lock()
	p, exists := c.procs[pid]
	if !exists {
		var err error
		p, err = process.NewProcess(pid)
		if err != nil {
			c.mu.Unlock()
			return 0, 0, false
		}
		c.procs[pid] = p
	}
	c.mu.Unlock()

	cpuPct, cpuErr := p.CPUPercent()
	memInfo, memErr := p.MemoryInfo()
	if cpuErr != nil && memErr != nil {
		return 0, 0, false
	}
	if memErr == nil && memInfo != nil {
		memMB = float64(memInfo.RSS) / 1024 / 1024
	}
	return cpuPct, memMB, true
}

// Forget drops cached process handles for PIDs that are no longer relevant,
// so long-running aitop sessions don't accumulate stale handles for every
// PID that ever existed.
func (c *Cache) Forget(alive map[int32]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for pid := range c.procs {
		if !alive[pid] {
			delete(c.procs, pid)
		}
	}
}
