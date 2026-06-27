package ui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
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

// Model is the root Bubble Tea model. It owns the current mode and delegates
// all input and rendering to the active sub-model.
type Model struct {
	mode        mode
	browser     Browser
	httpBrowser HttpBrowser
	width       int
	height      int
	env         map[string]string // session variables — persist set() results across requests
	ipcServer   *ipc.Server
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
	return Model{mode: modeBrowser, browser: b, env: make(map[string]string)}, nil
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
	return Model{mode: modeHttpBrowser, browser: b, httpBrowser: hb, env: make(map[string]string)}, nil
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
		return m, nil

	case openFileMsg:
		hb, err := newHttpBrowser(msg.path, m.width, m.height)
		if err != nil {
			return m, nil
		}
		// newHttpBrowser pre-seeds hb.env from .env + @var declarations.
		// Merge session variables on top so set() results from prior requests
		// override the seeded defaults.
		for k, v := range m.env {
			hb.env[k] = v
		}
		hb.ipcServer = m.ipcServer
		m.httpBrowser = hb
		m.mode = modeHttpBrowser
		return m, tea.Batch(hb.watchCmd(), tea.ClearScreen)

	case backMsg:
		m.env = m.httpBrowser.env // persist vars when leaving the http browser
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
