package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pw/internal/config"
	"pw/internal/project"
	"pw/internal/state"
	"pw/internal/ui"
)

func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func main() {
	rootFlag := flag.String("root", "", "root directory to scan for projects")
	flag.Parse()

	// Load config.json (best-effort, non-fatal)
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		fmt.Fprintln(os.Stderr, "warning: config.json:", cfgErr)
	}

	// Resolve root: flag > PW_ROOT env > config.json > $HOME/work
	root := *rootFlag
	if root == "" {
		root = os.Getenv("PW_ROOT")
	}
	if root == "" && cfg.Root != "" {
		root = cfg.Root
	}
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot determine home directory:", err)
			os.Exit(1)
		}
		root = filepath.Join(home, "work")
	}

	// Expand leading ~
	root = expandTilde(root)

	// Resolve to absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot resolve root path %q: %v\n", root, err)
		os.Exit(1)
	}
	root = absRoot

	// Validate root
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "root directory %q does not exist: %v\n", root, err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "root path %q is not a directory\n", root)
		os.Exit(1)
	}

	// Load state
	store, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load state:", err)
		store = &state.Store{Recent: map[string]int64{}}
	}

	// Scan projects
	projects, err := project.Scan(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning %q: %v\n", root, err)
		os.Exit(1)
	}
	if len(projects) == 0 {
		fmt.Fprintf(os.Stderr, "no projects found under %s\n", root)
		os.Exit(1)
	}

	// Open terminal for TUI I/O (stdout is reserved for the final path)
	ttyIn, ttyOut, ttyClose, err := openTTY()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pw must run in an interactive terminal")
		os.Exit(1)
	}
	defer ttyClose()

	renderer := lipgloss.NewRenderer(ttyOut)
	model := ui.New(root, projects, store.Recent, renderer)

	p := tea.NewProgram(
		model,
		tea.WithInput(ttyIn),
		tea.WithOutput(ttyOut),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error running TUI:", err)
		os.Exit(1)
	}

	m := finalModel.(ui.Model)
	path, confirmed := m.SelectedPath()

	if confirmed && path != "" {
		store.Touch(path)
		_ = store.Save()
		fmt.Println(path)
		os.Exit(0)
	}

	fmt.Fprintln(os.Stderr, "cancelled")
	os.Exit(1)
}
