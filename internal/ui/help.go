package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type helpSection struct {
	title string
	binds [][2]string
}

var (
	styleHelpBox     lipgloss.Style
	styleHelpHeading lipgloss.Style
	styleHelpSection lipgloss.Style
	styleHelpKey     lipgloss.Style
	styleHelpDesc    lipgloss.Style
)

func initHelpStyles() {
	t := activeTheme
	styleHelpBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(1, 3)

	styleHelpHeading = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)

	styleHelpSection = lipgloss.NewStyle().
		Foreground(t.FgDim).
		Bold(true)

	styleHelpKey = lipgloss.NewStyle().
		Foreground(t.SynSection).
		Width(16)

	styleHelpDesc = lipgloss.NewStyle().
		Foreground(t.FgBase)
}

var (
	helpBrowser = []helpSection{
		{title: "Navigation", binds: [][2]string{
			{"j / k", "move down / up"},
			{"g / G", "first / last"},
		}},
		{title: "Actions", binds: [][2]string{
			{"enter / l", "open directory or file"},
			{"h / - / ←", "go up one level"},
			{"/", "search entries"},
			{"q", "quit"},
		}},
	}

	helpFileView = []helpSection{
		{title: "Tabs", binds: [][2]string{
			{"1 – 7", "switch tab (definition/request/response/status/tests/logs/example)"},
			{"[ / ]", "previous / next tab"},
			{"tab", "toggle list / env focus"},
		}},
		{title: "Navigation", binds: [][2]string{
			{"j / k", "move / scroll (focused panel)"},
			{"g / G", "first / last"},
			{"ctrl+d / u", "scroll preview"},
			{"/", "search requests"},
		}},
		{title: "Actions (request list)", binds: [][2]string{
			{"enter / l", "expand group or inspect parts"},
			{"r", "run request"},
			{"t", "run with tests"},
			{"o", "open body in viewer"},
			{"e", "edit selected request block"},
			{"E", "edit file (context-aware)"},
			{"x", "save example"},
			{"h / esc", "go back"},
			{"q", "quit"},
		}},
		{title: "Actions (env panel)", binds: [][2]string{
			{"j / k", "move cursor"},
			{"e / enter", "edit value in $EDITOR (session only)"},
			{"tab / esc", "return to request list"},
		}},
	}

	helpPartsView = []helpSection{
		{title: "Tabs", binds: [][2]string{
			{"1 – 7", "switch tab (definition/request/response/status/tests/logs/example)"},
			{"[ / ]", "previous / next tab"},
			{"tab", "toggle parts / env focus"},
		}},
		{title: "Navigation", binds: [][2]string{
			{"j / k", "move / scroll (focused panel)"},
			{"ctrl+d / u", "scroll preview"},
		}},
		{title: "Actions (parts list)", binds: [][2]string{
			{"e / enter / l", "edit selected part"},
			{"r", "run request"},
			{"t", "run with tests"},
			{"o", "open body in viewer"},
			{"x", "save example"},
			{"h / esc", "go back"},
			{"q", "quit"},
		}},
		{title: "Actions (env panel)", binds: [][2]string{
			{"j / k", "move cursor"},
			{"e / enter", "edit value in $EDITOR (session only)"},
			{"tab / esc", "return to parts list"},
		}},
	}
)

// renderHelpOverlay renders a centered floating help box over a dimmed background.
func renderHelpOverlay(w, h int, sections []helpSection) string {
	var b strings.Builder
	b.WriteString(styleHelpHeading.Render("Keybindings") + "\n")
	for _, sec := range sections {
		b.WriteString("\n" + styleHelpSection.Render(sec.title) + "\n")
		for _, bind := range sec.binds {
			b.WriteString(styleHelpKey.Render(bind[0]) + styleHelpDesc.Render(bind[1]) + "\n")
		}
	}
	b.WriteString("\n" + styleDim.Render("any key to close"))

	box := styleHelpBox.Render(strings.TrimRight(b.String(), "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceBackground(activeTheme.BgDimmer))
}
