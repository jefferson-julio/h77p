package ui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/ipc"
)

type mode uint8

const (
	modeBrowser     mode = iota
	modeHttpBrowser
)

// openFileMsg is sent by Browser when the user selects a .http file.
type openFileMsg struct{ path string }

// backMsg is sent by HttpBrowser when the user presses h/esc in tree mode.
type backMsg struct{}

// switchFileMsg triggers a root file switch (e.g. from an IPC command that
// targets a file not in the current tree) and re-dispatches a pending command
// once the new HttpBrowser is ready.
type switchFileMsg struct {
	path       string
	pendingCmd *ipc.Command
}

// Model is the root Bubble Tea model. It owns the current mode and delegates
// all input and rendering to the active sub-model.
type Model struct {
	mode        mode
	browser     Browser
	httpBrowser HttpBrowser
	width       int
	height      int
	fileEnvs    map[string]map[string]string // per-root-file env snapshots, keyed by abs path
	rootForFile map[string]string            // any file path → its root .http file path
	ipcServer   *ipc.Server
}

// saveFileEnv snapshots the current HttpBrowser's env into fileEnvs.
func (m *Model) saveFileEnv() {
	if m.httpBrowser.filePath == "" {
		return
	}
	m.fileEnvs[m.httpBrowser.filePath] = copyEnv(m.httpBrowser.env)
}

// restoreFileEnv merges any previously saved env for hb.filePath into hb.env.
func (m *Model) restoreFileEnv(hb *HttpBrowser) {
	saved, ok := m.fileEnvs[hb.filePath]
	if !ok {
		return
	}
	for k, v := range saved {
		hb.env[k] = v
	}
}

// indexFileTree records every file in f's import tree as belonging to rootPath.
// Called whenever a new root HttpBrowser is created so we can resolve group
// file references back to their owning root.
func (m *Model) indexFileTree(rootPath string, f *httpfile.File) {
	if f == nil {
		return
	}
	m.rootForFile[f.Path] = rootPath
	for i := range f.Groups {
		if f.Groups[i].File != nil {
			m.indexFileTree(rootPath, f.Groups[i].File)
		}
	}
}

// resolveRoot returns the root file that should be opened when targetPath is
// requested via IPC. If targetPath is a group file of a previously loaded root,
// that root is returned instead so the TUI shows the full context.
func (m *Model) resolveRoot(targetPath string) string {
	if root, ok := m.rootForFile[targetPath]; ok {
		return root
	}
	return targetPath
}

// copyEnv returns a shallow copy of m, safe to pass to a goroutine.
func copyEnv(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// New starts in browser mode at the given directory.
func New(cwd string) (Model, error) {
	b, err := newBrowser(cwd)
	if err != nil {
		return Model{}, err
	}
	return Model{
		mode:        modeBrowser,
		browser:     b,
		fileEnvs:    make(map[string]map[string]string),
		rootForFile: make(map[string]string),
	}, nil
}

// NewAtFile starts directly in http-browser mode for the given .http file.
// The browser is pre-loaded at the file's parent directory so that pressing
// h/esc returns to a useful location.
func NewAtFile(path string) (Model, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Model{}, err
	}
	b, _ := newBrowser(filepath.Dir(abs)) // non-fatal if dir unreadable
	hb, err := newHttpBrowser(abs, 0, 0)
	if err != nil {
		return Model{}, err
	}
	m := Model{
		mode:        modeHttpBrowser,
		browser:     b,
		httpBrowser: hb,
		fileEnvs:    make(map[string]map[string]string),
		rootForFile: make(map[string]string),
	}
	m.indexFileTree(abs, hb.file)
	return m, nil
}

