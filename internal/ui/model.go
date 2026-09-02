package ui

import (
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"pw/internal/gitinfo"
	"pw/internal/preview"
	"pw/internal/project"
	"pw/internal/state"
	"pw/internal/term"
)

// styles holds all lipgloss styles bound to the real tty renderer.
type styles struct {
	header      lipgloss.Style
	gitRepo     lipgloss.Style
	behindTag   lipgloss.Style
	aheadTag    lipgloss.Style
	normal      lipgloss.Style
	cursor      lipgloss.Style
	recentTag   lipgloss.Style
	favoriteTag lipgloss.Style
	help        lipgloss.Style
	previewHead lipgloss.Style
	sep         lipgloss.Style
	paneBorder  lipgloss.Style
	renderer    *lipgloss.Renderer
}

// newStyles constructs all styles from a renderer bound to the real tty output.
// Using r.NewStyle() (not lipgloss.NewStyle()) ensures the renderer sees the
// tty's color capability instead of the default renderer's pipe-detected profile.
func newStyles(r *lipgloss.Renderer) styles {
	return styles{
		header:      r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		gitRepo:     r.NewStyle().Foreground(lipgloss.Color("10")),
		behindTag:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("9")),
		aheadTag:    r.NewStyle().Bold(true).Foreground(lipgloss.Color("10")),
		normal:      r.NewStyle(),
		cursor:      r.NewStyle().Reverse(true).Bold(true),
		recentTag:   r.NewStyle().Foreground(lipgloss.Color("8")),
		favoriteTag: r.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
		help:        r.NewStyle().Foreground(lipgloss.Color("8")),
		previewHead: r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		sep:         r.NewStyle().Foreground(lipgloss.Color("8")),
		paneBorder:  r.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("8")),
		renderer:    r,
	}
}

// right-pane view modes
const (
	modeGit   = 0
	modeFiles = 1
)

// navFrame stores the state of a navigation level for the drill-down stack.
type navFrame struct {
	dir         string
	items       []project.Project
	cursor      int
	filterValue string
}

// previewLoadedMsg is sent when async preview data is ready.
type previewLoadedMsg struct {
	path string
	data preview.Preview
}

// pullResultMsg is sent when an async `git pull` finishes.
type pullResultMsg struct {
	path   string
	output string
	err    error
}

// termTabResultMsg is sent when an async "open new terminal tab" attempt finishes.
type termTabResultMsg struct {
	err error
}

// explorerResultMsg is sent when an async "open Explorer" attempt finishes.
type explorerResultMsg struct {
	err error
}

// editorResultMsg is sent when an async "launch editor" attempt finishes.
type editorResultMsg struct {
	err error
}

// projectSource implements fuzzy.Source for fuzzy matching.
type projectSource struct {
	projects []project.Project
}

func (ps projectSource) String(i int) string { return ps.projects[i].Name }
func (ps projectSource) Len() int            { return len(ps.projects) }

// Model is the bubbletea model.
type Model struct {
	root          string
	currentDir    string
	stack         []navFrame
	all           []project.Project
	recent        map[string]int64
	store         *state.Store
	favoritesOnly bool
	filterInput   textinput.Model
	cursor        int
	filtered      []project.Project
	previewVP     viewport.Model
	previewCache  map[string]preview.Preview
	selectedPath  string
	confirmed     bool
	action        string
	editor        string
	width         int
	height        int
	styles        styles
	version       string
	pulling       bool
	pullStatus    string
	termStatus    string
	// right-pane file explorer state
	rightPaneMode int    // modeGit or modeFiles
	filesDir      string // current dir being browsed in files view
	filesRoot     string // selected project's dir; files view may not navigate above this
	fileEntries   []preview.FileEntry
	fileCursor    int
	fileNavStack  []string // dir history for going back in files view
	showHelp      bool     // whether the full-keybindings help popup is shown
}

// New creates a new Model bound to the given tty renderer. If cwd is a
// directory under root, the switcher opens with the nav stack descended
// down to and the cursor on the active subfolder matching cwd. editor is
// the configured file editor command used by the "open in editor" shortcut.
func New(root string, projects []project.Project, store *state.Store, renderer *lipgloss.Renderer, version string, cwd string, editor string) Model {
	ti := textinput.New()
	ti.Placeholder = "filter projects..."
	ti.Focus()
	ti.CharLimit = 100

	m := Model{
		root:         root,
		currentDir:   root,
		all:          projects,
		recent:       store.Recent,
		store:        store,
		filterInput:  ti,
		previewCache: make(map[string]preview.Preview),
		width:        80,
		height:       24,
		styles:       newStyles(renderer),
		version:      version,
		editor:       editor,
	}
	m.filtered = m.sortedProjects("")
	m = m.descendToCwd(cwd)
	return m
}

