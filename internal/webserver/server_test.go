package webserver

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grippado/aitop/internal/domain"
)

// fakeSnapshot is a known, hand-built Snapshot — the test never touches a real
// tool directory or the collector, only this fixture through a fake PullFunc.
func fakeSnapshot() domain.Snapshot {
	return domain.Snapshot{
		TakenAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		System:  domain.SystemStats{PerCoreCPUPct: []float64{10, 20}, MemUsedMB: 4096, MemTotalMB: 16384},
		Tools:   []domain.ToolStatus{{Tool: "claude-code", Installed: true, Running: true, SessionCount: 1}},
		Sessions: []domain.SessionInfo{{
			Tool: "claude-code", ID: "sess-1", PID: 4242, Alive: true, Status: "busy",
			Model: "opus 4.8", Title: "wiring the web view",
			TokensIn: 234000, TokensOut: 1200, ContextUsedPct: 23,
			UpdatedAt: time.Date(2026, 7, 14, 11, 59, 0, 0, time.UTC),
		}},
		Usage: []domain.UsageInfo{{Tool: "claude-code", Available: true}},
	}
}

func fakePull() domain.Snapshot { return fakeSnapshot() }

// newTestServer builds a server over a fake pull without starting the poll
// loop, so /api/snapshot and /api/stream both fall through to latest()'s
// on-demand pull. That keeps the test deterministic and time-independent.
func newTestServer(t *testing.T, pull PullFunc) *httptest.Server {
	t.Helper()
	h := newHub(pull, time.Second)
	srv := httptest.NewServer(handler(h))
	t.Cleanup(srv.Close)
	return srv
}

func TestSnapshotEndpoint(t *testing.T) {
	srv := newTestServer(t, fakePull)

	resp, err := http.Get(srv.URL + "/api/snapshot")
	if err != nil {
		t.Fatalf("GET /api/snapshot: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got domain.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].ID != "sess-1" {
		t.Fatalf("sessions = %+v, want one sess-1", got.Sessions)
	}
	if got.Sessions[0].ContextUsedPct != 23 {
		t.Errorf("context = %v, want 23", got.Sessions[0].ContextUsedPct)
	}
}

func TestStreamEmitsSnapshot(t *testing.T) {
	srv := newTestServer(t, fakePull)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/stream: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// The connection is primed with the current snapshot immediately, so the
	// first "data:" frame must arrive without waiting for a poll tick.
	sc := bufio.NewScanner(resp.Body)
	var payload string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			payload = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if payload == "" {
		t.Fatal("no data frame received from /api/stream")
	}
	var got domain.Snapshot
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("stream frame not valid JSON: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].ID != "sess-1" {
		t.Errorf("stream sessions = %+v, want one sess-1", got.Sessions)
	}
}

func TestServesIndex(t *testing.T) {
	srv := newTestServer(t, fakePull)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "<!doctype html>") {
		t.Errorf("index.html not served (got %q)", string(buf[:n]))
	}
}

func TestReadOnlyRejectsMutations(t *testing.T) {
	srv := newTestServer(t, fakePull)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, _ := http.NewRequest(method, srv.URL+"/api/snapshot", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s status = %d, want 405", method, resp.StatusCode)
		}
	}
}

// TestSinglePollFanout asserts the hub polls once per publish and every SSE
// subscriber sees that snapshot — i.e. N clients don't cause N pulls per tick.
func TestSinglePollFanout(t *testing.T) {
	var pulls int64
	pull := func() domain.Snapshot {
		atomic.AddInt64(&pulls, 1)
		return fakeSnapshot()
	}
	h := newHub(pull, time.Second)

	subs := []chan domain.Snapshot{h.subscribe(), h.subscribe(), h.subscribe()}
	before := atomic.LoadInt64(&pulls)
	h.publish(h.pull()) // one pull, fanned out to all three
	if got := atomic.LoadInt64(&pulls) - before; got != 1 {
		t.Fatalf("pulls during one publish = %d, want 1", got)
	}
	for i, ch := range subs {
		select {
		case snap := <-ch:
			if len(snap.Sessions) != 1 {
				t.Errorf("sub %d got %d sessions, want 1", i, len(snap.Sessions))
			}
		default:
			t.Errorf("sub %d received no broadcast", i)
		}
	}
}
