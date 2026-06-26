package ui

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type mode uint8

const (
	modeBrowser mode = iota
	modeFileView
)

// openFileMsg is sent by Browser when the user selects a .http file.
type openFileMsg struct{ path string }

// backMsg is sent by FileView when the user presses esc or h.
type backMsg struct{}

// Model is the root Bubble Tea model. It owns the current mode and delegates
// all input and rendering to the active sub-model.
type Model struct {
	mode     mode
	browser  Browser
	fileView FileView
	width    int
	height   int
}

// New starts in browser mode at the given directory.
func New(cwd string) (Model, error) {
	b, err := newBrowser(cwd)
	if err != nil {
		return Model{}, err
	}
	return Model{mode: modeBrowser, browser: b}, nil
}

// NewAtFile starts directly in file-view mode for the given .http file.
// The browser is pre-loaded at the file's parent directory so that pressing
// h/esc returns to a useful location.
func NewAtFile(path string) (Model, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Model{}, err
	}
	b, _ := newBrowser(filepath.Dir(abs)) // non-fatal if dir unreadable
	fv, err := newFileView(abs, 0, 0)
	if err != nil {
		return Model{}, err
	}
	return Model{mode: modeFileView, browser: b, fileView: fv}, nil
}

func (m Model) Init() tea.Cmd {
	if m.mode == modeFileView {
		return m.fileView.watchCmd()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.browser = m.browser.resize(msg.Width, msg.Height)
		m.fileView = m.fileView.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// Let "q" reach the active sub-model when its search prompt is open
		// so the character is appended to the query buffer.
		searching := (m.mode == modeBrowser && m.browser.search.active) ||
			(m.mode == modeFileView && m.fileView.search.active)
		if msg.String() == "q" && !searching {
			return m, tea.Quit
		}

	case openFileMsg:
		fv, err := newFileView(msg.path, m.width, m.height)
		if err != nil {
			return m, nil
		}
		m.fileView = fv
		m.mode = modeFileView
		return m, fv.watchCmd()

	case backMsg:
		m.mode = modeBrowser
		return m, nil
	}

	switch m.mode {
	case modeBrowser:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.update(msg)
		return m, cmd
	case modeFileView:
		var cmd tea.Cmd
		m.fileView, cmd = m.fileView.update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) View() string {
	switch m.mode {
	case modeBrowser:
		return m.browser.view()
	case modeFileView:
		return m.fileView.view()
	}
	return ""
}

// Start launches the TUI in browser mode at the given directory.
func Start(cwd string) error {
	m, err := New(cwd)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// StartAtFile launches the TUI directly in file-view mode for a .http file.
func StartAtFile(path string) error {
	m, err := NewAtFile(path)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
