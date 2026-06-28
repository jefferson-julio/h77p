package ui

import "github.com/charmbracelet/lipgloss"

// Theme holds every colour used across the TUI and syntax highlighter.
// Using hex (#RRGGBB) gives exact colours in truecolor terminals;
// lipgloss/termenv degrades them automatically to 256-color when needed.
type Theme struct {
	// Chrome backgrounds
	BgPanel  lipgloss.Color // header bar, tab bar
	BgSubtle lipgloss.Color // cursor row, status bar
	BgDimmer lipgloss.Color // overlay dim, inactive tab bg

	// Foregrounds
	FgBase  lipgloss.Color // primary text
	FgDim   lipgloss.Color // status bar text, inactive tab label, help section heading
	FgFaint lipgloss.Color // dividers, delimiters

	// Accent
	Accent lipgloss.Color // cursor fg, selected, active tab, help box border/heading
	Dir    lipgloss.Color // directory entry label

	// .http tokeniser colours
	SynSection    lipgloss.Color // ### separator lines
	SynKeyword    lipgloss.Color // @tags (@pre-request, @example …)
	SynHeaderKey  lipgloss.Color // HTTP header name
	SynHeaderVal  lipgloss.Color // HTTP header value / URL text
	SynVar        lipgloss.Color // {{variable}} interpolations
	SynInlineExpr lipgloss.Color // ${{js expression}} tokens

	// HTTP method colours
	MethodGET    lipgloss.Color
	MethodPOST   lipgloss.Color
	MethodPUT    lipgloss.Color
	MethodPATCH  lipgloss.Color
	MethodDELETE lipgloss.Color

	// HTTP status-code colours
	Status2xx lipgloss.Color
	Status3xx lipgloss.Color
	Status4xx lipgloss.Color
	Status5xx lipgloss.Color

	// Chroma theme name used for JS / JSON / XML body highlighting
	ChromaStyle string
}

// ── Three built-in themes ────────────────────────────────────────────────────

var ThemeMonokai = Theme{
	BgPanel:  "#3e3d32",
	BgSubtle: "#49483e",
	BgDimmer: "#272822",
	FgBase:   "#f8f8f2",
	FgDim:    "#75715e",
	FgFaint:  "#49483e",
	Accent:   "#f92672",
	Dir:      "#66d9ef",

	SynSection:    "#e6db74",
	SynKeyword:    "#ae81ff",
	SynHeaderKey:  "#66d9ef",
	SynHeaderVal:  "#f8f8f2",
	SynVar:        "#fd971f",
	SynInlineExpr: "#f92672",

	MethodGET:    "#a6e874",
	MethodPOST:   "#fd971f",
	MethodPUT:    "#66d9ef",
	MethodPATCH:  "#ae81ff",
	MethodDELETE: "#f92672",

	Status2xx: "#a6e874",
	Status3xx: "#fd971f",
	Status4xx: "#f92672",
	Status5xx: "#f92672",

	ChromaStyle: "monokai",
}

var ThemeNord = Theme{
	BgPanel:  "#3b4252",
	BgSubtle: "#434c5e",
	BgDimmer: "#2e3440",
	FgBase:   "#eceff4",
	FgDim:    "#7b88a1",
	FgFaint:  "#4c566a",
	Accent:   "#88c0d0",
	Dir:      "#81a1c1",

	SynSection:    "#ebcb8b",
	SynKeyword:    "#b48ead",
	SynHeaderKey:  "#88c0d0",
	SynHeaderVal:  "#eceff4",
	SynVar:        "#d08770",
	SynInlineExpr: "#a3be8c",

	MethodGET:    "#a3be8c",
	MethodPOST:   "#d08770",
	MethodPUT:    "#81a1c1",
	MethodPATCH:  "#b48ead",
	MethodDELETE: "#bf616a",

	Status2xx: "#a3be8c",
	Status3xx: "#d08770",
	Status4xx: "#bf616a",
	Status5xx: "#bf616a",

	ChromaStyle: "nord",
}

var ThemeCatppuccin = Theme{
	BgPanel:  "#313244",
	BgSubtle: "#45475a",
	BgDimmer: "#1e1e2e",
	FgBase:   "#cdd6f4",
	FgDim:    "#9399b2",
	FgFaint:  "#585b70",
	Accent:   "#cba6f7",
	Dir:      "#89b4fa",

	SynSection:    "#f9e2af",
	SynKeyword:    "#cba6f7",
	SynHeaderKey:  "#89dceb",
	SynHeaderVal:  "#cdd6f4",
	SynVar:        "#fab387",
	SynInlineExpr: "#89b4fa",

	MethodGET:    "#a6e3a1",
	MethodPOST:   "#fab387",
	MethodPUT:    "#89b4fa",
	MethodPATCH:  "#cba6f7",
	MethodDELETE: "#f38ba8",

	Status2xx: "#a6e3a1",
	Status3xx: "#fab387",
	Status4xx: "#f38ba8",
	Status5xx: "#eba0ac",

	ChromaStyle: "catppuccin-mocha",
}

// activeTheme is the theme used by all style-init functions.
// Call InitTheme before the TUI starts to override the default.
var activeTheme = ThemeCatppuccin

// InitTheme switches the active theme and re-initialises all style variables.
// Call this before starting the TUI (e.g. from a --theme CLI flag handler).
func InitTheme(t Theme) {
	activeTheme = t
	initStyles()
	initHighlightStyles()
	initHelpStyles()
}

// ThemeByName resolves "monokai", "nord", or "catppuccin" (case-insensitive).
// Returns the default (Catppuccin) and false if the name is not recognised.
func ThemeByName(name string) (Theme, bool) {
	switch name {
	case "monokai":
		return ThemeMonokai, true
	case "nord":
		return ThemeNord, true
	case "catppuccin":
		return ThemeCatppuccin, true
	}
	return ThemeCatppuccin, false
}

func init() {
	initStyles()
	initHighlightStyles()
	initHelpStyles()
}
