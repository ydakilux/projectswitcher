// Package term opens a new terminal tab (Windows Terminal) at a given
// directory, matching the shell of the currently running session
// (WSL distro, PowerShell, or cmd) on a best-effort basis.
package term

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// OpenNewTab opens a new Windows Terminal tab at path, attempting to match
// the shell of the current session:
//   - If running inside WSL (WSL_DISTRO_NAME set), opens a new tab running
//     `wsl.exe -d <distro>` cd'd to path (path is expected to be a Linux path).
//   - If running natively on Windows, opens a new tab running cmd.exe or
//     powershell.exe (best-effort guess via the PROMPT env var, which cmd.exe
//     always sets and PowerShell does not), cd'd to path.
//
// Returns an error if wt.exe (Windows Terminal) isn't found on PATH, or if
// the environment isn't Windows or WSL.
func OpenNewTab(path string) error {
	args := buildArgs(path)
	if args == nil {
		return errors.New("new terminal tab shortcut requires Windows or WSL with Windows Terminal installed")
	}
	if _, err := exec.LookPath("wt.exe"); err != nil {
		return errors.New("wt.exe (Windows Terminal) not found on PATH")
	}
	cmd := exec.Command("wt.exe", args...)
	return cmd.Start()
}

// buildArgs returns the wt.exe argument list for opening a new tab at path
// in a shell matching the current session, or nil if unsupported.
func buildArgs(path string) []string {
	if distro := os.Getenv("WSL_DISTRO_NAME"); distro != "" {
		return []string{"-w", "0", "new-tab", "wsl.exe", "-d", distro, "--cd", path}
	}
	if runtime.GOOS == "windows" {
		shell := "powershell.exe"
		if os.Getenv("PROMPT") != "" {
			// cmd.exe always sets PROMPT; PowerShell doesn't. Best-effort
			// heuristic since Go can't cheaply inspect the parent process.
			shell = "cmd.exe"
		}
		return []string{"-w", "0", "new-tab", "-d", path, shell}
	}
	return nil
}
