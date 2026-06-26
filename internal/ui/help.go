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
	styleHelpBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("212")).
			Padding(1, 3)

	styleHelpHeading = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	styleHelpSection = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Bold(true)

	styleHelpKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Width(16)

	styleHelpDesc = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

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
			{"1 – 5", "jump to tab"},
			{"tab", "cycle to next tab"},
		}},
		{title: "Navigation", binds: [][2]string{
			{"j / k", "move down / up"},
			{"g / G", "first / last"},
			{"ctrl+d / u", "scroll preview"},
			{"/", "search requests"},
		}},
		{title: "Actions", binds: [][2]string{
			{"enter / l", "inspect request parts"},
			{"r", "run request"},
			{"t", "run with tests"},
			{"o", "open body in viewer"},
			{"e", "edit whole file"},
			{"E", "edit request block"},
			{"x", "save example"},
			{"h / esc", "go back"},
			{"q", "quit"},
		}},
	}

	helpPartsView = []helpSection{
		{title: "Tabs", binds: [][2]string{
			{"1 – 5", "jump to tab"},
			{"tab", "cycle to next tab"},
		}},
		{title: "Navigation", binds: [][2]string{
			{"j / k", "move between parts"},
			{"ctrl+d / u", "scroll preview"},
		}},
		{title: "Actions", binds: [][2]string{
			{"e / enter / l", "edit selected part"},
			{"r", "run request"},
			{"t", "run with tests"},
			{"o", "open body in viewer"},
			{"x", "save example"},
			{"h / esc", "go back"},
			{"q", "quit"},
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
		lipgloss.WithWhitespaceBackground(lipgloss.Color("235")))
}
