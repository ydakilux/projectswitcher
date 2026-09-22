# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.2] - 2026-09-22

### Changed

- The "update available" popup is now opened with `Ctrl+A` instead of
  `Ctrl+V`. Windows Terminal (and most terminals) intercept `Ctrl+V` as
  paste, so the binding never reached pw. The help bar segment and
  keybindings popup row were updated accordingly, and the binding stays
  inert/hidden unless a newer release was detected.

## [0.7.1] - 2026-09-22

### Changed

- Bottom help bar is now adaptive: shortcuts are added by priority only
  while they fit the terminal width, whole segments are dropped rather
  than cut mid-word, and a trailing `…` marks truncation. `? help` is now
  the first segment so the full keybindings popup is always discoverable;
  `^v update` follows right after it when an update is available.
- Help/update/new-directory popups now size themselves from the terminal
  dimensions and always stay inside the app frame.

### Fixed

- Keybindings popup no longer cuts descriptions mid-sentence: the
  description column is derived from the actual popup width and wraps
  onto indented continuation lines. When content is too tall it compacts
  blank separators first, and only falls back to a scrollable popup
  (`↑↓ scroll`) if it still doesn't fit.

## [0.7.0] - 2026-09-22

### Added

- `Ctrl+V` opens a dismissible "Update available" modal (only when a newer
  release was actually detected) showing the current → new version, the
  update commands (`cd /path/to/projectswitcher` then `git pull && make
  install`), and a link to
  https://github.com/ydakilux/projectswitcher/releases. Dismiss with `Esc`
  or any other key. The binding is inert and hidden from the help bar/help
  popup when no update is available.
- The passive update indicator has moved from the bottom help bar to the
  right side of the Git/Files tab line, styled in the same blue as the
  path breadcrumb; it shows `v0.6.1 → v0.7.0` when a newer release exists.
- Automatic update check on startup: pw asynchronously queries GitHub's
  latest-release API for `ydakilux/projectswitcher` (3s timeout) and
  compares it against the running build's semver. The check never blocks
  startup and silently ignores all failures (offline, rate-limited, etc.).
  Opt out entirely via the `PW_NO_UPDATE_CHECK=1` environment variable.
- Build-time version injection: `make build` and `make build-windows` now
  pass `-ldflags -X pw/internal/version.Version=...` with the version
  resolved from `git describe --tags --abbrev=0` (stripping the leading
  `v`), falling back to the baked-in constant in
  `internal/version/version.go` when no tag is available. That file
  remains the semver source of truth.
- `Ctrl+L` in the files view, on a highlighted `.md` file: opens it in the
  configured editor and runs `md-to-pdf serve <file>` in the background for
  a live-reloading preview, parsing the server's own "ready" line for its
  real listen URL and opening that URL in a browser via
  `internal/term.OpenBrowser` (`explorer.exe` on WSL/Windows,
  `xdg-open`/`open` elsewhere). Only active when the optional
  [`md-to-pdf`](https://dev.azure.com/movu-robotics/Sandbox/_git/mdToPdfGenerator)
  (`mdToPdfGenerator`) package resolves via `PW_MDTOPDF` or `md-to-pdf` on
  `PATH` at startup; otherwise the shortcut is disabled and hidden from
  help. The server keeps running detached after pw exits — pw does not
  manage its lifecycle. Default port and conflict behavior are entirely
  `md-to-pdf`'s own (commonly 4000); pw only reports what it observes.

### Fixed

- Footer version indicator was previously dropped entirely when the help
  text filled the terminal width (no room left to pad it in); it now
  lives on the tab line instead, independent of help-bar width.
- Project list ordering is now deterministic. Favorites (and same-named
  projects) previously relied on `sort.Slice`, which is not stable, so
  entries sharing a case-insensitive name were left in Go's randomized
  map-iteration order. This made favorites appear to "jump" / reorder
  between renders and navigations while scrolling a long list. All
  alphabetical comparators now break ties on the exact project path via a
  shared `projectLess` helper, and regression tests assert the order is
  stable across map permutations.
- WSL: opening a URL for the live preview above no longer relies on
  `md-to-pdf`'s own browser-open flag (which could fail silently without
  `xdg-open`/`wslu` present); pw now launches the browser itself via
  `explorer.exe`.
