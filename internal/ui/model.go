package ui

import (
	"bufio"
	"bytes"
	"errors"
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
	"pw/internal/version"
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
	updateTag   lipgloss.Style
	version     lipgloss.Style
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
		updateTag:   r.NewStyle().Bold(true).Foreground(lipgloss.Color("141")),
		version:     r.NewStyle().Foreground(lipgloss.Color("12")),
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

// updateAvailableMsg is sent when the async GitHub "latest release" check
// finds a newer version than the running build. latest is empty if no
// update is available (or the check failed/was disabled) and, in that
// case, the message can simply be ignored.
type updateAvailableMsg struct {
	latest string
}

// mdLiveResultMsg is sent when an async "launch md-to-pdf live preview"
// attempt finishes: either the server failed to start (err set), or it
// reported ready at url, optionally with a separate browserErr if the
// server came up fine but opening the browser failed.
type mdLiveResultMsg struct {
	url        string
	err        error
	browserErr error
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
	mdToPdfBin    string // "md-to-pdf" binary path/name; empty if not installed
	width         int
	height        int
	styles        styles
	version       string
	latestVersion string
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
	helpScroll    int      // scroll offset within the help popup (only used if content overflows)
	showUpdate    bool     // whether the "update available" popup is shown
	updateChecked bool     // whether the async update check has responded yet
	// new-directory prompt state (files pane, "N" shortcut)
	newDirModal bool
	newDirInput textinput.Model
	newDirError string
}

// New creates a new Model bound to the given tty renderer. If cwd is a
// directory under root, the switcher opens with the nav stack descended
// down to and the cursor on the active subfolder matching cwd. editor is
// the configured file editor command used by the "open in editor" shortcut.
// mdToPdfBin is the resolved "md-to-pdf" binary (empty if not installed),
// used by the files-view "open + live preview" shortcut for Markdown files.
func New(root string, projects []project.Project, store *state.Store, renderer *lipgloss.Renderer, version string, cwd string, editor string, mdToPdfBin string) Model {
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
		mdToPdfBin:   mdToPdfBin,
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
	return tea.Batch(textinput.Blink, m.loadPreviewCmd(), checkUpdateCmd())
}

// checkUpdateCmd asynchronously checks GitHub for a newer release. It never
// blocks startup and never errors out to the UI: failures simply result in
// an updateAvailableMsg with an empty latest field.
func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		return updateAvailableMsg{latest: version.CheckLatest()}
	}
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
			return projectLess(projects[i], projects[j])
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
		return projectLess(favs[i], favs[j])
	})
	return append(favs, rest...)
}

// projectLess is the canonical alphabetical ordering for projects. It breaks
// ties on the exact (case-sensitive) Path so ordering is deterministic even
// when two distinct projects share a lowercase Name. Without this tiebreaker
// sort.Slice leaves equal-name entries in their (map-iteration) input order,
// which shuffles favorites between renders/navigations and makes the list
// appear to "jump" / reorder.
func projectLess(a, b project.Project) bool {
	an, bn := strings.ToLower(a.Name), strings.ToLower(b.Name)
	if an != bn {
		return an < bn
	}
	return a.Path < b.Path
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
		return projectLess(favs[i], favs[j])
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

// editorFileCmd launches the configured editor on a single file
// asynchronously, without exiting pw (e.g. `code path/to/file.md`).
func editorFileCmd(editor, filePath string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(editor, filePath)
		err := cmd.Start()
		return editorResultMsg{err: err}
	}
}

