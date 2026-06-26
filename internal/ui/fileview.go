package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jefferson-julio/h77p/internal/executor"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/jefferson-julio/h77p/internal/writer"
)

const (
	tabRequest = iota
	tabRun
	tabTests
	tabLogs
	tabExample
)

// fileChangedMsg is sent when the watched .http file is modified on disk.
type fileChangedMsg struct{}

// editFileDoneMsg is sent after an external editor session finishes.
type editFileDoneMsg struct{ err error }

// requestDoneMsg is sent by a background tea.Cmd when an HTTP request finishes.
type requestDoneMsg struct {
	result *runner.Result
	action string // "run" | "test" | "example"
	err    error
	vars   map[string]string // env after run (carries set() results back to caller)
}

// bodyViewerDoneMsg is sent after an external body viewer process exits.
type bodyViewerDoneMsg struct{ err error }

// cmdOpenBody suspends the TUI and opens body in the first available viewer
// (otree → jless → less). The body is fed via stdin so the program can open
// /dev/tty itself for keyboard navigation.
func cmdOpenBody(body string) tea.Cmd {
	if body == "" {
		return nil
	}
	var program string
	for _, name := range []string{"otree", "jless", "less"} {
		if _, err := exec.LookPath(name); err == nil {
			program = name
			break
		}
	}
	if program == "" {
		return func() tea.Msg {
			return bodyViewerDoneMsg{err: fmt.Errorf("no body viewer found (tried: otree, jless, less)")}
		}
	}
	cmd := exec.Command(program)
	cmd.Stdin = strings.NewReader(body)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return bodyViewerDoneMsg{err: err}
	})
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

	working    bool              // true while an HTTP request is in flight
	statusMsg  string            // brief status shown in the bar after a run
	lastResult *runner.Result    // most recent completed result
	activeTab  int               // which right-panel tab is shown (tabRequest…tabLogs)
	env        map[string]string // session variables: set() results persist here
	helpOpen   bool

	watchDone    chan struct{} // closed to stop the poll goroutine when leaving this view
	watchModTime time.Time     // last known file mod time; poll compares against this
}

func newFileView(path string, w, h int) (FileView, error) {
	file, err := parser.ParseFile(path)
	if err != nil {
		return FileView{}, err
	}
	pw := max(rightWidth(w), 1)
	ph := max(contentHeight(h)-1, 1) // -1 for tab bar
	fv := FileView{
		file:      file,
		preview:   viewport.New(pw, ph),
		width:     w,
		height:    h,
		watchDone: make(chan struct{}),
		env:       make(map[string]string),
	}
	if info, err := os.Stat(path); err == nil {
		fv.watchModTime = info.ModTime()
	}
	fv.filtered = fv.computeFiltered()
	return fv.withSyncedPreview(), nil
}

// watchCmd returns the initial tea.Cmd that starts polling the file for changes.
// Called once by the parent model when this view becomes active.
func (fv FileView) watchCmd() tea.Cmd {
	if fv.watchDone == nil || fv.file == nil {
		return nil
	}
	return cmdPollFile(fv.file.Path, fv.watchModTime, fv.watchDone)
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

// withSyncedPreview updates the viewport content to match the current tab.
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
	switch fv.activeTab {
	case tabRun:
		fv.preview.SetContent(renderHTTPResult(fv.lastResult))
	case tabTests:
		fv.preview.SetContent(renderTests(fv.lastResult))
	case tabLogs:
		fv.preview.SetContent(renderLogs(fv.lastResult))
	case tabExample:
		fv.preview.SetContent(renderExample(fv.file.Requests[reqIdx]))
	default: // tabRequest
		fv.preview.SetContent(renderRequest(fv.file.Requests[reqIdx]))
	}
	fv.preview.GotoTop()
	return fv
}

