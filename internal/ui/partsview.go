package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/jefferson-julio/h77p/internal/writer"
)

const (
	partRequest    = 0
	partPreScript  = 1
	partPostScript = 2
	partExample    = 3
)

type openPartsMsg struct {
	path string
	file *httpfile.File
	req  httpfile.Request
}

type partsEditDoneMsg struct {
	kind    int
	content string
	err     error
}

// PartsView lets the user inspect and edit individual parts of a request:
// the HTTP method/headers/body, pre-request script, post-response script,
// and the saved example response.
type PartsView struct {
	path       string
	file       *httpfile.File
	req        httpfile.Request
	cursor     int
	preview    viewport.Model
	width      int
	height     int
	status     string
	working    bool
	lastResult *runner.Result
	showResult bool
}

func newPartsView(path string, file *httpfile.File, req httpfile.Request, w, h int) PartsView {
	pw := max(rightWidth(w), 1)
	ph := max(contentHeight(h), 1)
	pv := PartsView{
		path:    path,
		file:    file,
		req:     req,
		preview: viewport.New(pw, ph),
		width:   w,
		height:  h,
	}
	pv.preview.SetContent(pv.previewContent())
	return pv
}

func (pv PartsView) withSyncedPreview() PartsView {
	if pv.showResult && pv.lastResult != nil {
		pv.preview.SetContent(renderResult(pv.lastResult))
	} else {
		pv.preview.SetContent(pv.previewContent())
	}
	pv.preview.GotoTop()
	return pv
}

func (pv PartsView) resize(w, h int) PartsView {
	pv.width, pv.height = w, h
	pv.preview.Width = max(rightWidth(w), 1)
	pv.preview.Height = max(contentHeight(h), 1)
	return pv
}

func (pv PartsView) update(msg tea.Msg) (PartsView, tea.Cmd) {
	if done, ok := msg.(partsEditDoneMsg); ok {
		return pv.handleEditDone(done)
	}
	if done, ok := msg.(requestDoneMsg); ok {
		return pv.handleRequestDone(done)
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		pv.preview, cmd = pv.preview.Update(msg)
		return pv, cmd
	}

	switch key.String() {
	case "j", "down":
		if pv.cursor < 3 {
			pv.cursor++
			pv.showResult = false
			pv = pv.withSyncedPreview()
		}
	case "k", "up":
		if pv.cursor > 0 {
			pv.cursor--
			pv.showResult = false
			pv = pv.withSyncedPreview()
		}
	case "e", "enter", "l":
		return pv, pv.cmdEdit()
	case "r":
		if !pv.working {
			pv.working = true
			pv.status = "running…"
			return pv, pv.cmdRun("run")
		}
	case "t":
		if !pv.working {
			pv.working = true
			pv.status = "running tests…"
			return pv, pv.cmdRun("test")
		}
	case "x":
		if !pv.working {
			pv.working = true
			pv.status = "running…"
			return pv, pv.cmdRun("example")
		}
	case "h", "esc":
		return pv, func() tea.Msg { return backMsg{} }
	default:
		var cmd tea.Cmd
		pv.preview, cmd = pv.preview.Update(msg)
		return pv, cmd
	}
	return pv, nil
}

func (pv PartsView) cmdEdit() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	kind := pv.cursor
	content := pv.editorContent()

	ext := ".http"
	if kind == partPreScript || kind == partPostScript {
		ext = ".js"
	}

	tmp, err := os.CreateTemp("", "h77p-*"+ext)
	if err != nil {
		return func() tea.Msg { return partsEditDoneMsg{kind: kind, err: err} }
	}
	_, _ = tmp.WriteString(content)
	tmp.Close()
	tmpPath := tmp.Name()

	return tea.ExecProcess(exec.Command(editor, tmpPath), func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return partsEditDoneMsg{kind: kind, err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		return partsEditDoneMsg{kind: kind, content: string(data), err: readErr}
	})
}

func (pv PartsView) cmdRun(action string) tea.Cmd {
	if pv.file == nil {
		return nil
	}
	file := pv.file
	reqName := pv.req.Name
	return func() tea.Msg {
		result, err := runner.Run(file, reqName, make(map[string]string))
		return requestDoneMsg{result: result, action: action, err: err}
	}
}