// mdLiveCmd launches `md-to-pdf serve <filePath>` asynchronously, starting
// a live-preview server for the given Markdown file. It does not pass
// md-to-pdf's own --open flag: on some setups (e.g. WSL images without
// xdg-open/wslu) that flag fails to open a browser silently, with no error
// and no indication anything went wrong. Instead this scans the server's
// stdout for its "serve-ready ... url=<url>" line to get the real listen
// URL (respecting whatever port it actually bound), then opens that URL
// itself via term.OpenBrowser (WSL/Windows/xdg-open aware). The server
// process is left running detached; pw does not wait on or manage its
// lifecycle beyond this readiness check.
func mdLiveCmd(bin, filePath string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(bin, "serve", filePath)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return mdLiveResultMsg{err: err}
		}
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		if err := cmd.Start(); err != nil {
			return mdLiveResultMsg{err: err}
		}

		type ready struct {
			url string
			err error
		}
		readyCh := make(chan ready, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				idx := strings.Index(line, "url=")
				if idx == -1 {
					continue
				}
				url := strings.TrimSpace(line[idx+len("url="):])
				if sp := strings.IndexByte(url, ' '); sp != -1 {
					url = url[:sp]
				}
				readyCh <- ready{url: url}
				return
			}
			// stdout closed without ever printing a ready line: the
			// process exited (or crashed) before the server came up.
			_ = cmd.Wait()
			msg := strings.TrimSpace(stderrBuf.String())
			if msg == "" {
				msg = "md-to-pdf exited before starting the live server"
			}
			readyCh <- ready{err: errors.New(msg)}
		}()

		select {
		case r := <-readyCh:
			if r.err != nil {
				return mdLiveResultMsg{err: r.err}
			}
			return mdLiveResultMsg{url: r.url, browserErr: term.OpenBrowser(r.url)}
		case <-time.After(5 * time.Second):
			return mdLiveResultMsg{err: errors.New("md-to-pdf did not report ready within 5s")}
		}
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

	case updateAvailableMsg:
		m.latestVersion = msg.latest
		m.updateChecked = true
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

	case mdLiveResultMsg:
		switch {
		case msg.err != nil:
			m.termStatus = "md-to-pdf failed: " + msg.err.Error()
		case msg.browserErr != nil:
			m.termStatus = "md-to-pdf live at " + msg.url + " (couldn't open browser: " + msg.browserErr.Error() + ")"
		default:
			m.termStatus = "opened in editor + md-to-pdf live at " + msg.url
		}
		return m, nil

	case tea.KeyMsg:
		if m.newDirModal {
			switch msg.Type {
			case tea.KeyEsc:
				m.newDirModal = false
				m.newDirInput.SetValue("")
				m.newDirError = ""
				return m, nil
			case tea.KeyEnter:
				name := strings.TrimSpace(m.newDirInput.Value())
				if name == "" {
					m.newDirError = "name cannot be empty"
					return m, nil
				}
				if err := preview.CreateDir(m.filesDir, name); err != nil {
					m.newDirError = err.Error()
					return m, nil
				}
				m.newDirModal = false
				m.newDirInput.SetValue("")
				m.newDirError = ""
				m = m.reloadFileEntries()
				for i, fe := range m.fileEntries {
					if fe.Name == name {
						m.fileCursor = i
						break
					}
				}
				m.previewVP.SetContent(m.renderPreviewContent())
				m.scrollFilesCursorIntoView()
				return m, nil
			default:
				var cmd tea.Cmd
				m.newDirInput, cmd = m.newDirInput.Update(msg)
				return m, cmd
			}
		}
		if m.showUpdate {
			// Any key closes the update popup (including F2 again).
			m.showUpdate = false
			return m, nil
		}
		if m.showHelp {
			if msg.Type == tea.KeyUp || msg.Type == tea.KeyDown {
				lines, availH := m.helpOverlayLayout()
				if len(lines) > availH {
					maxOffset := len(lines) - availH
					if msg.Type == tea.KeyUp {
						if m.helpScroll > 0 {
							m.helpScroll--
						}
					} else if m.helpScroll < maxOffset {
						m.helpScroll++
					}
					return m, nil
				}
			}
			// Any other key closes the help popup.
			m.showHelp = false
			m.helpScroll = 0
			return m, nil
		}
		if msg.String() == "?" {
			m.showHelp = true
			m.helpScroll = 0
			return m, nil
		}
		// F2 always opens the version/update popup (before the filter
		// textinput gets a chance to see the message — textinput doesn't
		// bind F-keys, but keep this ahead of it regardless so nothing can
		// ever swallow it, unlike Ctrl-key bindings which textinput can
		// intercept for its own line-editing shortcuts).
		if msg.Type == tea.KeyF2 {
			m.showUpdate = true
			return m, nil
		}
		if msg.Type == tea.KeyCtrlK && m.rightPaneMode == modeFiles {
			ti := textinput.New()
			ti.Placeholder = "new directory name..."
			ti.Focus()
			ti.CharLimit = 200
			m.newDirInput = ti
			m.newDirError = ""
			m.newDirModal = true
			return m, textinput.Blink
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

		case tea.KeyCtrlL:
			if m.rightPaneMode == modeFiles && m.mdToPdfBin != "" &&
				len(m.fileEntries) > 0 && m.fileCursor < len(m.fileEntries) {
				fe := m.fileEntries[m.fileCursor]
				if !fe.IsDir && strings.HasSuffix(strings.ToLower(fe.Name), ".md") {
					filePath := filepath.Join(m.filesDir, fe.Name)
					m.termStatus = ""
					return m, tea.Batch(
						editorFileCmd(m.editor, filePath),
						mdLiveCmd(m.mdToPdfBin, filePath),
					)
				}
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

// buildHelpBar joins segs (already ordered by priority, most useful first)
// with " · ", including only as many whole segments as fit within maxW.
// Segments are never cut mid-word — if any had to be dropped, a trailing
// "…" marks the truncation. maxW <= 0 (size unknown yet) returns everything.
func buildHelpBar(segs []string, maxW int) string {
	const sep = " · "
	full := strings.Join(segs, sep)
	if maxW <= 0 || len([]rune(full)) <= maxW {
		return full
	}
	avail := maxW - 1 // reserve room for the trailing "…"
	var used []string
	curLen := 0
	for _, s := range segs {
		add := len([]rune(s))
		if len(used) > 0 {
			add += len(sep)
		}
		if curLen+add > avail {
			break
		}
		used = append(used, s)
		curLen += add
	}
	if len(used) == 0 {
		return truncate(segs[0], maxW)
	}
	return strings.Join(used, sep) + "…"
}

// wrapWords greedily word-wraps s into lines no wider than width runes,
// breaking only at spaces (never mid-word). Individual words longer than
// width are hard-truncated as a last resort. Always returns at least one
// line (possibly empty).
func wrapWords(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	lines := make([]string, 0, 2)
	cur := words[0]
	for _, w := range words[1:] {
		if len([]rune(cur))+1+len([]rune(w)) <= width {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	lines = append(lines, cur)
	for i, l := range lines {
		if len([]rune(l)) > width {
			lines[i] = truncate(l, width)
		}
	}
	return lines
}

// overlayContentWidth returns the usable text width for a bordered overlay
// box styled with Padding(1, 2) — i.e. what to pass to Style.Width — sized
// to comfortably fit inside the terminal. Falls back to desired when
// m.width is 0/unknown (e.g. before the first WindowSizeMsg).
func (m Model) overlayContentWidth(desired int) int {
	const overhead = 6 // rounded border (1 each side) + padding (2 each side)
	if m.width > 0 {
		maxTotal := m.width - 2 // small margin so the box never touches the frame edge
		if maxTotal < overhead+10 {
			maxTotal = overhead + 10
		}
		if maxContent := maxTotal - overhead; maxContent < desired {
			desired = maxContent
		}
	}
	if desired < 20 {
		desired = 20
	}
	return desired
}

// overlayMaxContentHeight returns how many lines of box content (inside the
// border and Padding(1, 2)) an overlay can use without exceeding the
// terminal height. Falls back to fallback when m.height is 0/unknown.
func (m Model) overlayMaxContentHeight(fallback int) int {
	const overhead = 4 // border (top+bottom) + padding (top+bottom, 1 each)
	if m.height > 0 {
		h := m.height - overhead - 2 // small margin so it never touches the frame edge
		if h < 3 {
			h = 3
		}
		return h
	}
	return fallback
}

// clampOverlayHeight trims lines to the terminal's available overlay
// height for overlays too simple to warrant scrolling (new-dir / update
// popups). Drops from the end and marks the drop with a trailing "…" line.
func (m Model) clampOverlayHeight(lines []string) []string {
	maxH := m.overlayMaxContentHeight(len(lines))
	if len(lines) <= maxH {
		return lines
	}
	if maxH < 1 {
		maxH = 1
	}
	out := append([]string{}, lines[:maxH-1]...)
	out = append(out, "…")
	return out
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

// renderModeHeader renders the tab bar for the right pane, with the app
// version right-aligned to the pane's available content width. The version
// is dropped (never wrapped/truncated over the tabs) if there isn't enough
// room to show it alongside the tabs.
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
	tabs := gitTab + " " + filesTab + m.styles.sep.Render("  (tab to switch)")

	_, previewW := m.paneSizes()
	contentW := previewW - 2 // matches the Width(previewW-2) the right pane is rendered at
	if contentW <= 0 {
		return tabs
	}

	verText := "v" + m.version
	verRendered := m.styles.version.Render(truncate(verText, contentW))
	if m.latestVersion != "" {
		full := verText + " → v" + m.latestVersion
		if len([]rune(full)) <= contentW {
			verRendered = m.styles.version.Render(verText+" → ") + m.styles.updateTag.Render("v"+m.latestVersion)
		}
	}

	gap := contentW - lipgloss.Width(tabs) - lipgloss.Width(verRendered)
	if gap < 1 {
		// Not enough room to show the version without crowding/wrapping the
		// tabs — drop it rather than risk breaking the layout.
		return tabs
	}
	return tabs + strings.Repeat(" ", gap) + verRendered
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

	_, previewW := m.paneSizes()
	nameW := previewW - 2 - 30 // subtract pane padding + fixed columns (size, mod time, git status)
	if nameW < 10 {
		nameW = 10
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

		row := fmt.Sprintf(" %-*s %6s  %s  %s", nameW, truncate(name, nameW), sizeStr, modStr, gitStr)
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

	// Help bar — ordered by priority (most useful first). Segments are
	// appended only while they still fit within m.width; whole segments are
	// dropped (never cut mid-word) and a trailing "…" marks any drop.
	segs := []string{"? help"}
	if m.latestVersion != "" {
		segs = append(segs, "F2 update available")
	}
	segs = append(segs,
		"↑↓ move", "→ open", "← back", "↵ switch",
		"esc back/quit", "^c quit",
		"tab git/files",
		"^o opencode", "^e editor",
		"^f favorite", "^g favorites",
		"^r pull",
		"1-9 fav (empty filter)",
		"^k new dir (files)",
		"^u clear filter",
		"^d/^b scroll",
		"^t new tab", "^x explorer",
	)
	if m.mdToPdfBin != "" {
		segs = append(segs, "^l md preview")
	}
	if m.latestVersion == "" {
		// Low priority when there's nothing to act on — always shown, but
		// the first thing to get dropped on a narrow terminal.
		segs = append(segs, "F2 version")
	}
	helpText := buildHelpBar(segs, m.width)
	if m.pulling {
		helpText = "pulling…"
	} else if m.pullStatus != "" {
		helpText = m.pullStatus
	} else if m.termStatus != "" {
		helpText = m.termStatus
	}
	if m.width > 0 {
		helpText = truncate(helpText, m.width)
	}
	help := m.styles.help.Render(helpText)

	// Build panes — use renderer-bound styles for layout too
	leftPane := m.styles.paneBorder.Width(listW).Render(listContent)
	rightPane := m.styles.renderer.NewStyle().Width(previewW - 2).Render(previewContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	full := body + "\n" + help

	if m.newDirModal {
		return m.renderNewDirOverlay(full)
	}
	if m.showHelp {
		return m.renderHelpOverlay(full)
	}
	if m.showUpdate {
		return m.renderUpdateOverlay(full)
	}
	return full
}

// renderNewDirOverlay draws a small bordered prompt for entering a new
// directory name, centered over the given background content.
func (m Model) renderNewDirOverlay(background string) string {
	r := m.styles.renderer
	titleStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	errStyle := r.NewStyle().Foreground(lipgloss.Color("9"))

	contentW := m.overlayContentWidth(60)

	lines := []string{
		titleStyle.Render("New directory in " + truncate(m.filesDir, contentW-len("New directory in "))),
		"",
		m.newDirInput.View(),
	}
	if m.newDirError != "" {
		lines = append(lines, "", errStyle.Render(m.newDirError))
	}
	lines = append(lines, "", m.styles.sep.Render("Enter: create · Esc: cancel"))
	lines = m.clampOverlayHeight(lines)

	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(contentW).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("0")))
}

// helpOverlayLayout builds the (already-styled) content lines for the help
// overlay — sections, key column, and word-wrapped descriptions sized to
// the box's actual width — and reports how many lines fit at once given the
// current terminal height. If descriptions wrap, continuation lines are
// indented under the description column rather than the key column. If the
// full content (with blank separators between sections) doesn't fit, blank
// separators are dropped first before the caller falls back to scrolling.
func (m Model) helpOverlayLayout() (lines []string, availContentH int) {
	r := m.styles.renderer
	keyStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	descStyle := r.NewStyle().Foreground(lipgloss.Color("7"))
	sectionStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))

	type kb struct{ key, desc string }
	row := func(key, desc string) kb { return kb{key, desc} }

	rightPaneRows := []kb{
		row("Tab", "Toggle Git/Files view"),
		row("Ctrl+D / PgDn", "Scroll preview down"),
		row("Ctrl+B / PgUp", "Scroll preview up"),
		row("Ctrl+R", "git pull the highlighted repo"),
		row("Ctrl+K", "Files: create a new directory"),
	}
	if m.mdToPdfBin != "" {
		rightPaneRows = append(rightPaneRows, row("Ctrl+L", "Files: open .md + start live PDF preview"))
	}

	sections := []struct {
		title string
		keys  []kb
	}{
		{"Navigation", []kb{
			row("↑ / ↓, Ctrl+P/N", "Move cursor up/down"),
			row("→ (Right)", "Descend into folder / directory (files)"),
			row("← (Left)", "Go back / up a directory (files)"),
			row("Enter", "Select project & cd, or open directory"),
			row("Esc", "Go back a level, or cancel at root"),
			row("Ctrl+C", "Cancel immediately (any depth)"),
		}},
		{"Launch shortcuts", []kb{
			row("Ctrl+O", "Select project, cd, launch opencode"),
			row("Ctrl+E", "Open project in configured editor"),
			row("Ctrl+T", "New Windows Terminal tab (same shell)"),
			row("Ctrl+X", "Open path in Explorer (Windows/WSL)"),
		}},
		{"Favorites", []kb{
			row("Ctrl+F", "Toggle favorite on highlighted project"),
			row("Ctrl+G", "Toggle favorites-only view"),
			row("1 .. 9", "Jump to Nth favorite (favorites view, empty filter)"),
		}},
		{"Right pane", rightPaneRows},
		{"Filter", []kb{
			row("Type anything", "Filter projects (fuzzy)"),
			row("Ctrl+U", "Clear filter"),
		}},
		{"Misc", []kb{
			row("?", "Toggle this help popup"),
			row("F2", "Version / update details"),
		}},
	}

	// Key column width: longest key label, capped so descriptions keep
	// reasonable room even in wide terminals.
	keyW := 4
	for _, sec := range sections {
		for _, k := range sec.keys {
			if w := len([]rune(k.key)); w > keyW {
				keyW = w
			}
		}
	}
	if keyW > 20 {
		keyW = 20
	}

	const rowIndent = 2
	const keyDescGap = 2
	contentW := m.overlayContentWidth(78)
	descW := contentW - rowIndent - keyW - keyDescGap
	if descW < 12 {
		descW = 12
	}

	build := func(withBlanks bool) []string {
		var out []string
		indent := strings.Repeat(" ", rowIndent+keyW+keyDescGap)
		for i, sec := range sections {
			if i > 0 && withBlanks {
				out = append(out, "")
			}
			out = append(out, sectionStyle.Render(sec.title))
			for _, k := range sec.keys {
				keyCol := keyStyle.Render(fmt.Sprintf("%-*s", keyW, k.key))
				for j, dl := range wrapWords(k.desc, descW) {
					if j == 0 {
						out = append(out, strings.Repeat(" ", rowIndent)+keyCol+strings.Repeat(" ", keyDescGap)+descStyle.Render(dl))
					} else {
						out = append(out, indent+descStyle.Render(dl))
					}
				}
			}
		}
		return out
	}

	const chromeLines = 4 // title, blank, blank-before-footer, footer
	availH := m.overlayMaxContentHeight(200) - chromeLines
	if availH < 1 {
		availH = 1
	}

	lines = build(true)
	if len(lines) > availH {
		lines = build(false)
	}
	return lines, availH
}

// renderHelpOverlay draws a bordered popup listing every keybinding,
// centered over the given background content. Sized to the terminal; if
// the content is still too tall after compacting, it becomes scrollable
// with ↑/↓ (any other key still closes it — see Update()).
func (m Model) renderHelpOverlay(background string) string {
	r := m.styles.renderer
	titleStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))

	lines, availH := m.helpOverlayLayout()
	scroll := len(lines) > availH

	maxOffset := len(lines) - availH
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.helpScroll
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	visible := lines
	if scroll {
		end := offset + availH
		if end > len(lines) {
			end = len(lines)
		}
		visible = lines[offset:end]
	}

	footer := "Press any key to close"
	if scroll {
		footer = "↑↓ scroll · any other key to close"
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Keybindings"))
	sb.WriteString("\n\n")
	sb.WriteString(strings.Join(visible, "\n"))
	sb.WriteString("\n\n")
	sb.WriteString(m.styles.sep.Render(footer))

	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(m.overlayContentWidth(78)).
		Render(sb.String())

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("0")))
}