func (fv FileView) resize(w, h int) FileView {
	fv.width, fv.height = w, h
	fv.preview.Width = max(rightWidth(w), 1)
	fv.preview.Height = max(contentHeight(h)-1, 1) // -1 for tab bar
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

	// Handle file-change notifications from the poll goroutine.
	if _, ok := msg.(fileChangedMsg); ok {
		return fv.handleFileChanged()
	}

	// Handle external editor finishing.
	if done, ok := msg.(editFileDoneMsg); ok {
		return fv.handleEditFileDone(done)
	}

	// Handle external body viewer finishing.
	if done, ok := msg.(bodyViewerDoneMsg); ok {
		if done.err != nil {
			fv.statusMsg = "viewer: " + done.err.Error()
		}
		return fv, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (scroll events, etc.) go straight to the viewport.
		var cmd tea.Cmd
		fv.preview, cmd = fv.preview.Update(msg)
		return fv, cmd
	}

	if fv.helpOpen {
		fv.helpOpen = false
		return fv, nil
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
	case "?":
		fv.helpOpen = true

	case "/":
		fv.search.active = true
		fv.search.pos = len([]rune(fv.search.query))

	case "1":
		fv.activeTab = tabRequest
		fv = fv.withSyncedPreview()
	case "2":
		fv.activeTab = tabRun
		fv = fv.withSyncedPreview()
	case "3":
		fv.activeTab = tabTests
		fv = fv.withSyncedPreview()
	case "4":
		fv.activeTab = tabLogs
		fv = fv.withSyncedPreview()
	case "5":
		fv.activeTab = tabExample
		fv = fv.withSyncedPreview()
	case "tab":
		fv.activeTab = (fv.activeTab + 1) % 5
		fv = fv.withSyncedPreview()

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

	case "enter", "l":
		if n > 0 {
			if fv.watchDone != nil {
				close(fv.watchDone)
				fv.watchDone = nil
			}
			file := fv.file
			req := file.Requests[fv.filtered[fv.cursor]]
			return fv, func() tea.Msg {
				return openPartsMsg{path: file.Path, file: file, req: req}
			}
		}

	case "o":
		body := ""
		if fv.lastResult != nil && fv.lastResult.HTTP != nil {
			body = fv.lastResult.HTTP.Body
		}
		if body == "" && n > 0 {
			if req := fv.file.Requests[fv.filtered[fv.cursor]]; req.Example != nil {
				body = req.Example.Body
			}
		}
		if body != "" {
			return fv, cmdOpenBody(body)
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
		if !fv.working && fv.file != nil {
			if fv.watchDone != nil {
				close(fv.watchDone)
				fv.watchDone = nil
			}
			return fv, fv.cmdEditFile()
		}
	case "E":
		if !fv.working && n > 0 {
			if fv.watchDone != nil {
				close(fv.watchDone)
				fv.watchDone = nil
			}
			return fv, fv.cmdEditRequest()
		}
	case "x":
		if !fv.working && n > 0 {
			fv.working = true
			fv.statusMsg = "running…"
			return fv, fv.cmdRun("example")
		}

	case "h", "esc":
		if fv.watchDone != nil {
			close(fv.watchDone)
			fv.watchDone = nil
		}
		return fv, func() tea.Msg { return backMsg{} }

	default:
		var cmd tea.Cmd
		fv.preview, cmd = fv.preview.Update(msg)
		return fv, cmd
	}
	return fv, nil
}

// cmdEditFile suspends the TUI and opens the whole .http file in $EDITOR.
func (fv FileView) cmdEditFile() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return tea.ExecProcess(exec.Command(editor, fv.file.Path), func(err error) tea.Msg {
		return editFileDoneMsg{err: err}
	})
}

// cmdEditRequest suspends the TUI and opens the selected request block in $EDITOR.
func (fv FileView) cmdEditRequest() tea.Cmd {
	if len(fv.filtered) == 0 || fv.file == nil {
		return nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	req := fv.file.Requests[fv.filtered[fv.cursor]]
	block, err := writer.ExtractRequestBlock(fv.file.Path, req.Name)
	if err != nil {
		return func() tea.Msg { return editFileDoneMsg{err: err} }
	}
	tmp, err := os.CreateTemp("", "h77p-*.http")
	if err != nil {
		return func() tea.Msg { return editFileDoneMsg{err: err} }
	}
	_, _ = tmp.WriteString(block)
	tmp.Close()
	tmpPath := tmp.Name()
	filePath := fv.file.Path
	reqName := req.Name
	return tea.ExecProcess(exec.Command(editor, tmpPath), func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return editFileDoneMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		if readErr != nil {
			return editFileDoneMsg{err: readErr}
		}
		return editFileDoneMsg{err: writer.SaveRequestBlock(filePath, reqName, string(data))}
	})
}