func (m Model) Init() tea.Cmd {
	if m.mode == modeHttpBrowser {
		return m.httpBrowser.watchCmd()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.browser = m.browser.resize(msg.Width, msg.Height)
		m.httpBrowser = m.httpBrowser.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Let "q" reach the active sub-model when its search prompt is open
		// so the character is appended to the query buffer.
		searching := (m.mode == modeBrowser && m.browser.search.active) ||
			(m.mode == modeHttpBrowser && m.httpBrowser.search.active)
		if msg.String() == "q" && !searching {
			return m, tea.Quit
		}

	case ipc.Msg:
		if m.mode == modeHttpBrowser {
			var cmd tea.Cmd
			m.httpBrowser, cmd = m.httpBrowser.handleIPCCommand(msg.Cmd)
			return m, cmd
		}
		// In browser mode: if the command names a file, switch to it directly.
		if msg.Cmd.File != "" {
			abs, err := filepath.Abs(msg.Cmd.File)
			if err == nil {
				pending := msg.Cmd
				return m, func() tea.Msg { return switchFileMsg{path: abs, pendingCmd: &pending} }
			}
		}
		return m, nil

	case switchFileMsg:
		abs, err := filepath.Abs(msg.path)
		if err != nil {
			return m, nil
		}
		// If the target is a group file of a previously loaded root, open that root instead.
		abs = m.resolveRoot(abs)
		// Snapshot outgoing file's env before replacing the model.
		if m.mode == modeHttpBrowser {
			m.saveFileEnv()
			if m.httpBrowser.watchDone != nil {
				close(m.httpBrowser.watchDone)
				m.httpBrowser.watchDone = nil
			}
		}
		hb, err := newHttpBrowser(abs, m.width, m.height)
		if err != nil {
			return m, nil
		}
		m.indexFileTree(abs, hb.file)
		m.restoreFileEnv(&hb)
		hb.ipcServer = m.ipcServer
		m.httpBrowser = hb
		m.mode = modeHttpBrowser
		cmds := []tea.Cmd{hb.watchCmd(), tea.ClearScreen}
		if msg.pendingCmd != nil {
			var pendingTeaCmd tea.Cmd
			m.httpBrowser, pendingTeaCmd = m.httpBrowser.handleIPCCommand(*msg.pendingCmd)
			if pendingTeaCmd != nil {
				cmds = append(cmds, pendingTeaCmd)
			}
		}
		return m, tea.Batch(cmds...)

	case openFileMsg:
		// Snapshot the outgoing file's env before switching.
		if m.mode == modeHttpBrowser {
			m.saveFileEnv()
		}
		hb, err := newHttpBrowser(msg.path, m.width, m.height)
		if err != nil {
			return m, nil
		}
		m.indexFileTree(msg.path, hb.file)
		// Restore this file's previously saved env on top of the freshly seeded defaults.
		m.restoreFileEnv(&hb)
		hb.ipcServer = m.ipcServer
		m.httpBrowser = hb
		m.mode = modeHttpBrowser
		return m, tea.Batch(hb.watchCmd(), tea.ClearScreen)

	case backMsg:
		m.saveFileEnv() // snapshot current file's env before going to browser
		m.mode = modeBrowser
		return m, tea.ClearScreen
	}

	switch m.mode {
	case modeBrowser:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.update(msg)
		return m, cmd
	case modeHttpBrowser:
		var cmd tea.Cmd
		m.httpBrowser, cmd = m.httpBrowser.update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	switch m.mode {
	case modeBrowser:
		return m.browser.view()
	case modeHttpBrowser:
		return m.httpBrowser.view()
	}
	return ""
}

// Start launches the TUI in browser mode at the given directory.
// srv may be nil when IPC is not in use.
func Start(cwd string, srv *ipc.Server) error {
	m, err := New(cwd)
	if err != nil {
		return err
	}
	m.ipcServer = srv
	p := tea.NewProgram(m, tea.WithAltScreen())
	if srv != nil {
		srv.SetProgram(p)
	}
	_, err = p.Run()
	return err
}

// StartAtFile launches the TUI directly in http-browser mode for a .http file.
// srv may be nil when IPC is not in use.
func StartAtFile(path string, srv *ipc.Server) error {
	m, err := NewAtFile(path)
	if err != nil {
		return err
	}
	m.ipcServer = srv
	m.httpBrowser.ipcServer = srv
	p := tea.NewProgram(m, tea.WithAltScreen())
	if srv != nil {
		srv.SetProgram(p)
	}
	_, err = p.Run()
	return err
}
