package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
)

// FileView is the .http file inspector sub-model. Left panel lists requests
// by method + URL; right panel shows a scrollable, syntax-highlighted preview
// of the active request block.
type FileView struct {
	file      *httpfile.File
	cursor    int
	scrollTop int // index of the first visible row in the left panel
	preview   viewport.Model
	width     int
	height    int

	search   searchInput
	filtered []int // indices into file.Requests that pass the current filter
}

func newFileView(path string, w, h int) (FileView, error) {
	file, err := parser.ParseFile(path)
	if err != nil {
		return FileView{}, err
	}
	pw := max(rightWidth(w), 1)
	ph := max(contentHeight(h), 1)
	fv := FileView{
		file:    file,
		preview: viewport.New(pw, ph),
		width:   w,
		height:  h,
	}
	fv.filtered = fv.computeFiltered()
	return fv.withSyncedPreview(), nil
}

// computeFiltered rebuilds the filtered index list from the current search query.
func (fv FileView) computeFiltered() []int {
	if fv.file == nil {
		return nil
	}
	if fv.search.query == "" {
		all := make([]int, len(fv.file.Requests))
		for i := range all {
			all[i] = i
		}
		return all
	}
	q := strings.ToLower(fv.search.query)
	var out []int
	for i, req := range fv.file.Requests {
		if strings.Contains(strings.ToLower(req.Name), q) ||
			strings.Contains(strings.ToLower(req.Method), q) ||
			strings.Contains(strings.ToLower(req.URL), q) {
			out = append(out, i)
		}
	}
	return out
}

// withSyncedPreview updates the viewport content to match the current cursor.
func (fv FileView) withSyncedPreview() FileView {
	if len(fv.filtered) == 0 {
		msg := "(no requests)"
		if fv.search.query != "" {
			msg = "(no matches)"
		}
		fv.preview.SetContent(styleDim.Render(msg))
		return fv
	}
	reqIdx := fv.filtered[fv.cursor]
	fv.preview.SetContent(renderRequest(fv.file.Requests[reqIdx]))
	fv.preview.GotoTop()
	return fv
}

func (fv FileView) resize(w, h int) FileView {
	fv.width, fv.height = w, h
	fv.preview.Width = max(rightWidth(w), 1)
	fv.preview.Height = max(contentHeight(h), 1)
	return fv
}

// withScrollAdjusted ensures scrollTop keeps the cursor within the visible window.
func (fv FileView) withScrollAdjusted() FileView {
	ch := max(contentHeight(fv.height), 1)
	if fv.cursor < fv.scrollTop {
		fv.scrollTop = fv.cursor
	} else if fv.cursor >= fv.scrollTop+ch {
		fv.scrollTop = fv.cursor - ch + 1
	}
	return fv
}

func (fv FileView) update(msg tea.Msg) (FileView, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (scroll events, etc.) go straight to the viewport.
		var cmd tea.Cmd
		fv.preview, cmd = fv.preview.Update(msg)
		return fv, cmd
	}

	// While searching, route all keystrokes through the search input.
	if fv.search.active {
		oldQuery := fv.search.query
		fv.search = fv.search.handleKey(key)
		if fv.search.query != oldQuery {
			fv.filtered = fv.computeFiltered()
			fv.cursor = 0
			fv.scrollTop = 0
			fv = fv.withSyncedPreview()
		}
		return fv, nil
	}

	n := len(fv.filtered)

	switch key.String() {
	case "/":
		fv.search.active = true
		// Position cursor at end so the user can refine or backspace to clear.
		fv.search.pos = len([]rune(fv.search.query))

	case "j", "down":
		if fv.cursor < n-1 {
			fv.cursor++
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}
	case "k", "up":
		if fv.cursor > 0 {
			fv.cursor--
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}
	case "g":
		fv.cursor = 0
		fv = fv.withScrollAdjusted().withSyncedPreview()
	case "G":
		if n > 0 {
			fv.cursor = n - 1
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}
	case "h", "esc":
		return fv, func() tea.Msg { return backMsg{} }
	default:
		// Forward scroll keys (ctrl+d/u, pgup/pgdn, etc.) to the viewport.
		var cmd tea.Cmd
		fv.preview, cmd = fv.preview.Update(msg)
		return fv, cmd
	}
	return fv, nil
}

func (fv FileView) view() string {
	if fv.width == 0 {
		return ""
	}
	lw := leftWidth(fv.width)
	ch := max(contentHeight(fv.height), 1)

	name := "(no file)"
	if fv.file != nil {
		name = filepath.Base(fv.file.Path)
	}
	header := styleHeader.Width(fv.width).Render(name)

	left := fv.buildLeftLines(lw, ch)
	right := strings.Split(fv.preview.View(), "\n")
	body := zipPanels(left, right, lw, ch)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, fv.statusLine())
}

func (fv FileView) buildLeftLines(w, h int) []string {
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	if fv.file == nil {
		return lines
	}
	for screenIdx := range h {
		dataIdx := fv.scrollTop + screenIdx
		if dataIdx >= len(fv.filtered) {
			break
		}
		reqIdx := fv.filtered[dataIdx]
		req := fv.file.Requests[reqIdx]
		prefix := "  "
		if dataIdx == fv.cursor {
			prefix = "▸ "
		}
		label := truncate(fmt.Sprintf("%s%-7s %s", prefix, req.Method, req.URL), w)
		if dataIdx == fv.cursor {
			lines[screenIdx] = styleCursor.Width(w).Render(label)
		} else {
			s, ok := styleMethod[req.Method]
			if !ok {
				s = lipgloss.NewStyle()
			}
			lines[screenIdx] = s.Width(w).Render(label)
		}
	}
	return lines
}

func (fv FileView) statusLine() string {
	if fv.search.active {
		return styleStatusBar.Width(fv.width).Render(fv.search.renderPrompt())
	}
	hint := "j/k move  g/G top/bot  ctrl+d/u scroll  / search  h/esc back  q quit"
	if fv.search.query != "" {
		tag := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).
			Render(fmt.Sprintf("[/%s]", fv.search.query))
		hint = tag + "  " + hint
	}
	return styleStatusBar.Width(fv.width).Render(hint)
}

// renderRequest builds the text block for a single request that gets fed into
// the syntax highlighter and shown in the preview viewport.
func renderRequest(req httpfile.Request) string {
	var b strings.Builder

	fmt.Fprintf(&b, "### %s\n", req.Name)

	if req.PreScript != "" {
		fmt.Fprintf(&b, "\n@pre-request {%%\n%s\n%%}\n", req.PreScript)
	}

	fmt.Fprintf(&b, "\n%s %s\n", req.Method, req.URL)
	for _, h := range req.Headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if req.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", req.Body)
	}
	if req.PostScript != "" {
		fmt.Fprintf(&b, "\n@post-response {%%\n%s\n%%}\n", req.PostScript)
	}
	if req.Example != nil {
		fmt.Fprintf(&b, "\n@example {%%\n%s\n", req.Example.Status)
		for _, h := range req.Example.Headers {
			fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
		}
		if req.Example.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", req.Example.Body)
		}
		b.WriteString("%}\n")
	}

	return highlightHTTP(b.String())
}
