package ui

import (
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// searchInput tracks a live filter query with an in-line cursor position.
// It handles a small emacs-inspired set of editing keys.
type searchInput struct {
	query  string
	pos    int  // rune index of the cursor within query
	active bool // true while the user is actively typing
}

// handleKey processes one key event and returns the updated state.
func (s searchInput) handleKey(key tea.KeyMsg) searchInput {
	r := []rune(s.query)
	switch key.String() {
	case "esc", "enter":
		s.active = false

	case "backspace":
		if s.pos > 0 {
			r = append(r[:s.pos-1], r[s.pos:]...)
			s.pos--
			s.query = string(r)
		}

	case "ctrl+b":
		if s.pos > 0 {
			s.pos--
		}

	case "ctrl+e":
		s.pos = len(r)

	case "ctrl+u":
		// Kill from start of line to cursor.
		s.query = string(r[s.pos:])
		s.pos = 0

	case "alt+d":
		// Delete from cursor to end of next word.
		end := s.pos
		for end < len(r) && !isWordRune(r[end]) {
			end++
		}
		for end < len(r) && isWordRune(r[end]) {
			end++
		}
		s.query = string(append(r[:s.pos:s.pos], r[end:]...))

	default:
		// Only insert plain runes — skip alt-combos we don't handle.
		if key.Type == tea.KeyRunes && !key.Alt {
			newR := make([]rune, 0, len(r)+len(key.Runes))
			newR = append(newR, r[:s.pos]...)
			newR = append(newR, key.Runes...)
			newR = append(newR, r[s.pos:]...)
			s.pos += len(key.Runes)
			s.query = string(newR)
		}
	}
	return s
}

// renderPrompt renders "/ query" with a block cursor at the current position.
// Intended for display inside a status bar while active == true.
func (s searchInput) renderPrompt() string {
	r := []rune(s.query)
	before := string(r[:s.pos])

	var cur, after string
	if s.pos < len(r) {
		// Cursor sits on a character — highlight it.
		cur = lipgloss.NewStyle().
			Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0")).
			Render(string(r[s.pos : s.pos+1]))
		after = string(r[s.pos+1:])
	} else {
		// Cursor is past the end — show a block.
		cur = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("█")
	}

	slash := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Render("/")
	return slash + " " + before + cur + after
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
