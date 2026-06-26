package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jefferson-julio/h77p/internal/executor"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/jefferson-julio/h77p/internal/writer"
)

// requestDoneMsg is sent by a background tea.Cmd when an HTTP request finishes.
type requestDoneMsg struct {
	result *runner.Result
	action string // "run" | "test" | "example"
	err    error
}

// FileView is the .http file inspector sub-model. Left panel lists requests
// by method + URL; right panel shows a scrollable, syntax-highlighted preview
// of the active request block (or the last HTTP response).
type FileView struct {
	file      *httpfile.File
	cursor    int
	scrollTop int // index of the first visible row in the left panel
	preview   viewport.Model
	width     int
	height    int

	search   searchInput
	filtered []int // indices into file.Requests that pass the current filter

	working    bool           // true while an HTTP request is in flight
	statusMsg  string         // brief status shown in the bar after a run
	lastResult *runner.Result // most recent completed result
	showResult bool           // right panel shows result instead of request source
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
	if fv.showResult && fv.lastResult != nil {
		fv.preview.SetContent(renderResult(fv.lastResult))
	} else {
		fv.preview.SetContent(renderRequest(fv.file.Requests[reqIdx]))
	}
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
	// Handle background request results before the key check.
	if done, ok := msg.(requestDoneMsg); ok {
		return fv.handleRequestDone(done)
	}

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
		fv.search.pos = len([]rune(fv.search.query))

	case "j", "down":
		if fv.cursor < n-1 {
			fv.cursor++
			fv.showResult = false
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}
	case "k", "up":
		if fv.cursor > 0 {
			fv.cursor--
			fv.showResult = false
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}
	case "g":
		fv.cursor = 0
		fv.showResult = false
		fv = fv.withScrollAdjusted().withSyncedPreview()
	case "G":
		if n > 0 {
			fv.cursor = n - 1
			fv.showResult = false
			fv = fv.withScrollAdjusted().withSyncedPreview()
		}

	case "r":
		if !fv.working && n > 0 {
			fv.working = true
			fv.statusMsg = "running…"
			return fv, fv.cmdRun("run")
		}
	case "t":
		if !fv.working && n > 0 {
			fv.working = true
			fv.statusMsg = "running tests…"
			return fv, fv.cmdRun("test")
		}
	case "e":
		if !fv.working && n > 0 {
			fv.working = true
			fv.statusMsg = "running…"
			return fv, fv.cmdRun("example")
		}

	case "h", "esc":
		return fv, func() tea.Msg { return backMsg{} }

	default:
		var cmd tea.Cmd
		fv.preview, cmd = fv.preview.Update(msg)
		return fv, cmd
	}
	return fv, nil
}

// cmdRun fires a background tea.Cmd that runs the currently selected request.
func (fv FileView) cmdRun(action string) tea.Cmd {
	if len(fv.filtered) == 0 || fv.file == nil {
		return nil
	}
	req := fv.file.Requests[fv.filtered[fv.cursor]]
	file := fv.file
	return func() tea.Msg {
		result, err := runner.Run(file, req.Name, make(map[string]string))
		return requestDoneMsg{result: result, action: action, err: err}
	}
}

// handleRequestDone processes an incoming requestDoneMsg.
func (fv FileView) handleRequestDone(msg requestDoneMsg) (FileView, tea.Cmd) {
	fv.working = false

	if msg.err != nil {
		fv.statusMsg = "error: " + msg.err.Error()
		return fv, nil
	}

	fv.lastResult = msg.result
	fv.showResult = true

	// Build status bar summary.
	if msg.result != nil && msg.result.HTTP != nil {
		h := msg.result.HTTP
		fv.statusMsg = fmt.Sprintf("%s  %dms", h.Status, h.Duration.Milliseconds())
		if len(msg.result.Tests) > 0 {
			passed, failed := 0, 0
			for _, t := range msg.result.Tests {
				if t.Passed {
					passed++
				} else {
					failed++
				}
			}
			fv.statusMsg += fmt.Sprintf("  ·  %d/%d tests passed", passed, passed+failed)
		}
	}

	// For the "example" action: persist the response to the .http file.
	if msg.action == "example" && msg.result != nil && msg.result.HTTP != nil && fv.file != nil {
		h := msg.result.HTTP
		req := fv.file.Requests[fv.filtered[fv.cursor]]
		ex := httpResultToExample(h)
		if err := writer.SaveExample(fv.file.Path, req.Name, ex); err != nil {
			fv.statusMsg += "  (save failed: " + err.Error() + ")"
		} else {
			fv.statusMsg += "  ·  example saved"
			// Reload the file so the right panel reflects the new @example block on next navigation.
			if updated, err := parser.ParseFile(fv.file.Path); err == nil {
				fv.file = updated
				fv.filtered = fv.computeFiltered()
			}
		}
	}

	fv = fv.withSyncedPreview()
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
	if fv.working {
		return styleStatusBar.Width(fv.width).Render(fv.statusMsg)
	}

	hint := "r run  t test  e save example  j/k move  g/G top/bot  ctrl+d/u scroll  / search  h/esc back  q quit"
	if fv.statusMsg != "" {
		tag := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("[" + fv.statusMsg + "]")
		hint = tag + "  " + hint
	}
	if fv.search.query != "" {
		tag := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).
			Render(fmt.Sprintf("[/%s]", fv.search.query))
		hint = tag + "  " + hint
	}
	return styleStatusBar.Width(fv.width).Render(hint)
}

// renderResult builds a coloured response preview for the viewport.
func renderResult(result *runner.Result) string {
	if result == nil {
		return styleDim.Render("(no result)")
	}
	if result.Err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: " + result.Err.Error())
	}

	h := result.HTTP
	var b strings.Builder

	b.WriteString(colorStatusLine(h.Proto+" "+h.Status) + "\n")

	keys := make([]string, 0, len(h.Headers))
	for k := range h.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(colorHeader(k+": "+strings.Join(h.Headers[k], ", ")) + "\n")
	}

	if h.Body != "" {
		b.WriteString("\n")
		b.WriteString(h.Body)
		b.WriteString("\n")
	}

	if len(result.Tests) > 0 {
		b.WriteString("\n")
		for _, t := range result.Tests {
			if t.Passed {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  PASS  "+t.Name) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("  FAIL  "+t.Name+" — "+t.Error) + "\n")
			}
		}
	}

	return b.String()
}

// httpResultToExample converts an HTTP response into a file example block.
func httpResultToExample(ex *executor.Result) *httpfile.Example {
	example := &httpfile.Example{
		Status: ex.Proto + " " + ex.Status,
		Body:   ex.Body,
	}
	keys := make([]string, 0, len(ex.Headers))
	for k := range ex.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		example.Headers = append(example.Headers, httpfile.Header{
			Name:  k,
			Value: strings.Join(ex.Headers[k], ", "),
		})
	}
	return example
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