// descendToCwd walks the nav stack down into the directory the user is
// currently in (if it's under root), so the switcher opens with the cursor
// on the active subfolder instead of always starting at the root level.
func (m Model) descendToCwd(cwd string) Model {
	if cwd == "" {
		return m
	}
	rel, err := filepath.Rel(m.root, cwd)
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return m
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := -1
		for i, p := range m.filtered {
			if p.Name == part {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		m.cursor = idx
		p := m.filtered[idx]
		if p.IsGit {
			break
		}
		children, err := project.Scan(p.Path)
		if err != nil || len(children) == 0 {
			break
		}
		m.stack = append(m.stack, navFrame{
			dir:         m.currentDir,
			items:       m.all,
			cursor:      m.cursor,
			filterValue: m.filterInput.Value(),
		})
		m.currentDir = p.Path
		m.all = children
		m.filterInput.SetValue("")
		m.filtered = m.sortedProjects("")
		m.cursor = 0
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.loadPreviewCmd())
}

// sortedProjects returns projects sorted by filter text or recency.
func (m Model) sortedProjects(filter string) []project.Project {
	var source []project.Project
	if m.favoritesOnly {
		source = m.favoritesSorted()
	} else {
		source = m.all
	}

	var base []project.Project
	if filter == "" {
		// Sort alphabetically by name
		projects := make([]project.Project, len(source))
		copy(projects, source)
		sort.Slice(projects, func(i, j int) bool {
			return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
		})
		base = projects
	} else {
		// Fuzzy match
		results := fuzzy.FindFrom(filter, projectSource{source})
		out := make([]project.Project, len(results))
		for i, r := range results {
			out[i] = source[r.Index]
		}
		base = out
	}

	if m.favoritesOnly || m.store == nil {
		return base
	}

	// Pin favorites to the top, alphabetically among themselves, followed
	// by the rest in their existing order.
	var favs, rest []project.Project
	for _, p := range base {
		if m.store.IsFavorite(p.Path) {
			favs = append(favs, p)
		} else {
			rest = append(rest, p)
		}
	}
	sort.Slice(favs, func(i, j int) bool {
		return strings.ToLower(favs[i].Name) < strings.ToLower(favs[j].Name)
	})
	return append(favs, rest...)
}

// fallbackProject constructs a minimal project.Project for a favorited path
// that isn't present in m.all (e.g. favorited under a different root or a
// different drill-down subdirectory than the one currently being browsed).
// IsGit is left false since we have no cheap way to know without a stat;
// preview.Build and the list/preview rendering both handle IsGit==false
// (and any missing git data) gracefully already (see "not a git repo" path
// in renderPreviewContent).
func fallbackProject(path string) project.Project {
	return project.Project{
		Name: filepath.Base(path),
		Path: path,
	}
}

// favoritesSorted returns all favorited projects (sourced from the store,
// independent of the current navigation level/root), sorted alphabetically
// — the same order used to pin favorites to the top of the list, and used
// for the 1-9 highlight shortcut (favorites-only view, empty filter) and
// the favorites-only view itself.
func (m Model) favoritesSorted() []project.Project {
	if m.store == nil {
		return nil
	}
	// Build a lookup of currently-scanned projects so we reuse full data
	// (e.g. IsGit) when the favorite happens to be at the current level.
	byPath := make(map[string]project.Project, len(m.all))
	for _, p := range m.all {
		byPath[p.Path] = p
	}
	favs := make([]project.Project, 0, len(m.store.Favorites))
	for path := range m.store.Favorites {
		if p, ok := byPath[path]; ok {
			favs = append(favs, p)
		} else {
			favs = append(favs, fallbackProject(path))
		}
	}
	sort.Slice(favs, func(i, j int) bool {
		return strings.ToLower(favs[i].Name) < strings.ToLower(favs[j].Name)
	})
	return favs
}

// ascend pops the nav stack and restores the parent level.
func (m Model) ascend() Model {
	frame := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.currentDir = frame.dir
	m.all = frame.items
	m.filterInput.SetValue(frame.filterValue)
	m.filtered = m.sortedProjects(frame.filterValue)
	m.cursor = frame.cursor
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	return m
}

// loadPreviewCmd returns a Cmd to load preview for the current cursor item.
func (m Model) loadPreviewCmd() tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	p := m.filtered[m.cursor]
	if _, ok := m.previewCache[p.Path]; ok {
		return nil // already cached
	}
	return func() tea.Msg {
		data := preview.Build(p)
		return previewLoadedMsg{path: p.Path, data: data}
	}
}

// forceLoadPreviewCmd reloads preview for the current cursor item,
// bypassing the cache (used after a git pull to refresh status/log).
func (m Model) forceLoadPreviewCmd() tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	p := m.filtered[m.cursor]
	return func() tea.Msg {
		data := preview.Build(p)
		return previewLoadedMsg{path: p.Path, data: data}
	}
}

