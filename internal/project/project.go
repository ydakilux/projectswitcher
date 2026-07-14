package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Project represents a project directory.
type Project struct {
	Name  string
	Path  string
	IsGit bool
}

// Scan reads the root directory and returns a sorted list of projects.
func Scan(root string) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var projects []Project
	for _, entry := range entries {
		name := entry.Name()

		// Skip dotfile-prefixed names
		if strings.HasPrefix(name, ".") {
			continue
		}

		fullPath := filepath.Join(root, name)
		isDir := entry.IsDir()

		// Handle symlinks: os.ReadDir doesn't follow them
		if !isDir && entry.Type()&fs.ModeSymlink != 0 {
			info, err := os.Stat(fullPath)
			if err != nil {
				continue
			}
			isDir = info.IsDir()
		}

		if !isDir {
			continue
		}

		// Check if it's a git repo (works for both dirs and worktree files)
		gitPath := filepath.Join(fullPath, ".git")
		isGit := false
		if _, err := os.Stat(gitPath); err == nil {
			isGit = true
		}

		projects = append(projects, Project{
			Name:  name,
			Path:  fullPath,
			IsGit: isGit,
		})
	}

	// Sort alphabetically, case-insensitive
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, nil
}
