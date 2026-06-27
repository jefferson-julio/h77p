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
	"github.com/jefferson-julio/h77p/internal/ipc"
	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/jefferson-julio/h77p/internal/writer"
)

// ---------------------------------------------------------------------------
// Tab and part constants
// ---------------------------------------------------------------------------

const (
	tabRequest = iota
	tabRun
	tabTests
	tabLogs
	tabExample
)

const (
	partRequest = iota
	partPreScript
	partJQ
	partPostScript
	partExample
)

// ---------------------------------------------------------------------------
// Tree types
// ---------------------------------------------------------------------------

type browserDepth uint8

const (
	depthTree  browserDepth = iota
	depthParts              // drilling into a request's parts
)

type nodeKind uint8

const (
	nodeKindRequest nodeKind = iota
	nodeKindGroup
)

type treeNode struct {
	kind  nodeKind
	depth int // nesting depth: 0 = top-level, 1 = inside a group, 2 = nested, …
	// group nodes
	groupName string
	groupFile *httpfile.File // file that owns this group (for nil-import check)
	expanded  bool
	// request nodes
	req     httpfile.Request
	reqFile *httpfile.File
	reqPath string
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

type fileChangedMsg struct{}
type editFileDoneMsg struct{ err error }
type requestDoneMsg struct {
	result *runner.Result
	action string // "run" | "test" | "example"
	err    error
	vars   map[string]string
}
type bodyViewerDoneMsg struct{ err error }
type partsEditDoneMsg struct {
	kind    int
	content string
	err     error
}
type envEditDoneMsg struct {
	key   string
	value string
	err   error
}

// ---------------------------------------------------------------------------
// HttpBrowser model
// ---------------------------------------------------------------------------

// HttpBrowser is the unified .http file sub-model. It combines the old
// FileView (request tree) and PartsView (request part editor) into one
// component that also understands request groups from @import directives.
type HttpBrowser struct {
	file     *httpfile.File
	filePath string // root file path (watcher anchor + header title)

	depth browserDepth

	// ── tree mode state ──────────────────────────────────────────────────────
	treeItems      []treeNode
	expandedGroups map[string]bool // false = collapsed; missing = expanded (default)
	visible        []int           // indices into treeItems currently shown
	treeCursor     int
	treeScroll     int
	search         searchInput

	// ── parts mode state ─────────────────────────────────────────────────────
	activeReq   httpfile.Request
	activeFile  *httpfile.File
	activePath  string
	partsCursor int

	// ── shared right panel ────────────────────────────────────────────────────
	preview    viewport.Model
	width      int
	height     int
	working    bool
	status     string
	lastResult *runner.Result
	activeTab  int
	helpOpen   bool

	// ── env panel ─────────────────────────────────────────────────────────────
	env       map[string]string
	envFocus  bool
	envCursor int
	envScroll int

	// ── IPC ───────────────────────────────────────────────────────────────────
	ipcServer *ipc.Server

	// ── file watcher ──────────────────────────────────────────────────────────
	watchDone    chan struct{}
	watchModTime time.Time
}

func newHttpBrowser(path string, w, h int) (HttpBrowser, error) {
	file, err := parser.ParseFile(path)
	if err != nil {
		return HttpBrowser{}, err
	}
	pw := max(rightWidth(w), 1)
	ph := max(contentHeight(h)-1, 1)
	hb := HttpBrowser{
		file:           file,
		filePath:       path,
		preview:        viewport.New(pw, ph),
		width:          w,
		height:         h,
		watchDone:      make(chan struct{}),
		env:            make(map[string]string),
		expandedGroups: make(map[string]bool),
	}
	runner.SeedEnv(hb.file, hb.env)
	if info, err := os.Stat(path); err == nil {
		hb.watchModTime = info.ModTime()
	}
	hb.treeItems = buildTree(file, hb.expandedGroups, 0)
	hb = hb.rebuildVisible()
	hb = hb.withSyncedPreview()
	return hb, nil
}

func (hb HttpBrowser) resize(w, h int) HttpBrowser {
	hb.width, hb.height = w, h
	hb.preview.Width = max(rightWidth(w), 1)
	hb.preview.Height = max(contentHeight(h)-1, 1)
	return hb
}

func (hb HttpBrowser) watchCmd() tea.Cmd {
	if hb.watchDone == nil || hb.file == nil {
		return nil
	}
	return cmdPollFile(hb.filePath, hb.watchModTime, hb.watchDone)
}

// ---------------------------------------------------------------------------
// Tree building
// ---------------------------------------------------------------------------

// buildTree constructs the visible flat treeNode list from a parsed file.
// expandedGroups maps group names to explicit expand state (false = collapsed;
// missing = expanded by default). depth is the nesting level (0 = top-level).
// Children of collapsed groups are not included, so treeItems == visible nodes.
// buildTree constructs the visible flat treeNode list, preserving the document
// order recorded in file.Items (requests and groups interleaved as authored).
func buildTree(file *httpfile.File, expandedGroups map[string]bool, depth int) []treeNode {
	var nodes []treeNode
	for _, item := range file.Items {
		if item.IsGroup {
			g := &file.Groups[item.Index]
			expanded := false
			if v, ok := expandedGroups[g.Name]; ok {
				expanded = v
			}
			nodes = append(nodes, treeNode{
				kind:      nodeKindGroup,
				depth:     depth,
				groupName: g.Name,
				groupFile: file,
				expanded:  expanded,
			})
			if expanded && g.File != nil {
				nodes = append(nodes, buildTree(g.File, expandedGroups, depth+1)...)
			}
		} else {
			nodes = append(nodes, treeNode{
				kind:    nodeKindRequest,
				depth:   depth,
				req:     file.Requests[item.Index],
				reqFile: file,
				reqPath: file.Path,
			})
		}
	}
	return nodes
}

func (hb HttpBrowser) rebuildTree() HttpBrowser {
	hb.treeItems = buildTree(hb.file, hb.expandedGroups, 0)
	return hb.rebuildVisible()
}

// rebuildVisible recomputes the visible index list.
// In normal mode buildTree already omits collapsed children, so all indices
// are visible. In search mode we flatten to matching requests only.
func (hb HttpBrowser) rebuildVisible() HttpBrowser {
	var vis []int
	if hb.search.query != "" {
		q := strings.ToLower(hb.search.query)
		for i, n := range hb.treeItems {
			if n.kind != nodeKindRequest {
				continue
			}
			if strings.Contains(strings.ToLower(n.req.Name), q) ||
				strings.Contains(strings.ToLower(n.req.Method), q) ||
				strings.Contains(strings.ToLower(n.req.URL), q) {
				vis = append(vis, i)
			}
		}
	} else {
		vis = make([]int, len(hb.treeItems))
		for i := range hb.treeItems {
			vis[i] = i
		}
	}
	hb.visible = vis
	if len(vis) > 0 && hb.treeCursor >= len(vis) {
		hb.treeCursor = len(vis) - 1
	}
	return hb
}

// ---------------------------------------------------------------------------
// Selection helpers
// ---------------------------------------------------------------------------

func (hb HttpBrowser) selectedVisibleNode() (treeNode, bool) {
	if len(hb.visible) == 0 || hb.treeCursor >= len(hb.visible) {
		return treeNode{}, false
	}
	idx := hb.visible[hb.treeCursor]
	if idx >= len(hb.treeItems) {
		return treeNode{}, false
	}
	return hb.treeItems[idx], true
}

func (hb HttpBrowser) selectedRequestNode() (treeNode, bool) {
	node, ok := hb.selectedVisibleNode()
	if !ok || node.kind != nodeKindRequest {
		return treeNode{}, false
	}
	return node, true
}

// findRequestInFile searches file and all nested group files for a request by name.
func findRequestInFile(file *httpfile.File, name string) (httpfile.Request, *httpfile.File, bool) {
	for _, req := range file.Requests {
		if req.Name == name {
			return req, file, true
		}
	}
	for _, g := range file.Groups {
		if g.File == nil {
			continue
		}
		if req, f, ok := findRequestInFile(g.File, name); ok {
			return req, f, ok
		}
	}
	return httpfile.Request{}, nil, false
}

// refreshActiveReq updates activeReq/activeFile/activePath from the freshly
// reloaded file tree, matching by request name.
func (hb HttpBrowser) refreshActiveReq() HttpBrowser {
	if hb.activeReq.Name == "" || hb.file == nil {
		return hb
	}
	if req, f, ok := findRequestInFile(hb.file, hb.activeReq.Name); ok {
		hb.activeReq = req
		hb.activeFile = f
		hb.activePath = f.Path
	}
	return hb
}

// ---------------------------------------------------------------------------
// Scroll helpers
// ---------------------------------------------------------------------------

func envPanelHeight(contentH int) int {
	h := contentH * 3 / 10
	if h < 3 {
		h = 3
	}
	return h
}

func (hb HttpBrowser) withTreeScrollAdjusted() HttpBrowser {
	ch := max(contentHeight(hb.height), 1)
	listH := max(ch-envPanelHeight(ch), 1)
	if hb.treeCursor < hb.treeScroll {
		hb.treeScroll = hb.treeCursor
	} else if hb.treeCursor >= hb.treeScroll+listH {
		hb.treeScroll = hb.treeCursor - listH + 1
	}
	return hb
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (hb HttpBrowser) selectedEnvKey() (string, bool) {
	keys := sortedEnvKeys(hb.env)
	if len(keys) == 0 || hb.envCursor >= len(keys) {
		return "", false
	}
	return keys[hb.envCursor], true
}

func (hb HttpBrowser) envMoveCursor(delta int) HttpBrowser {
	n := len(hb.env)
	if n == 0 {
		return hb
	}
	hb.envCursor += delta
	if hb.envCursor < 0 {
		hb.envCursor = 0
	}
	if hb.envCursor >= n {
		hb.envCursor = n - 1
	}
	ch := max(contentHeight(hb.height), 1)
	envH := envPanelHeight(ch)
	visRows := max(envH-1, 0)
	if hb.envCursor < hb.envScroll {
		hb.envScroll = hb.envCursor
	} else if hb.envCursor >= hb.envScroll+visRows {
		hb.envScroll = hb.envCursor - visRows + 1
	}
	return hb
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (hb HttpBrowser) update(msg tea.Msg) (HttpBrowser, tea.Cmd) {
	switch m := msg.(type) {
	case requestDoneMsg:
		return hb.handleRequestDone(m)
	case fileChangedMsg:
		return hb.handleFileChanged()
	case editFileDoneMsg:
		return hb.handleEditFileDone(m)
	case partsEditDoneMsg:
		return hb.handleEditDone(m)
	case envEditDoneMsg:
		return hb.handleEnvEditDone(m)
	case bodyViewerDoneMsg:
		if m.err != nil {
			hb.status = "viewer: " + m.err.Error()
		}
		return hb, nil
	}

	if _, ok := msg.(tea.KeyMsg); !ok {
		var cmd tea.Cmd
		hb.preview, cmd = hb.preview.Update(msg)
		return hb, cmd
	}

	key := msg.(tea.KeyMsg)

	if hb.helpOpen {
		hb.helpOpen = false
		return hb, nil
	}

	if hb.depth == depthTree {
		return hb.updateTree(key)
	}
	return hb.updateParts(key)
}

func (hb HttpBrowser) updateTree(key tea.KeyMsg) (HttpBrowser, tea.Cmd) {
	if hb.search.active {
		oldQuery := hb.search.query
		hb.search = hb.search.handleKey(key)
		if hb.search.query != oldQuery {
			hb = hb.rebuildVisible()
			hb.treeCursor = 0
			hb.treeScroll = 0
			hb = hb.withSyncedPreview()
		}
		return hb, nil
	}

	// When env panel is focused only env-panel keys are handled; everything else
	// is swallowed so no request action fires accidentally.
	if hb.envFocus {
		switch key.String() {
		case "tab", "esc":
			hb.envFocus = false
		case "j", "down":
			hb = hb.envMoveCursor(1)
		case "k", "up":
			hb = hb.envMoveCursor(-1)
		case "e", "enter":
			if k, ok := hb.selectedEnvKey(); ok {
				return hb, hb.cmdEditEnvVar(k, hb.env[k])
			}
		}
		return hb, nil
	}

	n := len(hb.visible)

	switch key.String() {
	case "?":
		hb.helpOpen = true

	case "/":
		hb.search.active = true
		hb.search.pos = len([]rune(hb.search.query))

	case "1":
		hb.activeTab = tabRequest
		hb = hb.withSyncedPreview()
	case "2":
		hb.activeTab = tabRun
		hb = hb.withSyncedPreview()
	case "3":
		hb.activeTab = tabTests
		hb = hb.withSyncedPreview()
	case "4":
		hb.activeTab = tabLogs
		hb = hb.withSyncedPreview()
	case "5":
		hb.activeTab = tabExample
		hb = hb.withSyncedPreview()

	case "tab":
		hb.envFocus = true

	case "j", "down":
		if hb.treeCursor < n-1 {
			hb.treeCursor++
			hb = hb.withTreeScrollAdjusted().withSyncedPreview()
			return hb, hb.cmdIPCCursor()
		}
	case "k", "up":
		if hb.treeCursor > 0 {
			hb.treeCursor--
			hb = hb.withTreeScrollAdjusted().withSyncedPreview()
			return hb, hb.cmdIPCCursor()
		}
	case "g":
		hb.treeCursor = 0
		hb = hb.withTreeScrollAdjusted().withSyncedPreview()
		return hb, hb.cmdIPCCursor()
	case "G":
		if n > 0 {
			hb.treeCursor = n - 1
			hb = hb.withTreeScrollAdjusted().withSyncedPreview()
			return hb, hb.cmdIPCCursor()
		}

	case "enter", "l":
		if n == 0 {
			break
		}
		nodeIdx := hb.visible[hb.treeCursor]
		node := hb.treeItems[nodeIdx]
		if node.kind == nodeKindGroup {
			current := false
			if v, ok := hb.expandedGroups[node.groupName]; ok {
				current = v
			}
			hb.expandedGroups[node.groupName] = !current
			hb = hb.rebuildTree()
			if len(hb.visible) > 0 && hb.treeCursor >= len(hb.visible) {
				hb.treeCursor = len(hb.visible) - 1
			}
			hb = hb.withTreeScrollAdjusted()
		} else {
			hb.activeReq = node.req
			hb.activeFile = node.reqFile
			hb.activePath = node.reqPath
			hb.partsCursor = 0
			hb.depth = depthParts
			hb = hb.withSyncedPreview()
		}

	case "r":
		if !hb.working {
			if node, ok := hb.selectedRequestNode(); ok {
				hb.activeReq = node.req
				hb.activeFile = node.reqFile
				hb.activePath = node.reqPath
				hb.working = true
				hb.status = "running…"
				return hb, hb.cmdRunActive("run")
			}
		}
	case "t":
		if !hb.working {
			if node, ok := hb.selectedRequestNode(); ok {
				hb.activeReq = node.req
				hb.activeFile = node.reqFile
				hb.activePath = node.reqPath
				hb.working = true
				hb.status = "running tests…"
				return hb, hb.cmdRunActive("test")
			}
		}
	case "x":
		if !hb.working {
			if node, ok := hb.selectedRequestNode(); ok {
				hb.activeReq = node.req
				hb.activeFile = node.reqFile
				hb.activePath = node.reqPath
				hb.working = true
				hb.status = "running…"
				return hb, hb.cmdRunActive("example")
			}
		}

	case "o":
		body := ""
		if hb.lastResult != nil && hb.lastResult.HTTP != nil {
			body = hb.lastResult.HTTP.Body
		}
		if body == "" {
			if node, ok := hb.selectedRequestNode(); ok && node.req.Example != nil {
				body = node.req.Example.Body
			}
		}
		if body != "" {
			return hb, cmdOpenBody(body)
		}

	case "e":
		if !hb.working {
			if node, ok := hb.selectedRequestNode(); ok {
				if hb.watchDone != nil {
					close(hb.watchDone)
					hb.watchDone = nil
				}
				return hb, hb.cmdEditRequestBlock(node.reqPath, node.req.Name)
			}
		}
	case "E":
		// Edit the file that owns the current selection: group file when a group
		// request is selected, root file otherwise.
		if !hb.working && hb.file != nil {
			filePath := hb.filePath
			if node, ok := hb.selectedVisibleNode(); ok && node.reqPath != "" {
				filePath = node.reqPath
			}
			if hb.watchDone != nil {
				close(hb.watchDone)
				hb.watchDone = nil
			}
			return hb, hb.cmdEditWholeFile(filePath)
		}

	case "h", "esc":
		if hb.watchDone != nil {
			close(hb.watchDone)
			hb.watchDone = nil
		}
		return hb, func() tea.Msg { return backMsg{} }

	default:
		var cmd tea.Cmd
		hb.preview, cmd = hb.preview.Update(key)
		return hb, cmd
	}
	return hb, nil
}

func (hb HttpBrowser) updateParts(key tea.KeyMsg) (HttpBrowser, tea.Cmd) {
	if hb.envFocus {
		switch key.String() {
		case "tab", "esc":
			hb.envFocus = false
		case "j", "down":
			hb = hb.envMoveCursor(1)
		case "k", "up":
			hb = hb.envMoveCursor(-1)
		case "e", "enter":
			if k, ok := hb.selectedEnvKey(); ok {
				return hb, hb.cmdEditEnvVar(k, hb.env[k])
			}
		}
		return hb, nil
	}

	switch key.String() {
	case "?":
		hb.helpOpen = true

	case "1":
		hb.activeTab = tabRequest
		hb = hb.withSyncedPreview()
	case "2":
		hb.activeTab = tabRun
		hb = hb.withSyncedPreview()
	case "3":
		hb.activeTab = tabTests
		hb = hb.withSyncedPreview()
	case "4":
		hb.activeTab = tabLogs
		hb = hb.withSyncedPreview()
	case "5":
		hb.activeTab = tabExample
		hb = hb.withSyncedPreview()

	case "tab":
		hb.envFocus = true

	case "j", "down":
		if hb.partsCursor < 4 {
			hb.partsCursor++
			hb = hb.withSyncedPreview()
		}
	case "k", "up":
		if hb.partsCursor > 0 {
			hb.partsCursor--
			hb = hb.withSyncedPreview()
		}

	case "e", "enter", "l":
		return hb, hb.cmdEditPart()

	case "o":
		body := ""
		if hb.lastResult != nil && hb.lastResult.HTTP != nil {
			body = hb.lastResult.HTTP.Body
		}
		if body == "" && hb.activeReq.Example != nil {
			body = hb.activeReq.Example.Body
		}
		if body != "" {
			return hb, cmdOpenBody(body)
		}

	case "r":
		if !hb.working {
			hb.working = true
			hb.status = "running…"
			return hb, hb.cmdRunActive("run")
		}
	case "t":
		if !hb.working {
			hb.working = true
			hb.status = "running tests…"
			return hb, hb.cmdRunActive("test")
		}
	case "x":
		if !hb.working {
			hb.working = true
			hb.status = "running…"
			return hb, hb.cmdRunActive("example")
		}

	case "h", "esc":
		hb.depth = depthTree
		hb = hb.withSyncedPreview()

	default:
		var cmd tea.Cmd
		hb.preview, cmd = hb.preview.Update(key)
		return hb, cmd
	}
	return hb, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// IPC helpers
// ---------------------------------------------------------------------------

// cmdIPCSend returns a tea.Cmd that writes e to the IPC client. Returns nil
// (no-op) when srv is nil or no client is connected.
func cmdIPCSend(srv *ipc.Server, e ipc.Event) tea.Cmd {
	if srv == nil {
		return nil
	}
	return func() tea.Msg {
		_ = srv.Send(e)
		return nil
	}
}

// cmdIPCCursor returns a tea.Cmd emitting a "cursor" event for the currently
// selected tree node, or nil if the node is not a request or IPC is inactive.
func (hb HttpBrowser) cmdIPCCursor() tea.Cmd {
	if hb.ipcServer == nil || len(hb.visible) == 0 {
		return nil
	}
	node := hb.treeItems[hb.visible[hb.treeCursor]]
	if node.kind != nodeKindRequest {
		return nil
	}
	return cmdIPCSend(hb.ipcServer, ipc.Event{
		Event:   "cursor",
		File:    node.reqPath,
		Request: node.req.Name,
	})
}

// focusByName moves treeCursor to the first visible request with the given
// name. No-op if the name is not found.
func (hb HttpBrowser) focusByName(name string) HttpBrowser {
	for i, idx := range hb.visible {
		node := hb.treeItems[idx]
		if node.kind == nodeKindRequest && node.req.Name == name {
			hb.treeCursor = i
			hb = hb.withTreeScrollAdjusted()
			return hb
		}
	}
	return hb
}

// handleIPCCommand dispatches a command received from the IPC client.
func (hb HttpBrowser) handleIPCCommand(cmd ipc.Command) (HttpBrowser, tea.Cmd) {
	switch cmd.Cmd {
	case "run":
		if cmd.Request == "" {
			break
		}
		req, file, ok := findRequestInFile(hb.file, cmd.Request)
		if !ok {
			break
		}
		hb.activeReq = req
		hb.activeFile = file
		hb.activePath = file.Path
		hb.working = true
		hb.status = "running…"
		hb = hb.focusByName(cmd.Request)
		hb = hb.withSyncedPreview()
		return hb, hb.cmdRunActive("run")

	case "focus":
		if cmd.Request != "" {
			hb = hb.focusByName(cmd.Request)
			hb = hb.withSyncedPreview()
		}

	case "get_state":
		return hb, cmdIPCSend(hb.ipcServer, ipc.Event{
			Event:   "state",
			File:    hb.filePath,
			Request: hb.activeReq.Name,
			Env:     copyEnv(hb.env),
		})

	case "set_env", "save_edit":
		if cmd.Key != "" {
			if hb.env == nil {
				hb.env = make(map[string]string)
			}
			val := cmd.Value
			if cmd.Cmd == "save_edit" {
				val = cmd.Content
			}
			hb.env[cmd.Key] = val
		}
	}
	return hb, nil
}

func (hb HttpBrowser) cmdRunActive(action string) tea.Cmd {
	if hb.activeFile == nil {
		return nil
	}
	file := hb.activeFile
	reqName := hb.activeReq.Name
	vars := copyEnv(hb.env)
	return func() tea.Msg {
		result, err := runner.Run(file, reqName, vars)
		return requestDoneMsg{result: result, action: action, err: err, vars: vars}
	}
}

func (hb HttpBrowser) cmdEditWholeFile(filePath string) tea.Cmd {
	if hb.ipcServer != nil && hb.ipcServer.Connected() {
		return cmdIPCSend(hb.ipcServer, ipc.Event{Event: "open_file", File: filePath})
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return tea.ExecProcess(exec.Command(editor, filePath), func(err error) tea.Msg {
		return editFileDoneMsg{err: err}
	})
}

func (hb HttpBrowser) cmdEditRequestBlock(filePath, reqName string) tea.Cmd {
	if hb.ipcServer != nil && hb.ipcServer.Connected() {
		line, _ := writer.FindRequestLine(filePath, reqName)
		return cmdIPCSend(hb.ipcServer, ipc.Event{
			Event:   "open_file",
			File:    filePath,
			Request: reqName,
			Line:    line,
		})
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	block, err := writer.ExtractRequestBlock(filePath, reqName)
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
	return tea.ExecProcess(exec.Command(editor, tmpPath), func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpPath)
			return editFileDoneMsg{err: err}
		}
		data, _ := os.ReadFile(tmpPath)
		os.Remove(tmpPath)
		return editFileDoneMsg{err: writer.SaveRequestBlock(filePath, reqName, string(data))}
	})
}

func (hb HttpBrowser) cmdEditPart() tea.Cmd {
	kind := hb.partsCursor
	content := hb.partsEditorContent()

	if hb.ipcServer != nil && hb.ipcServer.Connected() && hb.activeFile != nil {
		var prefix string
		switch kind {
		case partRequest:
			prefix = "" // locate the HTTP method line
		case partPreScript:
			prefix = "@pre-request"
		case partJQ:
			prefix = "@jq"
		case partPostScript:
			prefix = "@post-response"
		case partExample:
			prefix = "@example"
		}
		filePath := hb.activeFile.Path
		reqName := hb.activeReq.Name
		line, _ := writer.FindPartLine(filePath, reqName, prefix)
		return cmdIPCSend(hb.ipcServer, ipc.Event{
			Event:   "open_file",
			File:    filePath,
			Request: reqName,
			Line:    line,
		})
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	ext := ".http"
	switch kind {
	case partPreScript, partPostScript:
		ext = ".js"
	case partJQ:
		ext = ".jq"
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

func (hb HttpBrowser) cmdEditEnvVar(key, value string) tea.Cmd {
	if hb.ipcServer != nil && hb.ipcServer.Connected() {
		return cmdIPCSend(hb.ipcServer, ipc.Event{Event: "open_edit", Key: key, Value: value})
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	tmp, err := os.CreateTemp("", "h77p-env-*")
	if err != nil {
		return func() tea.Msg { return envEditDoneMsg{key: key, err: err} }
	}
	_, _ = tmp.WriteString(value)
	tmp.Close()
	tmpPath := tmp.Name()
	return tea.ExecProcess(exec.Command(editor, tmpPath), func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return envEditDoneMsg{key: key, err: err}
		}
		data, _ := os.ReadFile(tmpPath)
		return envEditDoneMsg{key: key, value: strings.TrimRight(string(data), "\n\r")}
	})
}

func (hb HttpBrowser) handleEnvEditDone(m envEditDoneMsg) (HttpBrowser, tea.Cmd) {
	if m.err == nil && m.key != "" {
		if hb.env == nil {
			hb.env = make(map[string]string)
		}
		hb.env[m.key] = m.value
	}
	return hb, nil
}

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

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (hb HttpBrowser) handleRequestDone(msg requestDoneMsg) (HttpBrowser, tea.Cmd) {
	hb.working = false
	if msg.vars != nil {
		hb.env = msg.vars
		hb.envScroll = 0
	}
	if msg.err != nil {
		hb.status = "error: " + msg.err.Error()
		return hb, nil
	}

	hb.lastResult = msg.result
	hb.activeTab = tabRun

	if msg.result != nil && msg.result.HTTP != nil {
		h := msg.result.HTTP
		hb.status = fmt.Sprintf("%s  %dms", h.Status, h.Duration.Milliseconds())
		if len(msg.result.Tests) > 0 {
			passed, failed := 0, 0
			for _, t := range msg.result.Tests {
				if t.Passed {
					passed++
				} else {
					failed++
				}
			}
			hb.status += fmt.Sprintf("  ·  %d/%d tests passed", passed, passed+failed)
		}
	}

	if msg.action == "example" && msg.result != nil && msg.result.HTTP != nil && hb.activePath != "" {
		ex := httpResultToExample(msg.result.HTTP)
		if msg.result.JQOutput != "" {
			ex.Body = msg.result.JQOutput
		}
		if err := writer.SaveExample(hb.activePath, hb.activeReq.Name, ex); err != nil {
			hb.status += "  (save failed: " + err.Error() + ")"
		} else {
			hb.status += "  ·  example saved"
			if updated, err := parser.ParseFile(hb.filePath); err == nil {
				hb.file = updated
				hb = hb.rebuildTree()
				hb = hb.refreshActiveReq()
			}
		}
	}

	hb = hb.withSyncedPreview()

	if hb.ipcServer != nil && msg.result != nil && msg.result.HTTP != nil {
		return hb, cmdIPCSend(hb.ipcServer, ipc.Event{
			Event:      "request_done",
			File:       hb.activePath,
			Request:    hb.activeReq.Name,
			Status:     msg.result.HTTP.StatusCode,
			DurationMs: msg.result.HTTP.Duration.Milliseconds(),
			Passed:     msg.result.Passed,
		})
	}
	return hb, nil
}

func (hb HttpBrowser) handleFileChanged() (HttpBrowser, tea.Cmd) {
	if hb.file == nil {
		return hb, hb.watchCmd()
	}

	updated, err := parser.ParseFile(hb.filePath)
	if err == nil {
		// Preserve cursor position by item identity.
		var cursorKey string
		if node, ok := hb.selectedVisibleNode(); ok {
			if node.kind == nodeKindRequest {
				cursorKey = "req:" + node.req.Name
			} else {
				cursorKey = "grp:" + node.groupName
			}
		}

		hb.file = updated
		runner.SeedEnv(hb.file, hb.env)
		hb = hb.rebuildTree()

		if cursorKey != "" {
			for i, idx := range hb.visible {
				n := hb.treeItems[idx]
				var key string
				if n.kind == nodeKindRequest {
					key = "req:" + n.req.Name
				} else {
					key = "grp:" + n.groupName
				}
				if key == cursorKey {
					hb.treeCursor = i
					break
				}
			}
		}
		hb = hb.withTreeScrollAdjusted()
		hb = hb.refreshActiveReq()
		hb = hb.withSyncedPreview()
	}

	if info, err := os.Stat(hb.filePath); err == nil {
		hb.watchModTime = info.ModTime()
	}
	return hb, cmdPollFile(hb.filePath, hb.watchModTime, hb.watchDone)
}

func (hb HttpBrowser) handleEditFileDone(msg editFileDoneMsg) (HttpBrowser, tea.Cmd) {
	if msg.err != nil {
		hb.status = "edit error: " + msg.err.Error()
	}
	hb.watchDone = make(chan struct{})
	return hb.handleFileChanged()
}

func (hb HttpBrowser) handleEditDone(msg partsEditDoneMsg) (HttpBrowser, tea.Cmd) {
	if msg.err != nil {
		hb.status = "edit error: " + msg.err.Error()
		return hb, nil
	}
	if err := hb.partsApplyEdit(msg.kind, msg.content); err != nil {
		hb.status = "save error: " + err.Error()
		return hb, nil
	}
	updated, err := parser.ParseFile(hb.filePath)
	if err != nil {
		hb.status = "reload error: " + err.Error()
		return hb, nil
	}
	hb.file = updated
	hb = hb.rebuildTree()
	hb = hb.refreshActiveReq()
	hb.status = "saved"
	hb.activeTab = tabRequest
	hb = hb.withSyncedPreview()
	return hb, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (hb HttpBrowser) view() string {
	if hb.width == 0 {
		return ""
	}
	if hb.helpOpen {
		if hb.depth == depthParts {
			return renderHelpOverlay(hb.width, hb.height, helpPartsView)
		}
		return renderHelpOverlay(hb.width, hb.height, helpFileView)
	}

	lw := leftWidth(hb.width)
	rw := rightWidth(hb.width)
	ch := max(contentHeight(hb.height), 1)
	envH := envPanelHeight(ch)
	listH := ch - envH

	var headerText string
	if hb.depth == depthParts {
		headerText = hb.activeReq.Name
		if headerText == "" {
			headerText = hb.activeReq.Method + " " + hb.activeReq.URL
		}
	} else {
		if hb.file != nil {
			headerText = filepath.Base(hb.filePath)
		} else {
			headerText = "(no file)"
		}
	}
	header := styleHeader.Width(hb.width).Render(headerText)

	left := append(hb.buildLeftLines(lw, listH), hb.buildEnvLines(lw, envH)...)
	tabBar := renderTabBar(hb.activeTab, rw)
	vpLines := strings.Split(hb.preview.View(), "\n")
	right := append([]string{tabBar}, vpLines...)
	body := zipPanels(left, right, lw, rw, ch)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, hb.statusLine())
}

func (hb HttpBrowser) buildLeftLines(w, h int) []string {
	if hb.depth == depthParts {
		return hb.buildLeftLinesParts(w, h)
	}
	return hb.buildLeftLinesTree(w, h)
}

func (hb HttpBrowser) buildLeftLinesTree(w, h int) []string {
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	for screenIdx := range h {
		dataIdx := hb.treeScroll + screenIdx
		if dataIdx >= len(hb.visible) {
			break
		}
		nodeIdx := hb.visible[dataIdx]
		node := hb.treeItems[nodeIdx]
		isCursor := dataIdx == hb.treeCursor
		lines[screenIdx] = hb.renderTreeItem(node, isCursor, w)
	}
	return lines
}

func (hb HttpBrowser) renderTreeItem(node treeNode, isCursor bool, w int) string {
	prefix := "  "
	if isCursor {
		prefix = "▸ "
	}

	extra := strings.Repeat("  ", node.depth) // 2 spaces per nesting level

	switch node.kind {
	case nodeKindGroup:
		glyph := "▶"
		if node.expanded {
			glyph = "▼"
		}
		label := glyph + " [" + node.groupName + "]"
		// Show (!) when the imported file failed to load.
		if node.groupFile != nil {
			for _, g := range node.groupFile.Groups {
				if g.Name == node.groupName && g.File == nil {
					label += styleDim.Render(" (!)")
					break
				}
			}
		}
		full := prefix + extra + label
		if isCursor {
			return styleCursor.Width(w).Render(truncate(full, w))
		}
		return styleTabActive.Width(w).Render(truncate(full, w))

	case nodeKindRequest:
		req := node.req
		if isCursor {
			var plain string
			if req.Name != "" {
				plain = fmt.Sprintf("%s%s%-7s %s", prefix, extra, req.Method, req.Name)
			} else {
				plain = fmt.Sprintf("%s%s%-7s %s", prefix, extra, req.Method, req.URL)
			}
			return styleCursor.Width(w).Render(truncate(plain, w))
		}
		ms, ok := styleMethod[req.Method]
		if !ok {
			ms = lipgloss.NewStyle()
		}
		if req.Name != "" {
			main := ms.Render(fmt.Sprintf("%s%s%-7s %s", prefix, extra, req.Method, req.Name))
			tail := styleDim.Render("  " + req.URL)
			return lipgloss.NewStyle().Width(w).Render(ansi.Truncate(main+tail, w, ""))
		}
		label := truncate(fmt.Sprintf("%s%s%-7s %s", prefix, extra, req.Method, req.URL), w)
		return ms.Width(w).Render(label)
	}
	return strings.Repeat(" ", w)
}

func (hb HttpBrowser) buildLeftLinesParts(w, h int) []string {
	labels := hb.partLabels()
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
		if i == hb.partsCursor {
			prefix = "▸ "
		}
		plain := prefix + label
		if i == hb.partsCursor {
			lines[i] = styleCursor.Width(w).Render(truncate(plain, w))
		} else {
			lines[i] = styleDim.Width(w).Render(truncate(plain, w))
		}
	}
	return lines
}

func (hb HttpBrowser) buildEnvLines(w, h int) []string {
	return renderEnvPanel(hb.env, hb.envFocus, hb.envScroll, hb.envCursor, w, h)
}

func (hb HttpBrowser) statusLine() string {
	bg := activeTheme.BgSubtle
	base := lipgloss.NewStyle().Background(bg).Foreground(activeTheme.FgDim)
	accent := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("220"))
	sep := base.Render("  ")

	if hb.search.active {
		return renderStatusBar(bg, hb.search.renderPrompt(), hb.width)
	}
	if hb.working {
		return renderStatusBar(bg, base.Render(hb.status), hb.width)
	}

	var hint string
	if hb.envFocus {
		hint = base.Render("j/k move  e/enter edit value  tab/esc back to list")
	} else if hb.depth == depthParts {
		hint = base.Render("e edit  r run  o open body  j/k parts  tab env  ? help")
	} else {
		hint = base.Render("r run  e edit request  E edit file  enter inspect  / search  tab env  ? help")
	}

	var prefixes []string
	if hb.status != "" {
		prefixes = append(prefixes, accent.Render("["+hb.status+"]"))
	}
	if hb.search.query != "" {
		prefixes = append(prefixes, accent.Render("[/"+hb.search.query+"]"))
	}
	var content string
	if len(prefixes) > 0 {
		content = strings.Join(append(prefixes, hint), sep)
	} else {
		content = hint
	}
	return renderStatusBar(bg, content, hb.width)
}

// ---------------------------------------------------------------------------
// Right panel content
// ---------------------------------------------------------------------------

func (hb HttpBrowser) withSyncedPreview() HttpBrowser {
	if hb.depth == depthParts {
		switch hb.activeTab {
		case tabRun:
			hb.preview.SetContent(renderHTTPResult(hb.lastResult))
		case tabTests:
			hb.preview.SetContent(renderTests(hb.lastResult))
		case tabLogs:
			hb.preview.SetContent(renderLogs(hb.lastResult))
		case tabExample:
			hb.preview.SetContent(renderExample(hb.activeReq))
		default:
			hb.preview.SetContent(hb.partsPreviewContent())
		}
		hb.preview.GotoTop()
		return hb
	}

	// Tree mode
	switch hb.activeTab {
	case tabRun:
		hb.preview.SetContent(renderHTTPResult(hb.lastResult))
	case tabTests:
		hb.preview.SetContent(renderTests(hb.lastResult))
	case tabLogs:
		hb.preview.SetContent(renderLogs(hb.lastResult))
	case tabExample:
		if hb.activeFile != nil {
			hb.preview.SetContent(renderExample(hb.activeReq))
		} else {
			hb.preview.SetContent(styleDim.Render("(no request selected)"))
		}
	default: // tabRequest
		node, ok := hb.selectedVisibleNode()
		if !ok {
			msg := "(no requests)"
			if hb.search.query != "" {
				msg = "(no matches)"
			}
			hb.preview.SetContent(styleDim.Render(msg))
		} else if node.kind == nodeKindGroup {
			if node.groupFile != nil {
				for _, g := range node.groupFile.Groups {
					if g.Name == node.groupName {
						hb.preview.SetContent(renderGroup(g))
						break
					}
				}
			}
		} else {
			hb.activeReq = node.req
			hb.activeFile = node.reqFile
			hb.activePath = node.reqPath
			hb.preview.SetContent(renderRequest(node.req))
		}
	}
	hb.preview.GotoTop()
	return hb
}

func (hb HttpBrowser) partsPreviewContent() string {
	req := hb.activeReq
	switch hb.partsCursor {
	case partRequest:
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s\n", req.Method, req.URL)
		for _, h := range req.Headers {
			fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
		}
		if req.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", req.Body)
		}
		return highlightHTTP(b.String())
	case partPreScript:
		if req.PreScript == "" {
			return styleDim.Render("(empty — press e to create)")
		}
		return highlightHTTP(fmt.Sprintf("@pre-request {%%\n%s\n%%}", req.PreScript))
	case partJQ:
		if len(req.JQFilters) == 0 {
			return styleDim.Render("(empty — press e to add filters)")
		}
		var b strings.Builder
		for _, f := range req.JQFilters {
			fmt.Fprintf(&b, "@jq %s\n", f)
		}
		return highlightHTTP(b.String())
	case partPostScript:
		if req.PostScript == "" {
			return styleDim.Render("(empty — press e to create)")
		}
		return highlightHTTP(fmt.Sprintf("@post-response {%%\n%s\n%%}", req.PostScript))
	case partExample:
		if req.Example == nil {
			return styleDim.Render("(empty — press e to create)")
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
	return ""
}

// ---------------------------------------------------------------------------
// Parts helpers
// ---------------------------------------------------------------------------

func (hb HttpBrowser) partLabels() [5]string {
	req := hb.activeReq
	reqLabel := req.Method + " " + req.URL
	if req.Name != "" {
		reqLabel = req.Method + " " + req.Name
	}
	preLabel := "@pre-request"
	if req.PreScript == "" {
		preLabel += " (empty)"
	}
	jqLabel := "@jq"
	switch len(req.JQFilters) {
	case 0:
		jqLabel += " (empty)"
	case 1:
		jqLabel += " (1 filter)"
	default:
		jqLabel += fmt.Sprintf(" (%d filters)", len(req.JQFilters))
	}
	postLabel := "@post-response"
	if req.PostScript == "" {
		postLabel += " (empty)"
	}
	exLabel := "@example"
	if req.Example == nil {
		exLabel += " (empty)"
	}
	return [5]string{reqLabel, preLabel, jqLabel, postLabel, exLabel}
}

func (hb HttpBrowser) partsEditorContent() string {
	req := hb.activeReq
	switch hb.partsCursor {
	case partRequest:
		var b strings.Builder
		fmt.Fprintf(&b, "%s %s\n", req.Method, req.URL)
		for _, h := range req.Headers {
			fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
		}
		if req.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", req.Body)
		}
		return b.String()
	case partPreScript:
		if req.PreScript != "" {
			return dedent(req.PreScript)
		}
		return "// pre-request script\n// request.headers[\"X-Custom\"] = \"value\";\n"
	case partJQ:
		if len(req.JQFilters) > 0 {
			return strings.Join(req.JQFilters, "\n") + "\n"
		}
		return "# @jq filters — one filter per line\n# .items[] | select(.active == true)\n"
	case partPostScript:
		if req.PostScript != "" {
			return dedent(req.PostScript)
		}
		return "// post-response script\n// test(\"status 200\", () => { assert(response.status === 200); });\n"
	case partExample:
		if req.Example != nil {
			var b strings.Builder
			fmt.Fprintf(&b, "%s\n", req.Example.Status)
			for _, h := range req.Example.Headers {
				fmt.Fprintf(&b, "%s: %s\n", h.Name, h.Value)
			}
			if req.Example.Body != "" {
				fmt.Fprintf(&b, "\n%s\n", req.Example.Body)
			}
			return b.String()
		}
		return "HTTP/1.1 200 OK\nContent-Type: application/json\n\n{}\n"
	}
	return ""
}

func (hb HttpBrowser) partsApplyEdit(kind int, content string) error {
	content = strings.TrimRight(content, "\n")
	switch kind {
	case partRequest:
		method, url, headers, body := parseRequestEdit(content)
		return writer.SaveRequestLines(hb.activePath, hb.activeReq.Name, method, url, headers, body)
	case partPreScript:
		return writer.SaveScript(hb.activePath, hb.activeReq.Name, "pre-request", content)
	case partJQ:
		var filters []string
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			filters = append(filters, line)
		}
		return writer.SaveJQFilters(hb.activePath, hb.activeReq.Name, filters)
	case partPostScript:
		return writer.SaveScript(hb.activePath, hb.activeReq.Name, "post-response", content)
	case partExample:
		ex := parseExampleEdit(content)
		return writer.SaveExample(hb.activePath, hb.activeReq.Name, ex)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Shared renderers (moved from fileview.go)
// ---------------------------------------------------------------------------

func renderGroup(g httpfile.Group) string {
	var b strings.Builder
	b.WriteString(clrSection.Render("["+g.Name+"]") + "\n")
	if g.Source != "" {
		b.WriteString(styleDim.Render("@import "+g.Source) + "\n")
	}
	if g.File == nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("(import file not found)") + "\n")
		return b.String()
	}
	if len(g.File.Requests) == 0 {
		b.WriteString("\n" + styleDim.Render("(no requests)") + "\n")
		return b.String()
	}
	b.WriteString("\n")
	for _, req := range g.File.Requests {
		ms, ok := styleMethod[req.Method]
		if !ok {
			ms = lipgloss.NewStyle()
		}
		name := req.Name
		if name == "" {
			name = req.URL
		}
		b.WriteString(ms.Render(fmt.Sprintf("%-7s", req.Method)) + " " + name + "\n")
		b.WriteString(styleDim.Render("  "+req.URL) + "\n")
	}
	return b.String()
}

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

// ---------------------------------------------------------------------------
// Utility functions (moved from partsview.go)
// ---------------------------------------------------------------------------

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
		i++
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