// pullCmd runs `git pull` on the given project path asynchronously.
func pullCmd(path string) tea.Cmd {
	return func() tea.Msg {
		out, err := gitinfo.Pull(path)
		return pullResultMsg{path: path, output: out, err: err}
	}
}

// termTabCmd opens a new terminal tab at path asynchronously.
func termTabCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := term.OpenNewTab(path)
		return termTabResultMsg{err: err}
	}
}

// explorerCmd opens Windows Explorer at path asynchronously.
func explorerCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := term.OpenExplorer(path)
		return explorerResultMsg{err: err}
	}
}

// editorCmd launches the configured editor at path asynchronously, without
// exiting pw. The editor runs with its working directory set to path and
// "." as its argument, so editors like `code`/`code .` open a new window
// there instead of blocking the current process.
func editorCmd(editor, path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(editor, ".")
		cmd.Dir = path
		err := cmd.Start()
		return editorResultMsg{err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listW, previewW := m.paneSizes()
		m.previewVP.Width = previewW - 2  // account for border
		m.previewVP.Height = m.height - 4 // minus header + filter + help
		_ = listW
		m.previewVP.SetContent(m.renderPreviewContent())
		return m, nil

	case previewLoadedMsg:
		m.previewCache[msg.path] = msg.data
		if len(m.filtered) > 0 && m.filtered[m.cursor].Path == msg.path {
			m.previewVP.SetContent(m.renderPreviewContent())
		}
		return m, nil

	case pullResultMsg:
		m.pulling = false
		if msg.err != nil {
			m.pullStatus = "pull failed: " + firstLine(msg.output, msg.err)
		} else {
			m.pullStatus = "pull ok: " + firstLine(msg.output, nil)
		}
		delete(m.previewCache, msg.path)
		var cmd tea.Cmd
		if len(m.filtered) > 0 && m.filtered[m.cursor].Path == msg.path {
			cmd = m.forceLoadPreviewCmd()
		}
		return m, cmd

	case termTabResultMsg:
		if msg.err != nil {
			m.termStatus = "new tab failed: " + msg.err.Error()
		} else {
			m.termStatus = "opened new terminal tab"
		}
		return m, nil

	case explorerResultMsg:
		if msg.err != nil {
			m.termStatus = "explorer failed: " + msg.err.Error()
		} else {
			m.termStatus = "opened in Explorer"
		}
		return m, nil

	case editorResultMsg:
		if msg.err != nil {
			m.termStatus = "editor failed: " + msg.err.Error()
		} else {
			m.termStatus = "opened in editor"
		}
		return m, nil

	case tea.KeyMsg:
		if m.showHelp {
			// Any key closes the help popup.
			m.showHelp = false
			return m, nil
		}
		if msg.String() == "?" {
			m.showHelp = true
			return m, nil
		}
		if s := msg.String(); m.favoritesOnly && m.filterInput.Value() == "" && len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			n := int(s[0] - '0')
			favs := m.favoritesSorted()
			if n <= len(favs) {
				target := favs[n-1]
				for i, fp := range m.filtered {
					if fp.Path == target.Path {
						m.cursor = i
						m.previewVP.SetContent(m.renderPreviewContent())
						return m, m.loadPreviewCmd()
					}
				}
			}
			return m, nil
		}
		switch msg.Type {
		case tea.KeyCtrlC:
			m.confirmed = false
			return m, tea.Quit

		case tea.KeyEsc:
			if len(m.stack) > 0 {
				m = m.ascend()
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			m.confirmed = false
			return m, tea.Quit

		case tea.KeyRight:
			if m.rightPaneMode == modeFiles {
				// descend into directory
				if len(m.fileEntries) > 0 && m.fileCursor < len(m.fileEntries) {
					fe := m.fileEntries[m.fileCursor]
					if fe.IsDir {
						m.fileNavStack = append(m.fileNavStack, m.filesDir)
						m.filesDir = filepath.Join(m.filesDir, fe.Name)
						m = m.reloadFileEntries()
						m.fileCursor = 0
						m.previewVP.SetContent(m.renderPreviewContent())
						m.scrollFilesCursorIntoView()
					}
				}
				return m, nil
			}
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				if !p.IsGit {
					children, err := project.Scan(p.Path)
					if err == nil && len(children) > 0 {
						m.stack = append(m.stack, navFrame{
							dir:         m.currentDir,
							items:       m.all,
							cursor:      m.cursor,
							filterValue: m.filterInput.Value(),
						})
						m.currentDir = p.Path
						m.all = children
						m.filterInput.SetValue("")
						m.filtered = m.sortedProjects("")
						m.cursor = 0
						m.previewVP.SetContent(m.renderPreviewContent())
						return m, m.loadPreviewCmd()
					}
				}
			}
			return m, nil

		case tea.KeyLeft:
			if m.rightPaneMode == modeFiles {
				// go up in file explorer, but never above the selected project's root
				if m.filesDir == m.filesRoot {
					return m, nil
				}
				childName := filepath.Base(m.filesDir)
				if len(m.fileNavStack) > 0 {
					m.filesDir = m.fileNavStack[len(m.fileNavStack)-1]
					m.fileNavStack = m.fileNavStack[:len(m.fileNavStack)-1]
				} else {
					m.filesDir = filepath.Dir(m.filesDir)
				}
				m = m.reloadFileEntries()
				m.fileCursor = 0
				for i, fe := range m.fileEntries {
					if fe.Name == childName {
						m.fileCursor = i
						break
					}
				}
				m.previewVP.SetContent(m.renderPreviewContent())
				m.scrollFilesCursorIntoView()
				return m, nil
			}
			if len(m.stack) > 0 {
				m = m.ascend()
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			return m, nil

		case tea.KeyEnter:
			if m.rightPaneMode == modeFiles {
				// enter directory
				if len(m.fileEntries) > 0 && m.fileCursor < len(m.fileEntries) {
					fe := m.fileEntries[m.fileCursor]
					if fe.IsDir {
						m.fileNavStack = append(m.fileNavStack, m.filesDir)
						m.filesDir = filepath.Join(m.filesDir, fe.Name)
						m = m.reloadFileEntries()
						m.fileCursor = 0
						m.previewVP.SetContent(m.renderPreviewContent())
						m.scrollFilesCursorIntoView()
					}
				}
				return m, nil
			}
			if len(m.filtered) > 0 {
				m.confirmed = true
				m.selectedPath = m.filtered[m.cursor].Path
			}
			return m, tea.Quit

		case tea.KeyCtrlO:
			if len(m.filtered) > 0 {
				m.confirmed = true
				m.selectedPath = m.filtered[m.cursor].Path
				m.action = "opencode"
			}
			return m, tea.Quit

		case tea.KeyCtrlE:
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.termStatus = ""
				return m, editorCmd(m.editor, p.Path)
			}
			return m, nil

		case tea.KeyCtrlF:
			if len(m.filtered) > 0 && m.store != nil {
				p := m.filtered[m.cursor]
				path := p.Path
				m.store.ToggleFavorite(path)
				_ = m.store.Save()
				newFilter := m.filterInput.Value()
				m.filtered = m.sortedProjects(newFilter)
				// keep cursor on the same project if still present
				for i, fp := range m.filtered {
					if fp.Path == path {
						m.cursor = i
						break
					}
				}
				if m.cursor >= len(m.filtered) {
					m.cursor = len(m.filtered) - 1
				}
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.previewVP.SetContent(m.renderPreviewContent())
			}
			return m, nil

		case tea.KeyCtrlG:
			m.favoritesOnly = !m.favoritesOnly
			newFilter := m.filterInput.Value()
			m.filtered = m.sortedProjects(newFilter)
			m.cursor = 0
			m.previewVP.SetContent(m.renderPreviewContent())
			return m, m.loadPreviewCmd()

		case tea.KeyUp, tea.KeyCtrlP:
			if m.rightPaneMode == modeFiles {
				if m.fileCursor > 0 {
					m.fileCursor--
					m.previewVP.SetContent(m.renderPreviewContent())
					m.scrollFilesCursorIntoView()
				}
				return m, nil
			}
			if m.cursor > 0 {
				m.cursor--
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			return m, nil

		case tea.KeyDown, tea.KeyCtrlN:
			if m.rightPaneMode == modeFiles {
				if m.fileCursor < len(m.fileEntries)-1 {
					m.fileCursor++
					m.previewVP.SetContent(m.renderPreviewContent())
					m.scrollFilesCursorIntoView()
				}
				return m, nil
			}
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			return m, nil

		case tea.KeyCtrlR:
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				if p.IsGit && !m.pulling {
					m.pulling = true
					m.pullStatus = ""
					return m, pullCmd(p.Path)
				}
			}
			return m, nil

		case tea.KeyCtrlT:
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.termStatus = ""
				return m, termTabCmd(p.Path)
			}
			return m, nil

		case tea.KeyCtrlX:
			if len(m.filtered) > 0 {
				p := m.filtered[m.cursor]
				m.termStatus = ""
				return m, explorerCmd(p.Path)
			}
			return m, nil

		case tea.KeyCtrlU:
			// Clear filter
			m.filterInput.SetValue("")
			m.filtered = m.sortedProjects("")
			m.cursor = 0
			m.pullStatus = ""
			m.termStatus = ""
			m.previewVP.SetContent(m.renderPreviewContent())
			return m, m.loadPreviewCmd()

		case tea.KeyCtrlD, tea.KeyPgDown:
			m.previewVP.HalfViewDown()
			return m, nil

		case tea.KeyCtrlB, tea.KeyPgUp:
			m.previewVP.HalfViewUp()
			return m, nil

		case tea.KeyTab:
			if m.rightPaneMode == modeGit {
				m.rightPaneMode = modeFiles
				m = m.initFilesView()
			} else {
				m.rightPaneMode = modeGit
			}
			m.previewVP.SetContent(m.renderPreviewContent())
			if m.rightPaneMode == modeFiles {
				m.scrollFilesCursorIntoView()
			}
			return m, nil

		default:
			// All other keys go to filter input
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			newFilter := m.filterInput.Value()
			m.filtered = m.sortedProjects(newFilter)
			m.cursor = 0
			m.previewVP.SetContent(m.renderPreviewContent())
			return m, tea.Batch(cmd, m.loadPreviewCmd())
		}
	}

	return m, nil
}

