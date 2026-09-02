package main

import (
	"bufio"
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
	"pw/internal/version"
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
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("pw version", version.Version)
		os.Exit(0)
	}

	// Load config.json (best-effort, non-fatal)
	cfg, cfgFound, cfgErr := config.Load()
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

	// Resolve editor command: PW_EDITOR env > config.json > "code"
	editor := os.Getenv("PW_EDITOR")
	if editor == "" {
		editor = cfg.Editor
	}
	if editor == "" {
		editor = "code"
	}

	// Resolve to absolute path
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot resolve root path %q: %v\n", root, err)
		os.Exit(1)
	}
	root = absRoot

	// Offer to create config.json next to the exe if it doesn't exist yet,
	// so the resolved settings are persisted and editable (useful on
	// Windows where there's no shell rc file to set PW_ROOT/PW_EDITOR).
	var cfgPath string
	if dir := config.ExeDir(); dir != "" {
		cfgPath = filepath.Join(dir, "config.json")
	}
	if !cfgFound && cfgErr == nil && cfgPath != "" {
		fmt.Fprintf(os.Stderr, "no config.json found. Create %s with root %q? [Y/n] ", cfgPath, root)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "" || answer == "y" || answer == "yes" {
			if err := config.Save(filepath.Dir(cfgPath), config.Config{Root: root, Editor: editor}); err != nil {
				fmt.Fprintln(os.Stderr, "warning: could not create config.json:", err)
			} else {
				fmt.Fprintf(os.Stderr, "info: created %s - edit \"root\" if this isn't the right path\n", cfgPath)
			}
		}
	}

	// Validate root
	info, err := os.Stat(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "root directory %q does not exist: %v\n", root, err)
		if cfgPath != "" {
			fmt.Fprintf(os.Stderr, "edit \"root\" in %s to point at your projects directory, then run pw again\n", cfgPath)
		}
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "root path %q is not a directory\n", root)
		if cfgPath != "" {
			fmt.Fprintf(os.Stderr, "edit \"root\" in %s to point at your projects directory, then run pw again\n", cfgPath)
		}
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

	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	renderer := lipgloss.NewRenderer(ttyOut)
	model := ui.New(root, projects, store.Recent, renderer, version.Version, cwd, editor)

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
		fmt.Println(m.SelectedAction())
		fmt.Println(m.SelectedEditor())
		os.Exit(0)
	}

	fmt.Fprintln(os.Stderr, "cancelled")
	os.Exit(1)
}
