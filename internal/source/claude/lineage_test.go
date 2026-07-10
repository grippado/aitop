package claude

import (
	"context"
	"testing"

	"github.com/grippado/aitop/internal/domain"
)

// withFakePpid swaps the package-level ppidOf hook for a synthetic process
// tree so the PPID walk never touches live gopsutil (golden invariant 3), and
// restores the original after the test.
func withFakePpid(t *testing.T, tree map[int]int) {
	t.Helper()
	orig := ppidOf
	ppidOf = func(_ context.Context, pid int) (int, bool) {
		ppid, ok := tree[pid]
		return ppid, ok
	}
	t.Cleanup(func() { ppidOf = orig })
}

// TestSessions_ParsesKind proves the "kind" tag is read verbatim from the
// session file into SessionInfo.Kind — "bg" and "interactive" round-trip, and
// an absent key stays the honest empty string (invariant 2), never a guess.
func TestSessions_ParsesKind(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		wantKind string
	}{
		{"bg", `{"pid":111,"sessionId":"s1","cwd":"/x","status":"idle","kind":"bg","updatedAt":1000}`, "bg"},
		{"interactive", `{"pid":222,"sessionId":"s2","cwd":"/y","status":"busy","kind":"interactive","updatedAt":1000}`, "interactive"},
		{"absent", `{"pid":333,"sessionId":"s3","cwd":"/z","status":"idle","updatedAt":1000}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configDir := "/home/test/.claude"
			f := &fakeReader{
				dirs: map[string][]string{
					configDir + "/sessions": {"one.json"},
				},
				files: map[string][]byte{
					configDir + "/sessions/one.json": []byte(tc.raw),
				},
			}
			withFakeReader(t, f)
			// Neutralize the trailing walk: no live gopsutil, walk ends immediately.
			withFakePpid(t, map[int]int{})

			a := &Adapter{configDir: configDir, transcript: newTranscriptTracker()}
			sessions, err := a.Sessions(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(sessions) != 1 {
				t.Fatalf("expected 1 session, got %d", len(sessions))
			}
			if got := sessions[0].Kind; got != tc.wantKind {
				t.Fatalf("Kind = %q, want %q", got, tc.wantKind)
			}
		})
	}
}

// aliveSessions builds a tracked session set with Alive forced on, so the walk
// runs against the synthetic ppidOf tree and never consults live gopsutil for
// process existence.
func aliveSessions(pids ...int) []domain.SessionInfo {
	out := make([]domain.SessionInfo, 0, len(pids))
	for _, p := range pids {
		out = append(out, domain.SessionInfo{Tool: Name, PID: p, Alive: true})
	}
	return out
}

func find(sessions []domain.SessionInfo, pid int) domain.SessionInfo {
	for _, s := range sessions {
		if s.PID == pid {
			return s
		}
	}
	return domain.SessionInfo{}
}

// TestResolveParentPIDs_WalksThroughUntrackedAncestors is the load-bearing
// case: a bg session's chain passes through UNTRACKED host PIDs before reaching
// a tracked interactive session. The walk must NOT stop at the first
// non-session ancestor — it keeps going and lands on the first TRACKED PID.
func TestResolveParentPIDs_WalksThroughUntrackedAncestors(t *testing.T) {
	const (
		interactive = 100 // tracked parent
		bg          = 200 // tracked child
		host1       = 300 // untracked "claude" host
		host2       = 400 // untracked bg-pty-host
	)
	// bg -> host1 -> host2 -> interactive -> init(1)
	withFakePpid(t, map[int]int{
		bg:          host1,
		host1:       host2,
		host2:       interactive,
		interactive: 1,
	})

	sessions := aliveSessions(interactive, bg)
	resolveParentPIDs(context.Background(), sessions)

	if got := find(sessions, bg).ParentPID; got != interactive {
		t.Fatalf("bg ParentPID = %d, want %d (must walk through untracked hosts)", got, interactive)
	}
	if got := find(sessions, interactive).ParentPID; got != 0 {
		t.Fatalf("interactive ParentPID = %d, want 0 (a root — its chain reaches init)", got)
	}
}

// TestResolveParentPIDs_OrphanStaysZero covers both honest-zero exits: a chain
// that leaves the process tree (ok=false) and a chain that only ever passes
// through untracked PIDs before reaching init — neither may fabricate a parent.
func TestResolveParentPIDs_OrphanStaysZero(t *testing.T) {
	t.Run("leaves tree (ok=false)", func(t *testing.T) {
		const session = 500
		// 500 -> 600 (untracked), then ppidOf(600) reports ok=false.
		withFakePpid(t, map[int]int{session: 600})
		sessions := aliveSessions(session)
		resolveParentPIDs(context.Background(), sessions)
		if got := find(sessions, session).ParentPID; got != 0 {
			t.Fatalf("ParentPID = %d, want 0 when the walk leaves the process tree", got)
		}
	})

	t.Run("only untracked ancestors then init", func(t *testing.T) {
		const session = 510
		withFakePpid(t, map[int]int{session: 610, 610: 620, 620: 1})
		sessions := aliveSessions(session)
		resolveParentPIDs(context.Background(), sessions)
		if got := find(sessions, session).ParentPID; got != 0 {
			t.Fatalf("ParentPID = %d, want 0 when no tracked ancestor exists", got)
		}
	})
}

// TestResolveParentPIDs_SelfReferenceGuard ensures a session is never assigned
// its own PID, even when its immediate parent reports as itself.
func TestResolveParentPIDs_SelfReferenceGuard(t *testing.T) {
	const session = 700
	withFakePpid(t, map[int]int{session: session}) // direct self-loop
	sessions := aliveSessions(session)
	resolveParentPIDs(context.Background(), sessions)
	got := find(sessions, session).ParentPID
	if got == session {
		t.Fatalf("ParentPID = %d — a session must never be its own parent", got)
	}
	if got != 0 {
		t.Fatalf("ParentPID = %d, want 0 for a self-looping chain", got)
	}
}

// TestResolveParentPIDs_Bounded proves the walk terminates on pathological
// trees: a cycle and a chain longer than the hop cap both yield ParentPID 0
// rather than spinning forever.
func TestResolveParentPIDs_Bounded(t *testing.T) {
	t.Run("cycle among untracked ancestors", func(t *testing.T) {
		const session = 800
		// 800 -> 801 -> 802 -> 801 (cycle); none but 800 is tracked.
		withFakePpid(t, map[int]int{session: 801, 801: 802, 802: 801})
		sessions := aliveSessions(session)
		done := make(chan struct{})
		go func() {
			resolveParentPIDs(context.Background(), sessions)
			close(done)
		}()
		<-done // if the walk weren't bounded this test would hang, not pass
		if got := find(sessions, session).ParentPID; got != 0 {
			t.Fatalf("ParentPID = %d, want 0 for a cyclic chain", got)
		}
	})

	t.Run("chain longer than the cap", func(t *testing.T) {
		const session = 900
		// A linear chain of untracked hosts, with a tracked PID placed BEYOND
		// the cap so the walk terminates before it can ever be reached.
		const tracked = 100000
		tree := make(map[int]int)
		pid := session
		for i := 0; i < maxParentWalkHops+5; i++ {
			tree[pid] = pid + 1
			pid++
		}
		tree[pid] = tracked // reachable only past the cap
		tree[tracked] = 1

		sessions := aliveSessions(session, tracked)
		resolveParentPIDs(context.Background(), sessions)
		if got := find(sessions, session).ParentPID; got != 0 {
			t.Fatalf("ParentPID = %d, want 0 — a chain past the hop cap must not resolve", got)
		}
	})
}
