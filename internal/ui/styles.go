package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	leftPanelRatio = 3 // left panel takes 1/leftPanelRatio of total width
	headerHeight   = 1
	statusHeight   = 1
	panelPadding   = 0 // no borders on panels
)

var (
	styleSelected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	styleCursor = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("212")).
			Bold(true)

	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))

	styleDir = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")).
			Bold(true)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("238")).
			Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("244")).
			Padding(0, 1)

	styleDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	styleMethod = map[string]lipgloss.Style{
		"GET":    lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		"POST":   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		"PUT":    lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
		"PATCH":  lipgloss.NewStyle().Foreground(lipgloss.Color("99")),
		"DELETE": lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
	}
)

func leftWidth(total int) int {
	return total / leftPanelRatio
}

func rightWidth(total int) int {
	return total - leftWidth(total) - 1 // -1 for divider column
}

func contentHeight(total int) int {
	return total - headerHeight - statusHeight - panelPadding
}

// truncate shortens s to maxWidth runes, appending "…" if cut.
func truncate(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	return string(runes[:maxWidth-1]) + "…"
}

// zipPanels stitches left and right line slices side-by-side with a divider
// column. Both slices should already be padded/truncated to their panel widths.
func zipPanels(left, right []string, lw, h int) string {
	div := styleDivider.Render("│")
	rows := make([]string, h)
	blank := strings.Repeat(" ", lw)
	for i := 0; i < h; i++ {
		l := blank
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		rows[i] = l + div + r
	}
	return strings.Join(rows, "\n")
}