// handleEditFileDone processes the result of an external editor session.
func (fv FileView) handleEditFileDone(msg editFileDoneMsg) (FileView, tea.Cmd) {
	if msg.err != nil {
		fv.statusMsg = "edit error: " + msg.err.Error()
	}
	fv.watchDone = make(chan struct{})
	return fv.handleFileChanged()
}

// cmdRun fires a background tea.Cmd that runs the currently selected request.
func (fv FileView) cmdRun(action string) tea.Cmd {
	if len(fv.filtered) == 0 || fv.file == nil {
		return nil
	}
	req := fv.file.Requests[fv.filtered[fv.cursor]]
	file := fv.file
	vars := copyEnv(fv.env) // copy so goroutine mutations don't race the main loop
	return func() tea.Msg {
		result, err := runner.Run(file, req.Name, vars)
		return requestDoneMsg{result: result, action: action, err: err, vars: vars}
	}
}

// handleRequestDone processes an incoming requestDoneMsg.
func (fv FileView) handleRequestDone(msg requestDoneMsg) (FileView, tea.Cmd) {
	fv.working = false

	if msg.vars != nil {
		fv.env = msg.vars
	}

	if msg.err != nil {
		fv.statusMsg = "error: " + msg.err.Error()
		return fv, nil
	}

	fv.lastResult = msg.result
	fv.activeTab = tabRun

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
		if msg.result.JQOutput != "" {
			ex.Body = msg.result.JQOutput
		}
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
	if fv.helpOpen {
		return renderHelpOverlay(fv.width, fv.height, helpFileView)
	}
	lw := leftWidth(fv.width)
	rw := rightWidth(fv.width)
	ch := max(contentHeight(fv.height), 1)

	name := "(no file)"
	if fv.file != nil {
		name = filepath.Base(fv.file.Path)
	}
	header := styleHeader.Width(fv.width).Render(name)

	left := fv.buildLeftLines(lw, ch)
	tabBar := renderTabBar(fv.activeTab, rw)
	vpLines := strings.Split(fv.preview.View(), "\n")
	right := append([]string{tabBar}, vpLines...)
	body := zipPanels(left, right, lw, rw, ch)

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
		if dataIdx == fv.cursor {
			// Cursor line: plain text with cursor background.
			var plain string
			if req.Name != "" {
				plain = fmt.Sprintf("%s%-7s %s  %s", prefix, req.Method, req.Name, req.URL)
			} else {
				plain = fmt.Sprintf("%s%-7s %s", prefix, req.Method, req.URL)
			}
			lines[screenIdx] = styleCursor.Width(w).Render(truncate(plain, w))
		} else {
			ms, ok := styleMethod[req.Method]
			if !ok {
				ms = lipgloss.NewStyle()
			}
			if req.Name != "" {
				// Non-cursor: name in method color, URL dimmed.
				main := ms.Render(fmt.Sprintf("%s%-7s %s", prefix, req.Method, req.Name))
				tail := styleDim.Render("  " + req.URL)
				lines[screenIdx] = lipgloss.NewStyle().Width(w).Render(
					ansi.Truncate(main+tail, w, ""),
				)
			} else {
				label := truncate(fmt.Sprintf("%s%-7s %s", prefix, req.Method, req.URL), w)
				lines[screenIdx] = ms.Width(w).Render(label)
			}
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

	hint := "r run  e edit  enter inspect  / search  ? help"
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

// handleFileChanged reloads the parsed file and tries to keep the cursor on the
// same request name. After updating state it re-arms the poll goroutine.
func (fv FileView) handleFileChanged() (FileView, tea.Cmd) {
	if fv.file == nil {
		return fv, fv.watchCmd()
	}

	updated, err := parser.ParseFile(fv.file.Path)
	if err == nil {
		// Preserve cursor by request name across reloads.
		var cursorName string
		if len(fv.filtered) > 0 {
			cursorName = fv.file.Requests[fv.filtered[fv.cursor]].Name
		}

		fv.file = updated
		fv.filtered = fv.computeFiltered()

		// Try to re-find the same request.
		found := false
		if cursorName != "" {
			for i, idx := range fv.filtered {
				if fv.file.Requests[idx].Name == cursorName {
					fv.cursor = i
					found = true
					break
				}
			}
		}
		if !found && len(fv.filtered) > 0 && fv.cursor >= len(fv.filtered) {
			fv.cursor = len(fv.filtered) - 1
		}

		fv = fv.withScrollAdjusted().withSyncedPreview()
	}

	// Update the stored mod time so the next poll doesn't fire immediately.
	if info, err := os.Stat(fv.file.Path); err == nil {
		fv.watchModTime = info.ModTime()
	}

	return fv, cmdPollFile(fv.file.Path, fv.watchModTime, fv.watchDone)
}

// cmdPollFile returns a tea.Cmd that blocks until the file at path is modified
// (compared to modTime) or the done channel is closed.
func cmdPollFile(path string, modTime time.Time, done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return nil
			case <-ticker.C:
				info, err := os.Stat(path)
				if err == nil && info.ModTime().After(modTime) {
					return fileChangedMsg{}
				}
			}
		}
	}
}

