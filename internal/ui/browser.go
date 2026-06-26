package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type dirEntry struct {
	name  string
	path  string
	isDir bool
}

// Browser is the file-system navigator sub-model. It lists directories and
// .http files, letting the user drill in (l/enter) or go up (h/-).
type Browser struct {
	cwd       string
	entries   []dirEntry
	cursor    int
	scrollTop int // index of the first visible entry in the left panel
	preview   string
	width     int
	height    int

	search   searchInput
	filtered []int // indices into entries that pass the current filter

	helpOpen bool
}

func newBrowser(cwd string) (Browser, error) {
	b := Browser{cwd: cwd}
	var err error
	b.entries, err = loadEntries(cwd)
	if err != nil {
		return b, err
	}
	b.filtered = b.computeFiltered()
	return b.withSyncedPreview(), nil
}

func loadEntries(dir string) ([]dirEntry, error) {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := []dirEntry{}
	parent := filepath.Dir(dir)
	if parent != dir {
		entries = append(entries, dirEntry{name: "..", path: parent, isDir: true})
	}
	for _, e := range raw {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // skip hidden entries
		}
		if e.IsDir() || strings.HasSuffix(name, ".http") {
			entries = append(entries, dirEntry{
				name:  name,
				path:  filepath.Join(dir, name),
				isDir: e.IsDir(),
			})
		}
	}
	return entries, nil
}

// computeFiltered rebuilds the filtered index list from the current search query.
func (b Browser) computeFiltered() []int {
	if b.search.query == "" {
		all := make([]int, len(b.entries))
		for i := range all {
			all[i] = i
		}
		return all
	}
	q := strings.ToLower(b.search.query)
	var out []int
	for i, e := range b.entries {
		if strings.Contains(strings.ToLower(e.name), q) {
			out = append(out, i)
		}
	}
	return out
}

// withSyncedPreview refreshes the cached right-panel content to match the
// current cursor position. Call after any cursor or directory change.
func (b Browser) withSyncedPreview() Browser {
	if len(b.filtered) == 0 {
		if b.search.query != "" {
			b.preview = styleDim.Render("(no matches)")
		} else {
			b.preview = ""
		}
		return b
	}
	e := b.entries[b.filtered[b.cursor]]
	if e.isDir {
		b.preview = b.buildDirPreview(e.path)
	} else {
		data, err := os.ReadFile(e.path)
		if err != nil {
			b.preview = styleDim.Render("(error reading file)")
		} else if strings.HasSuffix(e.path, ".http") {
			b.preview = highlightHTTP(string(data))
		} else {
			b.preview = string(data)
		}
	}
	return b
}

func (b Browser) buildDirPreview(path string) string {
	entries, err := loadEntries(path)
	if err != nil {
		return styleDim.Render("(cannot read directory)")
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.name
		if e.isDir {
			lines = append(lines, styleDir.Render(name+"/"))
		} else {
			lines = append(lines, name)
		}
	}
	return strings.Join(lines, "\n")
}

func (b Browser) resize(w, h int) Browser {
	b.width, b.height = w, h
	return b
}

// withScrollAdjusted ensures scrollTop keeps the cursor within the visible window.
func (b Browser) withScrollAdjusted() Browser {
	ch := max(contentHeight(b.height), 1)
	if b.cursor < b.scrollTop {
		b.scrollTop = b.cursor
	} else if b.cursor >= b.scrollTop+ch {
		b.scrollTop = b.cursor - ch + 1
	}
	return b
}