func (pv PartsView) handleRequestDone(msg requestDoneMsg) (PartsView, tea.Cmd) {
	pv.working = false
	if msg.err != nil {
		pv.status = "error: " + msg.err.Error()
		return pv, nil
	}

	pv.lastResult = msg.result
	pv.showResult = true

	if msg.result != nil && msg.result.HTTP != nil {
		h := msg.result.HTTP
		pv.status = fmt.Sprintf("%s  %dms", h.Status, h.Duration.Milliseconds())
		if len(msg.result.Tests) > 0 {
			passed, failed := 0, 0
			for _, t := range msg.result.Tests {
				if t.Passed {
					passed++
				} else {
					failed++
				}
			}
			pv.status += fmt.Sprintf("  ·  %d/%d tests passed", passed, passed+failed)
		}
	}

	if msg.action == "example" && msg.result != nil && msg.result.HTTP != nil {
		ex := httpResultToExample(msg.result.HTTP)
		if err := writer.SaveExample(pv.path, pv.req.Name, ex); err != nil {
			pv.status += "  (save failed: " + err.Error() + ")"
		} else {
			pv.status += "  ·  example saved"
			if updated, err := parser.ParseFile(pv.path); err == nil {
				pv.file = updated
				for _, r := range updated.Requests {
					if r.Name == pv.req.Name {
						pv.req = r
						break
					}
				}
			}
		}
	}

	pv = pv.withSyncedPreview()
	return pv, nil
}

func (pv PartsView) editorContent() string {
	switch pv.cursor {
	case partRequest:
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s\n", pv.req.Method, pv.req.URL)
		for _, h := range pv.req.Headers {
			fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
		}
		if pv.req.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", pv.req.Body)
		}
		return b.String()
	case partPreScript:
		if pv.req.PreScript != "" {
			return dedent(pv.req.PreScript)
		}
		return "// pre-request script\n// request.headers[\"X-Custom\"] = \"value\";\n"
	case partPostScript:
		if pv.req.PostScript != "" {
			return dedent(pv.req.PostScript)
		}
		return "// post-response script\n// test(\"status 200\", () => { assert(response.status === 200); });\n"
	case partExample:
		if pv.req.Example != nil {
			var b strings.Builder
			fmt.Fprintf(&b, "%s\n", pv.req.Example.Status)
			for _, h := range pv.req.Example.Headers {
				fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
			}
			if pv.req.Example.Body != "" {
				fmt.Fprintf(&b, "\n%s\n", pv.req.Example.Body)
			}
			return b.String()
		}
		return "HTTP/1.1 200 OK\nContent-Type: application/json\n\n{}\n"
	}
	return ""
}

func (pv PartsView) handleEditDone(msg partsEditDoneMsg) (PartsView, tea.Cmd) {
	if msg.err != nil {
		pv.status = "edit error: " + msg.err.Error()
		return pv, nil
	}
	if err := pv.applyEdit(msg.kind, msg.content); err != nil {
		pv.status = "save error: " + err.Error()
		return pv, nil
	}
	updated, err := parser.ParseFile(pv.path)
	if err != nil {
		pv.status = "reload error: " + err.Error()
		return pv, nil
	}
	pv.file = updated
	for _, r := range updated.Requests {
		if r.Name == pv.req.Name {
			pv.req = r
			break
		}
	}
	pv.status = "saved"
	pv.showResult = false
	pv = pv.withSyncedPreview()
	return pv, nil
}

func (pv PartsView) applyEdit(kind int, content string) error {
	content = strings.TrimRight(content, "\n")
	switch kind {
	case partRequest:
		method, url, headers, body := parseRequestEdit(content)
		return writer.SaveRequestLines(pv.path, pv.req.Name, method, url, headers, body)
	case partPreScript:
		return writer.SaveScript(pv.path, pv.req.Name, "pre-request", content)
	case partPostScript:
		return writer.SaveScript(pv.path, pv.req.Name, "post-response", content)
	case partExample:
		ex := parseExampleEdit(content)
		return writer.SaveExample(pv.path, pv.req.Name, ex)
	}
	return nil
}

