package cards

import (
	"strings"
	"testing"

	"github.com/grippado/aitop/internal/ui/theme"
)

func TestRenderContextLine_OmitsFabricatedRatioWithoutRealTokens(t *testing.T) {
	th := theme.Default()

	// Cursor's ContextPct comes from its own independent reading, with no
	// guaranteed TokensIn to match — showing "0/0" here would be a
	// fabricated ratio, not a real one (the actual bug reported).
	c := Card{ContextPct: 35, HasTokens: false}
	got := renderContextLine(th, c, 80)
	if strings.Contains(got, "0/0") {
		t.Fatalf("renderContextLine() = %q, must not fabricate a 0/0 ratio", got)
	}
	if !strings.Contains(got, "35%") {
		t.Fatalf("renderContextLine() = %q, want it to still show the real percentage", got)
	}
}

func TestRenderContextLine_ShowsRealRatioWhenTokensAreKnown(t *testing.T) {
	th := theme.Default()

	c := Card{ContextPct: 50, HasTokens: true, TokensIn: 500}
	got := renderContextLine(th, c, 80)
	if !strings.Contains(got, "500/1k") {
		t.Fatalf("renderContextLine() = %q, want the real USED/TOTAL ratio 500/1k (500 tokens is 50%% of a 1k window)", got)
	}
}
