package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	leftPanelRatio = 3 // left panel takes 1/leftPanelRatio of total width
	headerHeight   = 1
	statusHeight   = 1
	panelPadding   = 0
)

var (
	styleSelected  lipgloss.Style
	styleCursor    lipgloss.Style
	styleDim       lipgloss.Style
	styleDir       lipgloss.Style
	styleHeader    lipgloss.Style
	styleStatusBar lipgloss.Style
	styleDivider   lipgloss.Style
	styleTabActive lipgloss.Style
	styleTabInactive lipgloss.Style
	styleMethod    map[string]lipgloss.Style
)

func initStyles() {
	t := activeTheme
	styleSelected = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)

	styleCursor = lipgloss.NewStyle().
		Background(t.BgSubtle).
		Foreground(t.Accent).
		Bold(true)

	styleDim = lipgloss.NewStyle().
		Foreground(t.FgDim)

	styleDir = lipgloss.NewStyle().
		Foreground(t.Dir).
		Bold(true)

	styleHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.FgBase).
		Background(t.BgPanel).
		Padding(0, 1)

	styleStatusBar = lipgloss.NewStyle().
		Background(t.BgSubtle).
		Foreground(t.FgDim).
		Padding(0, 1)

	styleDivider = lipgloss.NewStyle().
		Foreground(t.FgFaint)

	styleTabActive = lipgloss.NewStyle().
		Background(t.BgPanel).
		Foreground(t.Accent).
		Bold(true).
		Padding(0, 1)

	styleTabInactive = lipgloss.NewStyle().
		Background(t.BgDimmer).
		Foreground(t.FgDim).
		Padding(0, 1)

	styleMethod = map[string]lipgloss.Style{
		"GET":    lipgloss.NewStyle().Foreground(t.MethodGET),
		"POST":   lipgloss.NewStyle().Foreground(t.MethodPOST),
		"PUT":    lipgloss.NewStyle().Foreground(t.MethodPUT),
		"PATCH":  lipgloss.NewStyle().Foreground(t.MethodPATCH),
		"DELETE": lipgloss.NewStyle().Foreground(t.MethodDELETE),
	}
}

func leftWidth(total int) int {
	return total / leftPanelRatio
}

func rightWidth(total int) int {
	return total - leftWidth(total) - 1 // -1 for divider column
}

func contentHeight(total int) int {
	return total - headerHeight - statusHeight - panelPadding
}

// renderTabBar draws the five right-panel tabs and returns a line of width w.
func renderTabBar(activeTab, w int) string {
	labels := []string{"1 request", "2 run", "3 tests", "4 logs", "5 example"}
	var parts []string
	for i, label := range labels {
		if i == activeTab {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTabInactive.Render(label))
		}
	}
	bar := strings.Join(parts, styleDivider.Render("│"))
	return lipgloss.NewStyle().Width(w).Background(activeTheme.BgDimmer).Render(bar)
}

// renderEnvPanel renders the env state panel into exactly h rows of visual width w.
// Row 0 is a focus-sensitive header; rows 1..h-1 are scrollable variable entries.
// This is shared between FileView and PartsView.
func renderEnvPanel(env map[string]string, focused bool, scroll, cursor, w, h int) []string {
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	if h == 0 {
		return lines
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	visRows := h - 1
	scrollSuffix := ""
	if len(keys) > visRows {
		above := scroll > 0
		below := scroll+visRows < len(keys)
		switch {
		case above && below:
			scrollSuffix = " ↑↓"
		case above:
			scrollSuffix = " ↑"
		case below:
			scrollSuffix = " ↓"
		}
	}

	var title string
	if len(keys) == 0 {
		title = "env"
	} else {
		title = fmt.Sprintf("env (%d)%s", len(keys), scrollSuffix)
	}
	if focused {
		lines[0] = styleTabActive.Width(w).Render(title)
	} else {
		lines[0] = styleTabInactive.Width(w).Render(title)
	}

	if h <= 1 {
		return lines
	}

	if len(keys) == 0 {
		empty := ansi.Truncate("  "+styleDim.Render("(run a request to populate)"), w, "")
		lines[1] = lipgloss.NewStyle().Width(w).Render(empty)
		return lines
	}

	for screenIdx := 1; screenIdx < h; screenIdx++ {
		dataIdx := scroll + screenIdx - 1
		if dataIdx >= len(keys) {
			break
		}
		k := keys[dataIdx]
		v := env[k]
		line := "  " + clrKeyword.Render(k) + styleDim.Render(" = ") + clrHeaderVal.Render(v)
		if focused && dataIdx == cursor {
			lines[screenIdx] = styleCursor.Width(w).Render(ansi.Truncate("  "+k+" = "+v, w, ""))
		} else {
			lines[screenIdx] = lipgloss.NewStyle().Width(w).Render(
				ansi.Truncate(line, w, ""),
			)
		}
	}
	return lines
}

// renderStatusBar builds a full-width status bar line where every character has
// the given background explicitly set. This avoids the lipgloss nesting problem
// where an inner ANSI reset clears the outer background for subsequent text and
// for the width-filling padding that the outer Render() appends.
func renderStatusBar(bg lipgloss.Color, content string, width int) string {
	base := lipgloss.NewStyle().Background(bg)
	leftPad := base.Render(" ")
	vw := 1 + lipgloss.Width(content)
	rightFill := ""
	if width > vw {
		rightFill = base.Render(strings.Repeat(" ", width-vw))
	}
	return leftPad + content + rightFill
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

// zipPanels stitches left and right line slices side-by-side with a divider.
// Every right-side line is padded to exactly rw visible characters so that
// stale terminal content from a previous (wider) render is fully overwritten.
func zipPanels(left, right []string, lw, rw, h int) string {
	div := styleDivider.Render("│")
	blankL := strings.Repeat(" ", lw)
	blankR := strings.Repeat(" ", rw)
	rows := make([]string, h)
	for i := 0; i < h; i++ {
		l := blankL
		if i < len(left) {
			l = left[i]
		}
		r := blankR
		if i < len(right) {
			line := right[i]
			vw := lipgloss.Width(line)
			if vw < rw {
				line += strings.Repeat(" ", rw-vw)
			}
			r = line
		}
		rows[i] = l + div + r
	}
	return strings.Join(rows, "\n")
}
