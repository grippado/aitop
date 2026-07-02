package cards

import (
	"testing"

	"github.com/grippado/aitop/internal/domain"
)

func TestBuildCards_DropsCursorDuplicateOfLiveCursorAgentSession(t *testing.T) {
	snap := domain.Snapshot{
		Sessions: []domain.SessionInfo{
			{Tool: "cursor", ID: "shared-id", Alive: true, Title: "From composer store", Status: "busy"},
			{Tool: "cursor-agent", ID: "shared-id", Alive: true, Title: "From its own transcript", Status: "busy"},
		},
	}

	cs := BuildCards(snap, "")
	if len(cs) != 1 {
		t.Fatalf("expected exactly 1 card for the shared session ID, got %d: %+v", len(cs), cs)
	}
	if cs[0].Tool != "cursor-agent" {
		t.Fatalf("expected cursor-agent's card to win (richer per-session source), got tool=%q", cs[0].Tool)
	}
}

func TestBuildCards_KeepsCursorCardWhenNoLiveCursorAgentMatch(t *testing.T) {
	snap := domain.Snapshot{
		Sessions: []domain.SessionInfo{
			// cursor-agent's own process exited (task finished from the
			// CLI's perspective) but Cursor IDE still has the composer
			// open — the IDE card must take over, not disappear.
			{Tool: "cursor", ID: "shared-id", Alive: true, Title: "Still visible in the IDE", Status: "busy"},
		},
	}

	cs := BuildCards(snap, "")
	if len(cs) != 1 || cs[0].Tool != "cursor" {
		t.Fatalf("expected the cursor card to survive with no cursor-agent session to dedup against, got %+v", cs)
	}
}

func TestBuildCards_DoesNotDedupDifferentSessionIDs(t *testing.T) {
	snap := domain.Snapshot{
		Sessions: []domain.SessionInfo{
			{Tool: "cursor", ID: "id-a", Alive: true, Status: "busy"},
			{Tool: "cursor-agent", ID: "id-b", Alive: true, Status: "busy"},
		},
	}

	cs := BuildCards(snap, "")
	if len(cs) != 2 {
		t.Fatalf("expected 2 unrelated cards (different session IDs), got %d: %+v", len(cs), cs)
	}
}

func TestBuildCards_EmptyIDsNeverDedupAgainstEachOther(t *testing.T) {
	// Both adapters can legitimately report ID == "" (no session-tagged
	// data resolved yet) — that must never be treated as a shared ID.
	snap := domain.Snapshot{
		Sessions: []domain.SessionInfo{
			{Tool: "cursor", ID: "", Alive: true, Status: "busy"},
			{Tool: "cursor-agent", ID: "", Alive: true, Status: "busy"},
		},
	}

	cs := BuildCards(snap, "")
	if len(cs) != 2 {
		t.Fatalf("expected both empty-ID sessions to keep their own cards, got %d: %+v", len(cs), cs)
	}
}