func (m Model) paneSizes() (listW, previewW int) {
	listW = int(math.Round(float64(m.width) * 0.4))
	if listW < 20 {
		listW = 20
	}
	previewW = m.width - listW
	if previewW < 10 {
		previewW = 10
	}
	return
}

// truncate truncates s to maxW runes, appending "…" if truncated.
func truncate(s string, maxW int) string {
	runes := []rune(s)
	if len(runes) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	return string(runes[:maxW-1]) + "…"
}

// firstLine returns the first non-empty line of s, or err's message if s is empty.
func firstLine(s string, err error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		if err != nil {
			return err.Error()
		}
		return "(no output)"
	}
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[0])
}

// humanizeAge converts a unix timestamp to a relative string.
func humanizeAge(ts int64) string {
	if ts == 0 {
		return ""
	}
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
}

// breadcrumb returns a display string like "/home/user/work › rms › sync".
func (m Model) breadcrumb() string {
	parts := []string{m.root}
	for _, f := range m.stack {
		if f.dir != m.root {
			parts = append(parts, filepath.Base(f.dir))
		}
	}
	if m.currentDir != m.root {
		parts = append(parts, filepath.Base(m.currentDir))
	}
	return strings.Join(parts, " › ")
}

func (m Model) renderListPane(width int) string {
	var sb strings.Builder

	header := truncate("  "+m.breadcrumb(), width)
	if m.favoritesOnly {
		header = truncate("  "+m.breadcrumb()+" [favorites]", width)
	}
	sb.WriteString(m.styles.header.Render(header))
	sb.WriteString("\n")

	// Filter input
	sb.WriteString(m.filterInput.View())
	sb.WriteString("\n")

	availH := m.height - 4 // header + filter + help + border-fudge
	if availH < 1 {
		availH = 1
	}

	// Compute visible window
	start := 0
	end := len(m.filtered)
	if end > availH {
		// Center cursor
		half := availH / 2
		start = m.cursor - half
		if start < 0 {
			start = 0
		}
		end = start + availH
		if end > len(m.filtered) {
			end = len(m.filtered)
			start = end - availH
			if start < 0 {
				start = 0
			}
		}
	}

	// Map favorite path -> quick-highlight digit (1-9), matching the same
	// favoritesSorted() order used by the Update handler's digit-highlight
	// shortcut, so the number shown here is exactly what the user can press.
	favNum := map[string]int{}
	for i, p := range m.favoritesSorted() {
		if i >= 9 {
			break
		}
		favNum[p.Path] = i + 1
	}

	for i := start; i < end; i++ {
		p := m.filtered[i]
		ts := m.recent[p.Path]
		tag := humanizeAge(ts)
		isFav := m.store != nil && m.store.IsFavorite(p.Path)

		// favMarker is the compact favorite indicator prefixed to the name:
		// "★ " normally, or "N★ " when this favorite has a quick-highlight
		// digit (1-9) assigned, so the row shows the same number the "1-9"
		// shortcut expects.
		favMarker := ""
		if isFav {
			if n, ok := favNum[p.Path]; ok {
				favMarker = fmt.Sprintf("%d★ ", n)
			} else {
				favMarker = "★ "
			}
		}

		nameMaxW := width - 4
		if tag != "" {
			nameMaxW -= len(tag) + 2
		}
		if isFav {
			nameMaxW -= len([]rune(favMarker))
		}
		if nameMaxW < 4 {
			nameMaxW = 4
		}

		name := truncate(p.Name, nameMaxW)
		if isFav {
			name = favMarker + name
		}

		if i == m.cursor {
			// Cursor row: build plain row (no nested Render calls) so the single
			// outer cursor.Render() covers the entire row uniformly — a nested
			// styled tag's reset sequence would terminate the reverse-video early.
			var row string
			if tag != "" {
				padding := width - 2 - len([]rune(name)) - len(tag) - 2
				if padding < 1 {
					padding = 1
				}
				row = " " + name + strings.Repeat(" ", padding) + tag + " "
			} else {
				row = " " + name + " "
			}
			sb.WriteString(m.styles.cursor.Render(row))
		} else if tag != "" {
			// Non-cursor row with recency tag: color the tag, then style the whole row.
			padding := width - 2 - len([]rune(name)) - len(tag) - 2
			if padding < 1 {
				padding = 1
			}
			var namePart string
			if isFav {
				namePart = m.styles.favoriteTag.Render(favMarker) + strings.TrimPrefix(name, favMarker)
			} else {
				namePart = name
			}
			row := " " + namePart + strings.Repeat(" ", padding) + m.styles.recentTag.Render(tag) + " "
			if p.IsGit {
				sb.WriteString(m.styles.gitRepo.Render(row))
			} else {
				sb.WriteString(m.styles.normal.Render(row))
			}
		} else {
			var namePart string
			if isFav {
				namePart = m.styles.favoriteTag.Render(favMarker) + strings.TrimPrefix(name, favMarker)
			} else {
				namePart = name
			}
			row := " " + namePart + " "
			if p.IsGit {
				sb.WriteString(m.styles.gitRepo.Render(row))
			} else {
				sb.WriteString(m.styles.normal.Render(row))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// initFilesView sets up filesDir from current project and loads entries.
func (m Model) initFilesView() Model {
	if len(m.filtered) == 0 {
		return m
	}
	p := m.filtered[m.cursor]
	if m.filesDir == "" || m.filesRoot != p.Path {
		m.filesDir = p.Path
		m.filesRoot = p.Path
		m.fileNavStack = nil
		m.fileCursor = 0
	}
	return m.reloadFileEntries()
}

// reloadFileEntries loads FileEntry list for current filesDir, resolving
// git status dynamically via the enclosing repo root (works even when the
// selected project itself is a subfolder of a larger repo, e.g. monorepo
// packages where .git lives in a parent directory).
func (m Model) reloadFileEntries() Model {
	repoRoot, gitStatuses := gitinfo.StatusFor(m.filesDir)
	entries, err := preview.ListDirEntries(m.filesDir, gitStatuses, repoRoot)
	if err != nil {
		m.fileEntries = nil
	} else {
		m.fileEntries = entries
	}
	if m.fileCursor >= len(m.fileEntries) {
		m.fileCursor = 0
	}
	return m
}

// humanizeSize returns a human-readable file size string.
func humanizeSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}

// gitStatusStyle returns a color-coded style for a git porcelain XY status
// code so dirty files stand out in the file explorer: modified = yellow,
// untracked = cyan, added/staged = green, deleted = red, conflict = magenta.
func (m Model) gitStatusStyle(code string) lipgloss.Style {
	trimmed := strings.TrimSpace(code)
	r := m.styles.renderer
	switch {
	case trimmed == "??":
		return r.NewStyle().Bold(true).Foreground(lipgloss.Color("14")) // cyan: untracked
	case trimmed == "D" || strings.Contains(code, "D"):
		return r.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // red: deleted
	case trimmed == "U" || strings.Contains(code, "U"):
		return r.NewStyle().Bold(true).Foreground(lipgloss.Color("13")) // magenta: conflict
	case trimmed == "A" || strings.HasPrefix(code, "A"):
		return r.NewStyle().Bold(true).Foreground(lipgloss.Color("10")) // green: added/staged
	default:
		return r.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // yellow: modified/other
	}
}

// filesHeaderLines is the number of lines rendered before the file entry
// rows in renderFilesContent (mode tabs, blank, dir path, blank).
const filesHeaderLines = 4

// scrollFilesCursorIntoView adjusts previewVP's Y offset so the row at
// fileCursor is visible, in case it's above or below the current viewport.
func (m *Model) scrollFilesCursorIntoView() {
	target := filesHeaderLines + m.fileCursor
	if target < m.previewVP.YOffset {
		m.previewVP.SetYOffset(target)
	} else if target > m.previewVP.YOffset+m.previewVP.Height-1 {
		m.previewVP.SetYOffset(target - m.previewVP.Height + 1)
	}
}

// renderModeHeader renders the tab bar for the right pane.
func (m Model) renderModeHeader() string {
	activeStyle := m.styles.renderer.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	inactiveStyle := m.styles.renderer.NewStyle().Padding(0, 1)
	var gitTab, filesTab string
	if m.rightPaneMode == modeGit {
		gitTab = activeStyle.Render("Git")
		filesTab = inactiveStyle.Render("Files")
	} else {
		gitTab = inactiveStyle.Render("Git")
		filesTab = activeStyle.Render("Files")
	}
	return gitTab + " " + filesTab + m.styles.sep.Render("  (tab to switch)")
}

// renderFilesContent builds the file explorer right pane content.
func (m Model) renderFilesContent() string {
	var sb strings.Builder

	sb.WriteString(m.renderModeHeader())
	sb.WriteString("\n\n")

	if len(m.filtered) == 0 {
		sb.WriteString("(no project selected)")
		return sb.String()
	}

	sb.WriteString(m.styles.sep.Render(m.filesDir))
	sb.WriteString("\n\n")

	if len(m.fileEntries) == 0 {
		sb.WriteString(m.styles.sep.Render("(empty directory)"))
		return sb.String()
	}

	cursorStyle := m.styles.cursor
	for i, fe := range m.fileEntries {
		name := fe.Name
		if fe.IsDir {
			name += "/"
		}
		sizeStr := humanizeSize(fe.Size)
		if fe.IsDir {
			sizeStr = "     "
		}
		modStr := fe.ModTime.Format("2006-01-02 15:04")
		gitStr := fe.GitStatus
		if gitStr == "" {
			gitStr = "  "
		}

		row := fmt.Sprintf(" %-30s %6s  %s  %s", truncate(name, 30), sizeStr, modStr, gitStr)
		switch {
		case i == m.fileCursor:
			sb.WriteString(cursorStyle.Render(row))
		case fe.GitStatus != "":
			sb.WriteString(m.gitStatusStyle(fe.GitStatus).Render(row))
		default:
			sb.WriteString(row)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderPreviewContent() string {
	if m.rightPaneMode == modeFiles {
		return m.renderFilesContent()
	}
	if len(m.filtered) == 0 {
		return "(no projects)"
	}
	p := m.filtered[m.cursor]
	pv, ok := m.previewCache[p.Path]
	if !ok {
		return m.renderModeHeader() + "\n\nloading…"
	}

	var sb strings.Builder

	// Mode tab header
	sb.WriteString(m.renderModeHeader())
	sb.WriteString("\n\n")

	// Project name header
	sb.WriteString(m.styles.previewHead.Render("◆ " + p.Name))
	sb.WriteString("\n\n")

	// Git info line
	if !p.IsGit {
		sb.WriteString(m.styles.sep.Render("not a git repo"))
	} else if !pv.Git.Available {
		sb.WriteString(m.styles.sep.Render("git unavailable"))
	} else if pv.Git.Err != nil {
		sb.WriteString(m.styles.sep.Render("git error: " + pv.Git.Err.Error()))
	} else {
		dirtyStr := "clean"
		if pv.Git.Dirty {
			dirtyStr = "dirty"
		}
		sb.WriteString(m.styles.gitRepo.Render(pv.Git.Branch))
		if pv.Git.Behind > 0 {
			sb.WriteString(" " + m.styles.behindTag.Render(fmt.Sprintf("↓%d", pv.Git.Behind)))
		}
		if pv.Git.Ahead > 0 {
			sb.WriteString(" " + m.styles.aheadTag.Render(fmt.Sprintf("↑%d", pv.Git.Ahead)))
		}
		sb.WriteString(m.styles.gitRepo.Render(" · " + dirtyStr))
	}
	sb.WriteString("\n")

	if p.IsGit && pv.Git.Available && pv.Git.Err == nil {
		sb.WriteString("Last commit: " + pv.Git.LastCommit + "\n")
		if !pv.Git.LastSync.IsZero() {
			sb.WriteString("Last synced: " + humanizeAge(pv.Git.LastSync.Unix()) + "\n")
		} else {
			sb.WriteString("Last synced: never\n")
		}
		if pv.Git.Upstream != "" {
			switch {
			case pv.Git.Behind > 0 && pv.Git.Ahead > 0:
				sb.WriteString(m.styles.behindTag.Render(fmt.Sprintf("%d commits behind, %d ahead of %s", pv.Git.Behind, pv.Git.Ahead, pv.Git.Upstream)) + "\n")
			case pv.Git.Behind > 0:
				sb.WriteString(m.styles.behindTag.Render(fmt.Sprintf("%d commits behind %s", pv.Git.Behind, pv.Git.Upstream)) + "\n")
			case pv.Git.Ahead > 0:
				sb.WriteString(m.styles.gitRepo.Render(fmt.Sprintf("%d commits ahead of %s", pv.Git.Ahead, pv.Git.Upstream)) + "\n")
			default:
				sb.WriteString(m.styles.sep.Render("up to date with "+pv.Git.Upstream) + "\n")
			}
		}
	}

	sb.WriteString("\n")

	// README
	sb.WriteString(m.styles.sep.Render("── README ──"))
	sb.WriteString("\n")
	if pv.Readme == "" {
		sb.WriteString(m.styles.sep.Render("(no README)"))
	} else {
		sb.WriteString(pv.Readme)
	}
	sb.WriteString("\n\n")

	// Files
	sb.WriteString(m.styles.sep.Render("── Files ──"))
	sb.WriteString("\n")
	for _, f := range pv.Files {
		sb.WriteString("  " + f + "\n")
	}

	return sb.String()
}

func (m Model) View() string {
	listW, previewW := m.paneSizes()

	listContent := m.renderListPane(listW)

	previewContent := m.previewVP.View()

	// Help bar
	helpText := "↑↓ move · → open · ← back · ↵ switch · ^o opencode · ^e editor · ^t new tab · ^x explorer · ^r pull · ^f favorite · ^g favorites view · 1-9 highlight favorite (empty filter) · tab git/files · ? help · esc back/quit · ^c quit · ^u clear · ^d/^b scroll"
	if m.pulling {
		helpText = "pulling…"
	} else if m.pullStatus != "" {
		helpText = m.pullStatus
	} else if m.termStatus != "" {
		helpText = m.termStatus
	}
	verText := "v" + m.version
	avail := m.width - len([]rune(verText)) - 1
	if avail < 1 {
		avail = 1
	}
	helpText = truncate(helpText, avail)
	pad := m.width - len([]rune(helpText)) - len([]rune(verText)) - 1
	var help string
	if pad > 0 {
		help = m.styles.help.Render(helpText + strings.Repeat(" ", pad) + verText)
	} else {
		help = m.styles.help.Render(helpText)
	}

	// Build panes — use renderer-bound styles for layout too
	leftPane := m.styles.paneBorder.Width(listW).Render(listContent)
	rightPane := m.styles.renderer.NewStyle().Width(previewW - 2).Render(previewContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	full := body + "\n" + help

	if m.showHelp {
		return m.renderHelpOverlay(full)
	}
	return full
}

// renderHelpOverlay draws a bordered popup listing every keybinding,
// centered over the given background content.
func (m Model) renderHelpOverlay(background string) string {
	r := m.styles.renderer
	titleStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	keyStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	descStyle := r.NewStyle().Foreground(lipgloss.Color("7"))
	sectionStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))

	type kb struct{ key, desc string }
	row := func(key, desc string) kb { return kb{key, desc} }

	sections := []struct {
		title string
		keys  []kb
	}{
		{"Navigation", []kb{
			row("↑ / ↓, Ctrl+P/N", "Move cursor up/down"),
			row("→ (Right)", "Descend into folder / enter directory (files view)"),
			row("← (Left)", "Go back / go up a directory (files view)"),
			row("Enter", "Select project & cd, or enter directory (files view)"),
			row("Esc", "Go back one level, or cancel at root"),
			row("Ctrl+C", "Cancel immediately (any depth)"),
		}},
		{"Launch shortcuts", []kb{
			row("Ctrl+O", "Select project, cd, and launch opencode"),
			row("Ctrl+E", "Open project in configured editor (new window, pw stays open)"),
			row("Ctrl+T", "Open a new Windows Terminal tab at this path (same shell)"),
			row("Ctrl+X", "Open this path in Windows Explorer (Windows/WSL only)"),
		}},
		{"Favorites", []kb{
			row("Ctrl+F", "Toggle favorite on highlighted project"),
			row("Ctrl+G", "Toggle favorites-only view"),
			row("1 .. 9", "In favorites-only view with an empty filter: move cursor to the Nth favorite (then Enter/Ctrl+O/etc. to act on it)"),
		}},
		{"Right pane", []kb{
			row("Tab", "Toggle between Git view and Files view"),
			row("Ctrl+D / PgDn", "Scroll preview down"),
			row("Ctrl+B / PgUp", "Scroll preview up"),
			row("Ctrl+R", "git pull the highlighted repo"),
		}},
		{"Filter", []kb{
			row("Type anything", "Filter projects (fuzzy)"),
			row("Ctrl+U", "Clear filter"),
		}},
		{"Misc", []kb{
			row("?", "Toggle this help popup"),
		}},
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Keybindings"))
	sb.WriteString("\n\n")
	for i, sec := range sections {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(sectionStyle.Render(sec.title))
		sb.WriteString("\n")
		for _, k := range sec.keys {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render(fmt.Sprintf("%-22s", k.key)), descStyle.Render(k.desc)))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(m.styles.sep.Render("Press any key to close"))

	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Render(sb.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("0")))
}

// SelectedPath returns the selected path and confirmation status.
func (m Model) SelectedPath() (string, bool) {
	return m.selectedPath, m.confirmed
}

// SelectedAction returns the requested post-selection action, if any
// (e.g. "opencode" to launch the opencode CLI, "editor" to launch the
// configured file editor, both in the selected directory).
func (m Model) SelectedAction() string {
	return m.action
}

// SelectedEditor returns the configured editor command (used when
// SelectedAction() == "editor").
func (m Model) SelectedEditor() string {
	return m.editor
}