// renderExample shows the @example block for a request, syntax-highlighted.
func renderExample(req httpfile.Request) string {
	if req.Example == nil {
		return styleDim.Render("(no @example — run the request and press x to save one)")
	}
	ex := req.Example
	var b strings.Builder
	fmt.Fprintf(&b, "@example {%%\n%s\n", ex.Status)
	for _, h := range ex.Headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
	}
	if ex.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", ex.Body)
	}
	b.WriteString("%}")
	return highlightHTTP(b.String())
}

// renderHTTPResult shows just the HTTP response (status, headers, body) without tests.
// When @jq filters were applied, the transformed body is shown instead of the raw one.
func renderHTTPResult(result *runner.Result) string {
	if result == nil {
		return styleDim.Render("(no run yet — press r to run)")
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
	if result.JQOutput != "" {
		b.WriteString("\n")
		b.WriteString(styleDim.Render("[@jq]") + "\n")
		b.WriteString(colorJSON(result.JQOutput))
		b.WriteString("\n")
	} else if h.Body != "" {
		b.WriteString("\n")
		b.WriteString(highlightBodyFromHeaders(h.Body, h.Headers))
		b.WriteString("\n")
	}
	return b.String()
}

// renderTests shows only the test results from the last run.
func renderTests(result *runner.Result) string {
	if result == nil {
		return styleDim.Render("(no run yet — press t to run)")
	}
	if result.Err != nil && len(result.Tests) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("error: " + result.Err.Error())
	}
	if len(result.Tests) == 0 {
		return styleDim.Render("(no tests in post-response script)")
	}
	var b strings.Builder
	passed, failed := 0, 0
	for _, t := range result.Tests {
		if t.Passed {
			passed++
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("  PASS  "+t.Name) + "\n")
		} else {
			failed++
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("  FAIL  "+t.Name) + "\n")
			if t.Error != "" {
				b.WriteString(styleDim.Render("        "+t.Error) + "\n")
			}
		}
	}
	b.WriteString("\n")
	summary := fmt.Sprintf("%d passed", passed)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	b.WriteString(styleDim.Render(summary) + "\n")
	return b.String()
}

// renderLogs shows log() output from script execution.
func renderLogs(result *runner.Result) string {
	if result == nil {
		return styleDim.Render("(no run yet)")
	}
	if len(result.Logs) == 0 {
		return styleDim.Render("(no log() calls in scripts)")
	}
	var b strings.Builder
	for i, l := range result.Logs {
		b.WriteString(styleDim.Render(fmt.Sprintf("[%d]", i+1)) + "\n")
		b.WriteString(l + "\n\n")
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

	for _, v := range req.Variables {
		fmt.Fprintf(&b, "@%s = %s\n", v.Name, v.Value)
	}

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
	for _, filter := range req.JQFilters {
		fmt.Fprintf(&b, "@jq %s\n", filter)
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
