// Package ui holds the root Bubble Tea model that composes the 5 boxes.
package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grippado/aitop/internal/domain"
	"github.com/grippado/aitop/internal/ui/panes/agents"
	"github.com/grippado/aitop/internal/ui/panes/system"
	"github.com/grippado/aitop/internal/ui/panes/tools"
	"github.com/grippado/aitop/internal/ui/panes/usage"
	"github.com/grippado/aitop/internal/ui/theme"
)

// PullFunc fetches the latest snapshot; supplied by main (the demo generator
// in v1's first milestone, later the real collector).
type PullFunc func() domain.Snapshot

// Model is aitop's root Bubble Tea model.
type Model struct {
	theme    theme.Theme
	pull     PullFunc
	refresh  time.Duration
	snapshot domain.Snapshot

	focus       int // 1-5, which box has keyboard focus
	toolFilter  string
	sortCol     agents.SortColumn
	selected    int
	paused      bool
	usageExpand bool
	showHelp    bool
	quitting    bool

	width, height int
}

// New builds a Model driven by pull, polled every refresh interval.
func New(pull PullFunc, refresh time.Duration) Model {
	return Model{
		theme:   theme.Default(),
		pull:    pull,
		refresh: refresh,
		focus:   4,
		sortCol: agents.SortCPU,
	}
}

type snapshotMsg domain.Snapshot
type tickMsg time.Time

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.pullCmd(), tickCmd(m.refresh))
}

func (m Model) pullCmd() tea.Cmd {
	pull := m.pull
	return func() tea.Msg {
		return snapshotMsg(pull())
	}
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
	case "tab":
		m.focus = m.focus%5 + 1
	case "shift+tab":
		m.focus = (m.focus+3)%5 + 1
	case "1", "2", "3", "4", "5":
		m.focus = int(msg.String()[0] - '0')
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
	case " ":
		m.paused = !m.paused
	case "u":
		m.usageExpand = !m.usageExpand
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
	halfW := w/2 - 1

	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		system.RenderCPU(m.theme, m.snapshot, halfW, 8, m.focus == 1),
		system.RenderMemNet(m.theme, m.snapshot, halfW, 8, m.focus == 2),
	)
	row2 := tools.Render(m.theme, m.snapshot, w, 3+len(m.snapshot.Tools), m.focus == 3)

	rows := agents.BuildRows(m.snapshot, m.toolFilter)
	agents.Sort(rows, m.sortCol)
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	row3 := agents.Render(m.theme, rows, m.selected, m.sortCol, w, 12, m.focus == 4)

	row4 := usage.Render(m.theme, m.snapshot, m.usageExpand, w, 3, m.focus == 5)

	footer := "q quit · tab focus · 1-5 jump · j/k move · f filter · o sort · space pause · u usage · r refresh · ? help"
	if m.toolFilter != "" {
		footer = "[filter: " + m.toolFilter + "] " + footer
	}
	if m.paused {
		footer = "[PAUSED] " + footer
	}

	return lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3, row4, footer)
}

func (m Model) helpView() string {
	return `aitop — keybindings

  q / ctrl+c    quit
  tab/shift+tab cycle pane focus
  1-5           jump to box
  j/k, arrows   move selection
  f             filter by tool
  o             cycle sort column
  space         pause/resume refresh
  u             expand/collapse usage panel
  r             force refresh
  ?             toggle this help

Press ? again to return.
`
}
