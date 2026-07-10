// Package ui holds the root Bubble Tea model. aitop's product is agent
// context, not system resources: the main area is a stack (or grid) of
// per-agent cards, and whole-machine CPU/MEM/NET is a condensed footer,
// not the headline.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/panes/cards"
	"github.com/grippado/aitop/internal/ui/panes/system"
	"github.com/grippado/aitop/internal/ui/theme"
)

// PullFunc fetches the latest snapshot; supplied by main (the demo
// generator or the real collector).
type PullFunc func() domain.Snapshot

// Model is aitop's root Bubble Tea model.
type Model struct {
	theme    theme.Theme
	pull     PullFunc
	refresh  time.Duration
	snapshot domain.Snapshot

	toolFilter string
	sortCol    cards.SortColumn
	selected   int
	grid       bool // list (default) vs grid layout, toggled by 'v'
	paused     bool
	showHelp   bool
	quitting   bool

	width, height int
}

// New builds a Model driven by pull, polled every refresh interval.
func New(pull PullFunc, refresh time.Duration) Model {
	return Model{
		theme:   theme.Default(),
		pull:    pull,
		refresh: refresh,
		sortCol: cards.SortContext,
	}
}

type snapshotMsg domain.Snapshot
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.pullCmd(), tickCmd(m.refresh))
}

func (m Model) pullCmd() tea.Cmd {
	pull := m.pull
	return func() tea.Msg { return snapshotMsg(pull()) }
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tickMsg:
		if m.paused {
			return m, tickCmd(m.refresh)
		}
		return m, tea.Batch(m.pullCmd(), tickCmd(m.refresh))
	case snapshotMsg:
		m.snapshot = domain.Snapshot(msg)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "j", "down":
		m.selected++
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "f":
		m.toolFilter = nextFilter(m.toolFilter, m.snapshot)
		m.selected = 0
	case "o":
		m.sortCol = (m.sortCol + 1) % 4
	case "v":
		m.grid = !m.grid
	case "space":
		m.paused = !m.paused
	case "r":
		return m, m.pullCmd()
	case "?":
		m.showHelp = !m.showHelp
	}
	return m, nil
}

// nextFilter cycles through the distinct tool names present in the
// snapshot, plus "" (= all) as the last step of the cycle.
func nextFilter(cur string, snap domain.Snapshot) string {
	var names []string
	seen := map[string]bool{}
	for _, t := range snap.Tools {
		if !seen[t.Tool] {
			seen[t.Tool] = true
			names = append(names, t.Tool)
		}
	}
	names = append(names, "")
	for i, n := range names {
		if n == cur {
			return names[(i+1)%len(names)]
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.showHelp {
		return m.helpView()
	}

	w := m.width
	if w == 0 {
		w = 100
	}

	var top []string
	if banner := degradedBanner(m.theme, m.snapshot); banner != "" {
		top = append(top, banner)
	}

	cs := cards.BuildCards(m.snapshot, m.toolFilter)
	cards.Sort(cs, m.sortCol)
	if !m.grid {
		// LIST layout nests spawned children under their parent (RFC 0003);
		// GRID stays flat (badge + provenance still show on each card). Done
		// before the selection clamp so m.selected indexes the final order
		// and the scroll math lines up.
		cs = cards.NestByParent(cs)
	}
	if m.selected >= len(cs) {
		m.selected = len(cs) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}

	var cardsBlock string
	if len(cs) == 0 {
		cardsBlock = "no agent sessions detected yet"
	} else if m.grid {
		cardsBlock = cards.RenderGrid(m.theme, cs, m.selected, w)
	} else {
		cardsBlock = cards.RenderList(m.theme, cs, m.selected, w)
	}

	bottom := []string{system.RenderFooter(m.theme, m.snapshot, w), m.footerLine()}

	// tea.WithAltScreen() draws View()'s output verbatim, with no height
	// clamp of its own — confirmed in practice: on a terminal shorter than
	// the full card stack, the tail of the content simply fell off the
	// bottom of the alt-screen, unreachable no matter how the user
	// scrolled. cardsBlock is clipped to whatever vertical room survives
	// the fixed top/bottom sections, auto-scrolled to keep the selected
	// card on screen — every card stays reachable via j/k regardless of
	// terminal height, not just the ones that happen to fit.
	overhead := blockLines(top) + blockLines(bottom)
	var scrollNote string
	if m.height > 0 && len(cs) > 0 {
		cardsBlock, scrollNote = clipCardsVertically(cardsBlock, m.selected, m.grid, len(cs), m.height-overhead)
	}

	var sections []string
	sections = append(sections, top...)
	sections = append(sections, cardsBlock)
	if scrollNote != "" {
		sections = append(sections, scrollNote)
	}
	sections = append(sections, bottom...)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// clipCardsVertically slices cardsBlock down to availH lines when it
// doesn't already fit, scrolled so the selected card's row is the topmost
// visible line — pinned to the bottom instead once scrolling further would
// leave blank space past the last row. No scroll state is stored on Model:
// since the visible window is a pure function of m.selected (which Update
// already persists via j/k), recomputing it fresh every View() call is
// simpler than tracking a separate offset and can't drift out of sync
// with the selection.
func clipCardsVertically(cardsBlock string, selected int, grid bool, cardCount, availH int) (block, note string) {
	if availH < 1 {
		availH = 1
	}
	lines := strings.Split(cardsBlock, "\n")
	if len(lines) <= availH {
		return cardsBlock, ""
	}

	// One line reserved for the scroll indicator itself.
	availH--
	if availH < 1 {
		availH = 1
	}

	row := selected
	if grid {
		row = selected / 2
	}
	scroll := row * cards.CardHeight
	if maxScroll := len(lines) - availH; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	end := scroll + availH
	if end > len(lines) {
		end = len(lines)
	}

	note = fmt.Sprintf("[%d/%d sessions — j/k to scroll]", selected+1, cardCount)
	return strings.Join(lines[scroll:end], "\n"), note
}

func linesOf(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func blockLines(blocks []string) int {
	n := 0
	for _, b := range blocks {
		n += linesOf(b)
	}
	return n
}

func (m Model) footerLine() string {
	layout := "list"
	if m.grid {
		layout = "grid"
	}
	line := fmt.Sprintf("q quit · j/k move · f filter · o sort(%s) · v layout(%s) · space pause · r refresh · ? help", m.sortCol, layout)
	if m.toolFilter != "" {
		line = "[filter: " + m.toolFilter + "] " + line
	}
	if m.paused {
		line = "[PAUSED] " + line
	}
	return line
}

// degradedBanner surfaces any tool whose adapter reported a degraded-but-
// alive state (e.g. Cursor's log format changed) — the honesty rule that
// used to live in the process-table pane's per-tool notes.
func degradedBanner(th theme.Theme, snap domain.Snapshot) string {
	var notes []string
	for _, t := range snap.Tools {
		if t.Note != "" {
			notes = append(notes, "⚠ "+t.Tool+": "+t.Note)
		}
	}
	if len(notes) == 0 {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(th.Warn)
	out := ""
	for i, n := range notes {
		if i > 0 {
			out += "\n"
		}
		out += style.Render(n)
	}
	return out
}

func (m Model) helpView() string {
	return `aitop — keybindings

  q / ctrl+c    quit
  j/k, arrows   move selection
  f             filter by tool
  o             cycle sort column (context / tokens / age / tool)
  v             toggle list/grid layout
  space         pause/resume refresh
  r             force refresh
  ?             toggle this help

Press ? again to return.
`
}
