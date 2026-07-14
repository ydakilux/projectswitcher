package preview

import (
	"os"
	"path/filepath"
	"strings"

	"pw/internal/gitinfo"
	"pw/internal/project"
)

// Preview holds the preview data for a project.
type Preview struct {
	Readme string
	Git    gitinfo.Info
	Files  []string
}

// Build constructs a Preview for the given project.
func Build(p project.Project) Preview {
	var pv Preview

	if p.IsGit {
		pv.Git = gitinfo.Get(p.Path)
	}

	// Find readme
	pv.Readme = findReadme(p.Path)

	// List files
	pv.Files = listFiles(p.Path)

	return pv
}

var readmeNames = []string{"readme.md", "readme", "readme.txt"}

func findReadme(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Build a map of lowercase name -> actual name
	nameMap := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			nameMap[strings.ToLower(e.Name())] = e.Name()
		}
	}

	for _, candidate := range readmeNames {
		actual, ok := nameMap[candidate]
		if !ok {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, actual))
		if err != nil {
			continue
		}

		// Cap to 2000 bytes
		if len(data) > 2000 {
			data = data[:2000]
		}

		// Cap to first 40 lines, trim trailing whitespace per line
		lines := strings.Split(string(data), "\n")
		if len(lines) > 40 {
			lines = lines[:40]
		}
		for i, l := range lines {
			lines[i] = strings.TrimRight(l, " \t\r")
		}
		return strings.Join(lines, "\n")
	}

	return ""
}

func listFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		files = append(files, name)
		if len(files) >= 20 {
			break
		}
	}
	return files
}
