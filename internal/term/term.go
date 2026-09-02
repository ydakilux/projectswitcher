// Package term opens a new terminal tab (Windows Terminal) at a given
// directory, matching the shell of the currently running session
// (WSL distro, PowerShell 7, Windows PowerShell 5.1, or cmd) on a
// best-effort basis.
package term

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// OpenNewTab opens a new Windows Terminal tab at path, attempting to match
// the shell of the current session:
//   - If running inside WSL (WSL_DISTRO_NAME set), opens a new tab running
//     `wsl.exe -d <distro>` cd'd to path (path is expected to be a Linux path).
//   - If running natively on Windows, opens a new tab running pwsh.exe,
//     powershell.exe, or cmd.exe (best-effort guess, see detectShell), cd'd
//     to path.
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
		return []string{"-w", "0", "new-tab", "-d", path, detectShell()}
	}
	return nil
}

// detectShell guesses which shell binary hosts the current session:
//   - cmd.exe always sets the PROMPT env var; PowerShell (either edition)
//     doesn't, so its presence is a strong cmd.exe signal.
//   - Otherwise, PowerShell prepends its own edition-specific module path
//     as the first PSModulePath entry at startup: a path containing
//     "\WindowsPowerShell\" means Windows PowerShell 5.1, while one
//     containing "\PowerShell\" (without "Windows") means PowerShell 7+
//     (pwsh.exe). This lets us tell the two editions apart, which the
//     PROMPT check alone can't do.
//
// Go can't cheaply inspect the parent process on Windows without extra
// OS-specific dependencies, so this remains a best-effort heuristic.
func detectShell() string {
	if os.Getenv("PROMPT") != "" {
		return "cmd.exe"
	}
	firstModulePath := strings.SplitN(os.Getenv("PSModulePath"), ";", 2)[0]
	lower := strings.ToLower(firstModulePath)
	if strings.Contains(lower, `\windowspowershell\`) {
		return "powershell.exe"
	}
	if strings.Contains(lower, `\powershell\`) {
		if _, err := exec.LookPath("pwsh.exe"); err == nil {
			return "pwsh.exe"
		}
	}
	return "powershell.exe"
}