func (pv PartsView) previewContent() string {
	switch pv.cursor {
	case partRequest:
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s\n", pv.req.Method, pv.req.URL)
		for _, h := range pv.req.Headers {
			fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
		}
		if pv.req.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", pv.req.Body)
		}
		return highlightHTTP(b.String())
	case partPreScript:
		if pv.req.PreScript == "" {
			return styleDim.Render("(empty — press e to create)")
		}
		return highlightHTTP(fmt.Sprintf("@pre-request {%%\n%s\n%%}", pv.req.PreScript))
	case partPostScript:
		if pv.req.PostScript == "" {
			return styleDim.Render("(empty — press e to create)")
		}
		return highlightHTTP(fmt.Sprintf("@post-response {%%\n%s\n%%}", pv.req.PostScript))
	case partExample:
		if pv.req.Example == nil {
			return styleDim.Render("(empty — press e to create)")
		}
		ex := pv.req.Example
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
	return ""
}

func (pv PartsView) view() string {
	if pv.width == 0 {
		return ""
	}
	lw := leftWidth(pv.width)
	ch := max(contentHeight(pv.height), 1)

	title := pv.req.Name
	if title == "" {
		title = pv.req.Method + " " + pv.req.URL
	}
	header := styleHeader.Width(pv.width).Render(title)

	left := pv.buildLeftLines(lw, ch)
	right := strings.Split(pv.preview.View(), "\n")
	body := zipPanels(left, right, lw, ch)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, pv.statusLine())
}

func (pv PartsView) buildLeftLines(w, h int) []string {
	labels := pv.partLabels()
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	for i, label := range labels {
		if i >= h {
			break
		}
		prefix := "  "
		if i == pv.cursor {
			prefix = "▸ "
		}
		plain := prefix + label
		if i == pv.cursor {
			lines[i] = styleCursor.Width(w).Render(truncate(plain, w))
		} else {
			lines[i] = styleDim.Width(w).Render(truncate(plain, w))
		}
	}
	return lines
}

func (pv PartsView) partLabels() [4]string {
	reqLabel := pv.req.Method + " " + pv.req.URL
	if pv.req.Name != "" {
		reqLabel = pv.req.Method + " " + pv.req.Name
	}

	preLabel := "@pre-request"
	if pv.req.PreScript == "" {
		preLabel += " (empty)"
	}

	postLabel := "@post-response"
	if pv.req.PostScript == "" {
		postLabel += " (empty)"
	}

	exLabel := "@example"
	if pv.req.Example == nil {
		exLabel += " (empty)"
	}

	return [4]string{reqLabel, preLabel, postLabel, exLabel}
}

func (pv PartsView) statusLine() string {
	if pv.working {
		return styleStatusBar.Width(pv.width).Render(pv.status)
	}
	hint := "e edit  r run  t test  x save example  j/k move  h/esc back  q quit"
	if pv.status != "" {
		tag := lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("[" + pv.status + "]")
		hint = tag + "  " + hint
	}
	return styleStatusBar.Width(pv.width).Render(hint)
}

// dedent removes the common leading whitespace from all non-empty lines.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent <= 0 {
		return s
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= minIndent {
			out[i] = line[minIndent:]
		} else {
			out[i] = ""
		}
	}
	return strings.Join(out, "\n")
}

func parseRequestEdit(content string) (method, url string, headers []httpfile.Header, body string) {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return
	}
	if m, u, ok := strings.Cut(strings.TrimSpace(lines[0]), " "); ok {
		method = strings.TrimSpace(m)
		url = strings.TrimSpace(u)
	}
	i := 1
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
		if name, val, ok := strings.Cut(lines[i], ":"); ok {
			headers = append(headers, httpfile.Header{
				Name:  strings.TrimSpace(name),
				Value: strings.TrimSpace(val),
			})
		}
		i++
	}
	if i < len(lines) {
		i++ // skip blank separator line
		body = strings.TrimRight(strings.Join(lines[i:], "\n"), "\n")
	}
	return
}

func parseExampleEdit(content string) *httpfile.Example {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	ex := &httpfile.Example{}
	if len(lines) == 0 {
		return ex
	}
	ex.Status = strings.TrimSpace(lines[0])
	i := 1
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
		if name, val, ok := strings.Cut(lines[i], ":"); ok {
			ex.Headers = append(ex.Headers, httpfile.Header{
				Name:  strings.TrimSpace(name),
				Value: strings.TrimSpace(val),
			})
		}
		i++
	}
	if i < len(lines) {
		i++
		ex.Body = strings.TrimRight(strings.Join(lines[i:], "\n"), "\n")
	}
	return ex
}
