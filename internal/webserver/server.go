// Package webserver serves aitop's read-only browser view: the same
// domain.Snapshot the TUI renders, exposed over HTTP as JSON plus a
// Server-Sent Events stream, with a self-contained SPA (embedded via
// go:embed) on top. See docs/rfcs/0004-aitop-web-view.md.
//
// Invariants this package upholds (they are the whole point):
//   - Read-only. Only GET routes; nothing mutates tool state. Default bind is
//     127.0.0.1, never 0.0.0.0 — routable exposure is an explicit user choice.
//   - No new dependencies, no cgo. Only net/http + embed from the stdlib, so
//     the CGO_ENABLED=0 release build is untouched.
//   - Never fabricate. This layer serializes a Snapshot verbatim; the "—"
//     honesty lives in the frontend, which renders missing fields as a dash,
//     never a zero. The server invents nothing.
//   - No tool-state reads. Unlike an adapter, the server never opens a tool
//     directory — the Reader rule (invariant #3) simply doesn't apply here
//     because there is no filesystem access to gate.
package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// PullFunc fetches the latest snapshot. It is the exact same signature the TUI
// uses (ui.PullFunc); declared locally so this package doesn't import
// internal/ui (which would be a cycle and pull the whole Bubble Tea tree in).
type PullFunc func() domain.Snapshot

// Run starts the read-only web dashboard and blocks until the server stops
// (i.e. until the process is killed). pull is polled once per refresh by a
// single hub goroutine and fanned out to every connected SSE client. addr is
// a host:port; callers should default it to a loopback address.
func Run(pull PullFunc, refresh time.Duration, addr string) error {
	h := newHub(pull, refresh)
	go h.loop()

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler(h),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}

// handler wires the read-only routes onto a hub. Split out from Run so tests
// can exercise the HTTP surface with httptest, without binding a port or
// running the poll loop.
func handler(h *hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", h.handleSnapshot)
	mux.HandleFunc("/api/stream", h.handleStream)
	mux.Handle("/", assetsHandler())
	return getOnly(mux)
}

// getOnly rejects any method other than GET/HEAD, enforcing the read-only
// contract at the transport edge — no handler can accidentally accept a
// mutating request because none is ever routed.
func getOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "aitop serve is read-only", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hub owns the single poll loop. One ticker calls pull() at the refresh
// cadence, caches the latest Snapshot, and broadcasts it to every subscribed
// SSE client — so N browser tabs cost one poll, not N (collector.Snapshot
// samples CPU for 200ms and documents that it isn't built for concurrent
// callers).
type hub struct {
	pull    PullFunc
	refresh time.Duration

	mu      sync.RWMutex
	last    domain.Snapshot
	hasLast bool

	subMu sync.Mutex
	subs  map[chan domain.Snapshot]struct{}
}

func newHub(pull PullFunc, refresh time.Duration) *hub {
	if refresh <= 0 {
		refresh = 2 * time.Second
	}
	return &hub{
		pull:    pull,
		refresh: refresh,
		subs:    make(map[chan domain.Snapshot]struct{}),
	}
}

// loop is the sole caller of pull() on the timed path. It polls once
// immediately (so the first client doesn't wait a full refresh for data) and
// then every refresh, publishing each snapshot to subscribers.
func (h *hub) loop() {
	h.publish(h.pull())
	t := time.NewTicker(h.refresh)
	defer t.Stop()
	for range t.C {
		h.publish(h.pull())
	}
}

func (h *hub) publish(snap domain.Snapshot) {
	h.mu.Lock()
	h.last = snap
	h.hasLast = true
	h.mu.Unlock()

	h.subMu.Lock()
	for ch := range h.subs {
		// Non-blocking send: a slow/stuck client must never wedge the poll
		// loop. It just misses this tick and gets the next one.
		select {
		case ch <- snap:
		default:
		}
	}
	h.subMu.Unlock()
}

// latest returns the cached snapshot, pulling once on demand if the loop
// hasn't produced one yet (e.g. a request that races startup).
func (h *hub) latest() domain.Snapshot {
	h.mu.RLock()
	if h.hasLast {
		defer h.mu.RUnlock()
		return h.last
	}
	h.mu.RUnlock()
	return h.pull()
}

func (h *hub) subscribe() chan domain.Snapshot {
	ch := make(chan domain.Snapshot, 1)
	h.subMu.Lock()
	h.subs[ch] = struct{}{}
	h.subMu.Unlock()
	return ch
}

func (h *hub) unsubscribe(ch chan domain.Snapshot) {
	h.subMu.Lock()
	delete(h.subs, ch)
	h.subMu.Unlock()
}

// handleSnapshot serves one snapshot as indented JSON — byte-for-byte the same
// shape (and indentation) as `aitop --once --json`, so the two surfaces stay a
// single contract.
func (h *hub) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(h.latest())
}

// handleStream serves the live Snapshot feed as Server-Sent Events: the cached
// snapshot immediately, then one compact-JSON event per poll tick. Compact
// (not indented) JSON is required — an SSE data field is newline-delimited, so
// embedded newlines would corrupt the frame.
func (h *hub) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	// Prime the connection with the current snapshot so a freshly-opened tab
	// paints immediately instead of waiting for the next tick.
	writeEvent(w, h.latest())
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case snap := <-ch:
			if !writeEvent(w, snap) {
				return
			}
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, snap domain.Snapshot) bool {
	b, err := json.Marshal(snap)
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", b)
	return err == nil
}
