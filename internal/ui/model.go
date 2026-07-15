package ui

import (
	"fmt"
	"math"
	"path/filepath"
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
		help:        r.NewStyle().Foreground(lipgloss.Color("8")),
		previewHead: r.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		sep:         r.NewStyle().Foreground(lipgloss.Color("8")),
		paneBorder:  r.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("8")),
		renderer:    r,
	}
}

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

// projectSource implements fuzzy.Source for fuzzy matching.
type projectSource struct {
	projects []project.Project
}

func (ps projectSource) String(i int) string { return ps.projects[i].Name }
func (ps projectSource) Len() int            { return len(ps.projects) }

// Model is the bubbletea model.
type Model struct {
	root         string
	currentDir   string
	stack        []navFrame
	all          []project.Project
	recent       map[string]int64
	filterInput  textinput.Model
	cursor       int
	filtered     []project.Project
	previewVP    viewport.Model
	previewCache map[string]preview.Preview
	selectedPath string
	confirmed    bool
	width        int
	height       int
	styles       styles
	version      string
	pulling      bool
	pullStatus   string
}

// New creates a new Model bound to the given tty renderer.
func New(root string, projects []project.Project, recent map[string]int64, renderer *lipgloss.Renderer, version string) Model {
	ti := textinput.New()
	ti.Placeholder = "filter projects..."
	ti.Focus()
	ti.CharLimit = 100

	m := Model{
		root:         root,
		currentDir:   root,
		all:          projects,
		recent:       recent,
		filterInput:  ti,
		previewCache: make(map[string]preview.Preview),
		width:        80,
		height:       24,
		styles:       newStyles(renderer),
		version:      version,
	}
	m.filtered = m.sortedProjects("")
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.loadPreviewCmd())
}

// sortedProjects returns projects sorted by filter text or recency.
func (m Model) sortedProjects(filter string) []project.Project {
	if filter == "" {
		// Sort by recency descending, then alphabetical
		projects := make([]project.Project, len(m.all))
		copy(projects, m.all)
		type entry struct {
			p  project.Project
			ts int64
		}
		entries := make([]entry, len(projects))
		for i, p := range projects {
			entries[i] = entry{p: p, ts: m.recent[p.Path]}
		}
		// Insertion sort: recent first (ts desc), tie-break name asc
		for i := 1; i < len(entries); i++ {
			for j := i; j > 0; j-- {
				a, b := entries[j-1], entries[j]
				swap := false
				if a.ts < b.ts {
					swap = true
				} else if a.ts == b.ts && strings.ToLower(a.p.Name) > strings.ToLower(b.p.Name) {
					swap = true
				}
				if swap {
					entries[j-1], entries[j] = entries[j], entries[j-1]
				} else {
					break
				}
			}
		}
		for i, e := range entries {
			projects[i] = e.p
		}
		return projects
	}

	// Fuzzy match
	results := fuzzy.FindFrom(filter, projectSource{m.all})
	out := make([]project.Project, len(results))
	for i, r := range results {
		out[i] = m.all[r.Index]
	}
	return out
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

	case tea.KeyMsg:
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
			if len(m.stack) > 0 {
				m = m.ascend()
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			return m, nil

		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.confirmed = true
				m.selectedPath = m.filtered[m.cursor].Path
			}
			return m, tea.Quit

		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
				m.previewVP.SetContent(m.renderPreviewContent())
				return m, m.loadPreviewCmd()
			}
			return m, nil

		case tea.KeyDown, tea.KeyCtrlN:
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

		case tea.KeyCtrlU:
			// Clear filter
			m.filterInput.SetValue("")
			m.filtered = m.sortedProjects("")
			m.cursor = 0
			m.previewVP.SetContent(m.renderPreviewContent())
			return m, m.loadPreviewCmd()

		case tea.KeyCtrlD, tea.KeyCtrlF, tea.KeyPgDown:
			m.previewVP.HalfViewDown()
			return m, nil

		case tea.KeyCtrlB, tea.KeyPgUp:
			m.previewVP.HalfViewUp()
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

	for i := start; i < end; i++ {
		p := m.filtered[i]
		ts := m.recent[p.Path]
		tag := humanizeAge(ts)

		nameMaxW := width - 4
		if tag != "" {
			nameMaxW -= len(tag) + 2
		}
		if nameMaxW < 4 {
			nameMaxW = 4
		}

		name := truncate(p.Name, nameMaxW)

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
			row := " " + name + strings.Repeat(" ", padding) + m.styles.recentTag.Render(tag) + " "
			if p.IsGit {
				sb.WriteString(m.styles.gitRepo.Render(row))
			} else {
				sb.WriteString(m.styles.normal.Render(row))
			}
		} else {
			row := " " + name + " "
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

func (m Model) renderPreviewContent() string {
	if len(m.filtered) == 0 {
		return "(no projects)"
	}
	p := m.filtered[m.cursor]
	pv, ok := m.previewCache[p.Path]
	if !ok {
		return "loading…"
	}

	var sb strings.Builder

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
	helpText := "↑↓ move · → open · ← back · ↵ switch · ^r pull · esc back/quit · ^c quit · ^u clear · ^d/^b scroll"
	if m.pulling {
		helpText = "pulling…"
	} else if m.pullStatus != "" {
		helpText = m.pullStatus
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

	return body + "\n" + help
}

// SelectedPath returns the selected path and confirmation status.
func (m Model) SelectedPath() (string, bool) {
	return m.selectedPath, m.confirmed
}
