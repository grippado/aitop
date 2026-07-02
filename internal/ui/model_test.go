package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/grippado/aitop/internal/ui/panes/cards"
)

// fakeCardsBlock builds n synthetic "cards" of cards.CardHeight lines each,
// numbered so assertions can pinpoint exactly which lines survived
// clipping without depending on RenderCard's real output.
func fakeCardsBlock(n int) string {
	var lines []string
	for i := 0; i < n; i++ {
		for l := 0; l < cards.CardHeight; l++ {
			lines = append(lines, "card"+strconv.Itoa(i)+"-line"+strconv.Itoa(l))
		}
	}
	return strings.Join(lines, "\n")
}

func TestClipCardsVertically_FitsNoClipping(t *testing.T) {
	block := fakeCardsBlock(2)
	total := 2 * cards.CardHeight
	got, note := clipCardsVertically(block, 0, false, 2, total)
	if got != block {
		t.Fatalf("expected unclipped block when it already fits")
	}
	if note != "" {
		t.Fatalf("expected no scroll note when unclipped, got %q", note)
	}
}

func TestClipCardsVertically_ScrollsToSelectedTop_List(t *testing.T) {
	block := fakeCardsBlock(5)
	availH := cards.CardHeight + 2 // room for ~1 card plus a sliver, list mode (1 card/row)

	got, note := clipCardsVertically(block, 2, false, 5, availH)
	if note == "" {
		t.Fatalf("expected a scroll note once content is clipped")
	}
	lines := strings.Split(got, "\n")
	if lines[0] != "card2-line0" {
		t.Fatalf("expected the selected card's row to be the topmost visible line, got %q", lines[0])
	}
}

func TestClipCardsVertically_PinsToBottomNearEnd(t *testing.T) {
	block := fakeCardsBlock(5)
	total := 5 * cards.CardHeight
	availH := cards.CardHeight + 2

	// Selecting the last card would scroll past the end if not clamped.
	got, _ := clipCardsVertically(block, 4, false, 5, availH)
	lines := strings.Split(got, "\n")
	wantLastLine := "card4-line" + strconv.Itoa(cards.CardHeight-1)
	if lines[len(lines)-1] != wantLastLine {
		t.Fatalf("expected clipping to pin to the bottom, last line = %q, want %q", lines[len(lines)-1], wantLastLine)
	}
	if len(lines) != availH-1 { // -1: one line reserved for the scroll note
		t.Fatalf("expected exactly availH-1 lines, got %d", len(lines))
	}
	_ = total
}

// fakeGridBlock builds rows of cards.CardHeight lines each, the shape
// cards.RenderGrid actually produces: two cards packed side by side per
// row, so row r's lines represent BOTH card 2r and card 2r+1 at once —
// unlike fakeCardsBlock, which is list-shaped (one row per card).
func fakeGridBlock(rows int) string {
	var lines []string
	for r := 0; r < rows; r++ {
		for l := 0; l < cards.CardHeight; l++ {
			lines = append(lines, "row"+strconv.Itoa(r)+"-line"+strconv.Itoa(l))
		}
	}
	return strings.Join(lines, "\n")
}

func TestClipCardsVertically_GridModeTwoPerRow(t *testing.T) {
	block := fakeGridBlock(2) // card indices 0,1 -> row 0; card index 2 -> row 1
	availH := cards.CardHeight + 1

	// Selecting card index 2 (row 1 in grid mode, since row = selected/2)
	// should scroll to row 1, not to a line offset as if it were list
	// mode's row 2.
	got, _ := clipCardsVertically(block, 2, true, 3, availH)
	lines := strings.Split(got, "\n")
	if lines[0] != "row1-line0" {
		t.Fatalf("expected grid-mode scroll to land on row 1, got %q", lines[0])
	}
}