func (b Browser) update(msg tea.Msg) (Browser, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return b, nil
	}

	if b.helpOpen {
		b.helpOpen = false
		return b, nil
	}

	// While searching, route all keystrokes through the search input.
	if b.search.active {
		oldQuery := b.search.query
		b.search = b.search.handleKey(key)
		if b.search.query != oldQuery {
			b.filtered = b.computeFiltered()
			b.cursor = 0
			b.scrollTop = 0
			b = b.withSyncedPreview()
		}
		return b, nil
	}

	n := len(b.filtered)
	switch key.String() {
	case "?":
		b.helpOpen = true

	case "/":
		b.search.active = true
		// Position cursor at end of any existing query so the user can refine
		// or backspace to clear it.
		b.search.pos = len([]rune(b.search.query))

	case "j", "down":
		if b.cursor < n-1 {
			b.cursor++
			b = b.withScrollAdjusted().withSyncedPreview()
		}
	case "k", "up":
		if b.cursor > 0 {
			b.cursor--
			b = b.withScrollAdjusted().withSyncedPreview()
		}
	case "g":
		b.cursor = 0
		b = b.withScrollAdjusted().withSyncedPreview()
	case "G":
		if n > 0 {
			b.cursor = n - 1
			b = b.withScrollAdjusted().withSyncedPreview()
		}
	case "enter", "l":
		if n == 0 {
			break
		}
		e := b.entries[b.filtered[b.cursor]]
		if e.isDir {
			entries, err := loadEntries(e.path)
			if err == nil {
				b.cwd = e.path
				b.entries = entries
				b.cursor = 0
				b.scrollTop = 0
				b.search = searchInput{} // clear filter on directory change
				b.filtered = b.computeFiltered()
				b = b.withSyncedPreview()
			}
		} else {
			return b, func() tea.Msg { return openFileMsg{path: e.path} }
		}
	case "h", "-", "backspace":
		parent := filepath.Dir(b.cwd)
		if parent == b.cwd {
			break
		}
		entries, err := loadEntries(parent)
		if err == nil {
			b.cwd = parent
			b.entries = entries
			b.cursor = 0
			b.scrollTop = 0
			b.search = searchInput{} // clear filter on directory change
			b.filtered = b.computeFiltered()
			b = b.withSyncedPreview()
		}
	}
	return b, nil
}

func (b Browser) view() string {
	if b.width == 0 {
		return ""
	}
	if b.helpOpen {
		return renderHelpOverlay(b.width, b.height, helpBrowser)
	}
	lw := leftWidth(b.width)
	rw := rightWidth(b.width)
	ch := max(contentHeight(b.height), 1)

	header := styleHeader.Width(b.width).Render(b.cwd)
	left := b.buildLeftLines(lw, ch)
	right := buildLinesFromString(b.preview, rw, ch)
	body := zipPanels(left, right, lw, rw, ch)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, b.statusLine())
}

func (b Browser) buildLeftLines(w, h int) []string {
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	for screenIdx := range h {
		dataIdx := b.scrollTop + screenIdx
		if dataIdx >= len(b.filtered) {
			break
		}
		eIdx := b.filtered[dataIdx]
		e := b.entries[eIdx]
		prefix := "  "
		if dataIdx == b.cursor {
			prefix = "▸ "
		}
		name := e.name
		if e.isDir {
			name += "/"
		}
		label := truncate(prefix+name, w)
		switch {
		case dataIdx == b.cursor:
			lines[screenIdx] = styleCursor.Width(w).Render(label)
		case e.isDir:
			lines[screenIdx] = styleDir.Width(w).Render(label)
		default:
			lines[screenIdx] = lipgloss.NewStyle().Width(w).Render(label)
		}
	}
	return lines
}

// buildLinesFromString splits a string into at most h display lines,
// each truncated to w visible characters (ANSI-aware).
func buildLinesFromString(content string, w, h int) []string {
	lines := make([]string, h)
	blank := strings.Repeat(" ", w)
	for i := range lines {
		lines[i] = blank
	}
	if content == "" {
		return lines
	}
	raw := strings.Split(content, "\n")
	for i := 0; i < min(len(raw), h); i++ {
		// Expand tabs to 4 spaces so terminal tab-stop expansion doesn't
		// misalign rows (ansi.StringWidth counts \t as 1 col, terminals don't).
		lines[i] = ansi.Truncate(strings.ReplaceAll(raw[i], "\t", "    "), w, "")
	}
	return lines
}

func (b Browser) statusLine() string {
	bg := activeTheme.BgSubtle
	base := lipgloss.NewStyle().Background(bg).Foreground(activeTheme.FgDim)
	accent := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("220"))

	if b.search.active {
		return renderStatusBar(bg, b.search.renderPrompt(), b.width)
	}
	hint := base.Render("j/k move  enter open  h/- up  / search  ? help")
	if b.search.query != "" {
		tag := accent.Render(fmt.Sprintf("[/%s]", b.search.query))
		hint = tag + base.Render("  ") + hint
	}
	return renderStatusBar(bg, hint, b.width)
}
