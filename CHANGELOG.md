# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] - 2026-09-02

### Fixed

- `Ctrl+T`: shell detection now distinguishes PowerShell 7 (`pwsh.exe`) from
  Windows PowerShell 5.1 (`powershell.exe`) via the first `PSModulePath`
  entry, instead of always defaulting to `powershell.exe` regardless of
  which edition was actually running.
- `install.ps1`: now hooks **both** the Windows PowerShell 5.1 and
  PowerShell 7 profiles (not just the one running the installer), since
  `Ctrl+T`'s shell guess can land in either edition.
- `install.ps1`: automatically strips stale hook lines left over from before
  the `pw.ps1` -> `pw-profile.ps1` rename from any existing profile, instead
  of leaving a broken reference that errors on every new shell.
- `install.ps1`: clears a stray ReadOnly file attribute before writing to a
  profile, which could otherwise cause a generic "Access to the path is
  denied" error on some pre-existing profile files.

## [0.3.0] - 2026-09-02

### Added

- `Ctrl+T` shortcut: open a new Windows Terminal tab at the selected
  project's path, best-effort matching the current session's shell (WSL
  distro, PowerShell, or cmd.exe). Requires `wt.exe` on `PATH`; Windows/WSL
  only.
- `?` toggles a full-keybindings help popup, listing every shortcut grouped
  by category (navigation, launch shortcuts, right pane, filter).
- If no `config.json` exists next to the `pw` binary, `pw` now interactively
  prompts to create one with the resolved root/editor settings (`[Y/n]`,
  defaults to yes), so the path is persisted and easy to edit afterward
  instead of silently falling back to `$HOME/work` every run.

### Fixed

- Windows install/build is now considerably more robust after end-to-end
  testing on a real Windows machine:
  - `Makefile`: `help`/`build`/`test`/`clean`/`install` now work under native
    Windows `make` (e.g. GnuWin32/mingw32-make), which previously bypassed
    `SHELL` for simple recipe lines via a raw `CreateProcess`, failing to
    find `go`. Recipes are now explicitly routed through `cmd /c` on
    Windows.
  - `install.ps1`: renamed `shell/pw.ps1` to `shell/pw-profile.ps1`. Once
    `~/go/bin` is added to PATH, a file literally named `pw.ps1` there would
    shadow `pw.exe` when typing bare `pw` (PowerShell resolves script files
    by base name), causing an "unsigned script" execution-policy error
    instead of running the `pw` function. A stale `pw.ps1` from a previous
    install is now also removed automatically.
  - `install.ps1`: the profile hook now bundles
    `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force`
    ahead of the dot-source line, so an unsigned `pw-profile.ps1` loads
    under a default/Restricted execution policy with no manual step.
  - `install.ps1`: profile-hook step now uses raw `System.IO` calls instead
    of `Test-Path`/`New-Item`/`Add-Content`, which could misreport a
    directory as present (or fail to create the profile file) on some
    OneDrive-synced profile paths; falls back to precise manual
    File-Explorer instructions if the write still fails.
  - `install.ps1`: now also adds `~/go/bin` to the user's persisted PATH, so
    `pw.exe` resolves by name even if the profile hook doesn't take effect.
  - `install.ps1`: removed non-ASCII em-dash characters, which Windows
    PowerShell 5.1 could misdecode (UTF-8 without BOM read via the system
    codepage), corrupting script parsing ("missing closing brace").
  - `shell/pw-profile.ps1`: forces array coercion on `pw.exe`'s captured
    output (PowerShell can collapse it to a plain string under some
    conditions, silently breaking line indexing), trims stray whitespace,
    and now surfaces `Set-Location` failures instead of failing silently.

## [0.2.0] - 2026-09-01

### Added

- Right pane **Files view**: press `Tab` to toggle between the Git view and a
  navigable file explorer for the selected project. Shows each entry's size,
  modified date, and git status (colored: yellow modified, cyan untracked,
  green added/staged, red deleted, magenta conflict). Navigate into
  subdirectories with `Enter`/`Right`, back up with `Left` (bounded to the
  selected project's root), and scroll long directory listings with the
  existing `Ctrl+D`/`Ctrl+B`/PgUp/PgDn bindings. The cursor is preserved on
  the folder you came from when navigating back up.
- Auto-descend to the active subfolder on startup: launching `pw` from a
  directory under the configured root now opens the switcher with the nav
  stack descended to, and the cursor on, the matching project.
- `Ctrl+O` shortcut: select project, `cd`, and launch `opencode`.
- `Ctrl+E` shortcut: select project, `cd`, and open in a configured editor.
- `editor` field in `config.json` and `PW_EDITOR` environment variable to
  configure the editor launched by `Ctrl+E` (default: `code`).

### Changed

- Shell wrappers (`pw.bash`, `pw.zsh`, `pw.fish`, `pw.ps1`) now read up to
  three lines from `pw`'s stdout (selected path, launch action, editor
  command) to support the new `Ctrl+O` / `Ctrl+E` launch shortcuts.
- Preview pane's "Last commit" line now shows the absolute commit date
  (`YYYY-MM-DD`) alongside the relative age, ordered right after the commit
  hash so it isn't clipped when the subject line is long.

## [0.1.0] - 2026-07-15

### Added

- Initial release: terminal UI (TUI) fuzzy project switcher.
- Scans directories under a configurable root and lists them with fuzzy
  filtering.
- Drill-down navigation into non-git container folders (`→` / `←`).
- Git status preview pane (branch, ahead/behind, dirty state, last commit,
  README, file listing).
- `Ctrl+R` to `git pull` the highlighted repo.
- Recent-projects tracking with relative timestamps, persisted to a state
  file.
- Root resolution via `--root` flag, `PW_ROOT` env var, `config.json`, or
  `$HOME/work` default.
- Shell integration for bash, zsh, fish, and PowerShell.
- Windows support (cross-compiled `pw.exe`, PowerShell wrapper).

[Unreleased]: https://github.com/ydakilux/projectswitcher/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/ydakilux/projectswitcher/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/ydakilux/projectswitcher/compare/v0.2.0...v0.3.0
[0.1.0]: https://github.com/ydakilux/projectswitcher/releases/tag/v0.1.0
