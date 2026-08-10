# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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

[Unreleased]: https://github.com/ydakilux/projectswitcher/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ydakilux/projectswitcher/releases/tag/v0.1.0