- Files view: the filename column width is now computed dynamically from
  the pane width instead of a fixed 30 characters, so long filenames are
  truncated less aggressively on wide terminals and the size/date/git-status
  columns stay aligned at narrow widths.

## [0.6.1] - 2026-09-10

### Added

- `Ctrl+K` in the files view creates a new directory in the current folder
  (prompt for a name, `Enter` to create, `Esc` to cancel).

### Changed

- Stopped tracking the built `pw.exe` binary in git (matches existing
  `pw` ignore rule); build it locally via `make build-windows` instead.

## [0.6.0] - 2026-09-02

### Added

- Favorites: `Ctrl+F` toggles the highlighted project as a favorite.
  Favorited projects are pinned to the top of the list (alphabetically
  sorted among themselves) and marked with a `★` glyph; favorite state
  persists in the same state file as recent projects.
- `Ctrl+G` toggles a favorites-only view, restricting the list to favorited
  projects (fuzzy filter still applies on top). The header shows
  `[favorites]` while active.
- `1`..`9` quick-highlight: while in favorites-only view (`Ctrl+G`) with an
  empty filter, pressing a bare digit key moves the cursor to (highlights)
  the Nth favorite (alphabetical order) without selecting/exiting — press
  `Enter`, `Ctrl+O`, `Ctrl+E`, etc. afterward to act on it. Digits still
  type into the filter as usual outside of that specific mode/state.
  This lands on a bare-digit scheme after two earlier attempts proved
  non-portable: `Ctrl+1`-`Ctrl+9` don't transmit distinct control codes for
  Ctrl+digit in most terminals, and `Alt+1`-`Alt+9` (ESC-prefixed) behaved
  inconsistently across terminals/WSL configurations.
- Favorited rows show their `1`-`9` quick-highlight digit right before the
  `★` glyph (e.g. `3★ myproject`) for the first 9 favorites, matching the
  number expected by the shortcut above.

### Changed

- `Ctrl+F` no longer scrolls the preview pane down (conflicted with the new
  favorite-toggle shortcut); use `Ctrl+D` or `PgDn` instead.

## [0.5.1] - 2026-09-02

### Fixed

- `install.ps1` no longer depends on the `Get-FileHash` cmdlet, which failed
  with "not recognized" in some restricted/managed PowerShell environments
  (module auto-loading issue). Replaced with a direct `System.IO`/
  `System.Security.Cryptography` SHA256 implementation, matching the
  dependency-free approach already used elsewhere in the script.

## [0.5.0] - 2026-09-02

### Changed

- `Ctrl+E` no longer exits `pw` or `cd`s the calling shell: it now launches
  the configured editor directly in a new window and pw stays open, so you
  can keep browsing/opening multiple projects in your editor without
  re-running `pw` each time.
- `make install` now works end-to-end on native Windows too: it
  automatically runs `install.ps1` instead of just printing instructions to
  run it manually. WSL/Linux/macOS behavior (hooking bash/zsh/fish rc files)
  is unchanged.

## [0.4.0] - 2026-09-02

### Added

- `Ctrl+X` shortcut: open Windows Explorer at the selected project's path.
  Works both natively on Windows and from inside WSL (translates the Linux
  path via `wslpath -w` first). Requires `explorer.exe` on `PATH`;
  Windows/WSL only.

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

[Unreleased]: https://github.com/ydakilux/projectswitcher/compare/v0.7.2...HEAD
[0.7.2]: https://github.com/ydakilux/projectswitcher/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/ydakilux/projectswitcher/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/ydakilux/projectswitcher/compare/v0.6.1...v0.7.0
[0.5.1]: https://github.com/ydakilux/projectswitcher/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/ydakilux/projectswitcher/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/ydakilux/projectswitcher/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/ydakilux/projectswitcher/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/ydakilux/projectswitcher/compare/v0.2.0...v0.3.0
[0.1.0]: https://github.com/ydakilux/projectswitcher/releases/tag/v0.1.0