// renderUpdateOverlay draws a bordered "Version"/"Update available" popup,
// centered over the given background content. Content depends on whether
// the async startup update check has responded yet and whether it found a
// newer release. It's dismissed by Esc, F2, or any other key (see the
// KeyMsg handling in Update()).
func (m Model) renderUpdateOverlay(background string) string {
	r := m.styles.renderer
	titleStyle := r.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	verStyle := m.styles.version
	newVerStyle := m.styles.updateTag
	codeStyle := r.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236")).Padding(0, 1)
	linkStyle := r.NewStyle().Foreground(lipgloss.Color("12")).Underline(true)
	hintStyle := m.styles.sep

	contentW := m.overlayContentWidth(56)
	hasUpdate := m.latestVersion != ""

	var title, statusLine, cmdHeading string
	switch {
	case hasUpdate:
		title = "Update available"
		statusLine = verStyle.Render("v"+m.version) + hintStyle.Render(" → ") + newVerStyle.Render("v"+m.latestVersion)
		cmdHeading = "Run this to update:"
	case m.updateChecked:
		title = "Version"
		statusLine = verStyle.Render("v"+m.version) + hintStyle.Render(" — up to date")
		cmdHeading = "Update commands (for reference):"
	default:
		title = "Version"
		statusLine = verStyle.Render("v"+m.version) + hintStyle.Render(" — checking for updates…")
		cmdHeading = "Update commands (for reference):"
	}

	lines := []string{
		titleStyle.Render(title),
		"",
		statusLine,
		"",
		hintStyle.Render(cmdHeading),
		"",
		codeStyle.Render(truncate("cd /path/to/projectswitcher", contentW-2)),
		codeStyle.Render(truncate("git pull && make install", contentW-2)),
		"",
		hintStyle.Render("Release notes: ") + linkStyle.Render(truncate("https://github.com/ydakilux/projectswitcher/releases", contentW-16)),
	}
	if !hasUpdate {
		lines = append(lines, "",
			hintStyle.Render(truncate("Checked at startup. Disable with PW_NO_UPDATE_CHECK=1.", contentW)))
	}
	lines = append(lines, "", hintStyle.Render("esc to close"))
	lines = m.clampOverlayHeight(lines)

	box := r.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2).
		Width(contentW).
		Render(strings.Join(lines, "\n"))

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
