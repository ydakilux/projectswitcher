# pw — Project Switcher

A terminal UI (TUI) fuzzy project switcher. Lists directories under a root folder, lets you filter/select one, and `cd`s into it via a shell wrapper.

See [CHANGELOG.md](CHANGELOG.md) for release history.

## Build & Install

```bash
git clone <repo>
cd projectswitcher
make build      # builds ./pw
make install    # copies pw to ~/go/bin/pw
```

Or manually:
```bash
go build -o pw .
cp pw /usr/local/bin/pw   # or anywhere on $PATH
```

## Shell Integration

Source the appropriate file for your shell. Add to your shell config:

**bash** (`~/.bashrc`):
```bash
source /path/to/shell/pw.bash
```

**zsh** (`~/.zshrc`):
```zsh
source /path/to/shell/pw.zsh
```

**fish** (`~/.config/fish/config.fish`):
```fish
source /path/to/shell/pw.fish
```

## Usage

```bash
pw                          # scan $HOME/work
pw --root ~/projects        # scan a specific root
PW_ROOT=~/projects pw       # via env var
```

Root resolution order: `--root` flag → `PW_ROOT` env var → `config.json` → `$HOME/work`.

## config.json

Place a `config.json` file in the **same directory as the `pw` binary** (e.g. `~/go/bin/config.json` if installed there).

**Schema:**
```json
{
  "root": "/home/you/projects",
  "editor": "code"
}
```
or with tilde expansion for `root`:
```json
{"root": "~/projects"}
```

**Precedence:** `--root` > `PW_ROOT` > `config.json` > `~/work`.

**Quick setup:**
```bash
echo '{"root": "~/projects"}' > ~/go/bin/config.json
```

## Launch Shortcuts

Besides `Enter` (select & `cd`), two shortcuts select the highlighted
project, `cd` into it, and launch an external tool:

| Shortcut | Launches |
|----------|----------|
| `Ctrl+O` | [`opencode`](https://opencode.ai) |
| `Ctrl+E` | Configured editor (see below) |

### Editor configuration

`Ctrl+E` opens the selected project in a configurable editor command. Set it
via the `editor` field in `config.json`:

```json
{"editor": "code"}
```

**Precedence:** `PW_EDITOR` env var > `config.json` `editor` field > `code` (default).

## Keybindings

| Key | Action |
|-----|--------|
| Type anything | Filter projects (fuzzy) |
| `↑` / `↓` | Move cursor up/down |
| `Ctrl+P` / `Ctrl+N` | Move cursor up/down |
| `→` (Right) | Descend into highlighted folder (non-git container) |
| `←` (Left) | Go back to parent level |
| `Enter` | Select project & `cd` (works at any depth) |
| `Ctrl+O` | Select project, `cd`, and launch `opencode` |
| `Ctrl+E` | Select project, `cd`, and open in the configured editor |
| `Esc` | Go back one level, or cancel at root |
| `Ctrl+C` | Cancel immediately (any depth) |
| `Ctrl+U` | Clear filter |
| `Ctrl+D` / `Ctrl+F` / `PgDn` | Scroll preview down |
| `Ctrl+B` / `PgUp` | Scroll preview up |
| `Tab` | Toggle right pane between Git view and Files view |

## Files view

Press `Tab` to switch the right pane from the Git view to a navigable file
explorer for the highlighted project. Each entry shows size, modified date,
and git status (colored: yellow modified, cyan untracked, green
added/staged, red deleted, magenta conflict).

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move cursor |
| `Enter` / `→` (Right) | Descend into highlighted directory |
| `←` (Left) | Go back up a directory (bounded to the project's root) |
| `Ctrl+D` / `Ctrl+F` / `PgDn` | Scroll down |
| `Ctrl+B` / `PgUp` | Scroll up |

## Notes

- **git is optional**: if the `git` binary is not on `PATH`, git info is omitted gracefully — the rest of the UI works fine.
- Recent projects float to the top of the unfiltered list with a relative timestamp (e.g. "2h ago").
- The binary writes its result to stdout as up to three lines — selected path, launch action (empty, `opencode`, or `editor`), and editor command — consumed by the shell wrapper. All interactive UI output goes to stderr or `/dev/tty`.
- **State file location:**
  - Linux/macOS: `$XDG_STATE_HOME/pw/recent.json` (default: `~/.local/state/pw/recent.json`)
  - Windows: `%LOCALAPPDATA%\pw\recent.json` (e.g. `C:\Users\you\AppData\Local\pw\recent.json`)

## Windows

### Building

Cross-compile from Linux/macOS:
```bash
make build-windows   # produces pw.exe
```

Or build natively on Windows:
```powershell
go build -o pw.exe .
```

### Requirements

- Requires `git.exe` on `PATH` (Git for Windows) for the git-info preview feature. Degrades gracefully if absent — same behaviour as Unix.
- Colors and TUI rendering require a VT100-capable terminal: **Windows Terminal**, **PowerShell 7 (pwsh)**, or the Windows 10 1909+ console host. Legacy `cmd.exe` / old conhost may render control sequences as raw text.

### PowerShell setup

Add to your PowerShell profile (`$PROFILE`):
```powershell
. C:\path\to\shell\pw.ps1
```

Then use `pw` normally — it calls `pw.exe` and `cd`s into the selected directory.

### Notes

- A `cmd.exe` batch wrapper is not provided (fragile for this use case). Use PowerShell, Windows Terminal, or WSL (which uses the existing bash/zsh wrapper).
- WSL users: use the `shell/pw.bash` or `shell/pw.zsh` wrapper inside the WSL environment as normal.
