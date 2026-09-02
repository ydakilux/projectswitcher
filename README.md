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

| Shortcut | Behavior |
|----------|----------|
| `Enter` | Select highlighted project, `cd` into it, exit pw |
| `Ctrl+O` | Select project, `cd` into it, exit pw, and launch [`opencode`](https://opencode.ai) |
| `Ctrl+E` | Open project in configured editor (new window) - pw stays open, no `cd` |
| `Ctrl+T` | New Windows Terminal tab at this path (see below) - pw stays open |
| `Ctrl+X` | Windows Explorer at this path (see below) - pw stays open |
| `Ctrl+F` | Toggle favorite on highlighted project |
| `Ctrl+G` | Toggle favorites-only view |
| `1`..`9` | In favorites-only view with an empty filter: move cursor to Nth favorite (then act on it) |

`Ctrl+E` opens the selected project in a configurable editor command. Set it
via the `editor` field in `config.json`:

```json
{"editor": "code"}
```

**Precedence:** `PW_EDITOR` env var > `config.json` `editor` field > `code` (default).

### New terminal tab (`Ctrl+T`)

`Ctrl+T` opens a new [Windows Terminal](https://aka.ms/terminal) tab at the
selected project's path, best-effort matching the shell of the current
session:

- Inside WSL (`WSL_DISTRO_NAME` set): opens a new tab running the same WSL
  distro, `cd`'d to the path.
- Native Windows: opens `cmd.exe`, `powershell.exe`, or `pwsh.exe`, guessed
  from the `PROMPT` env var (cmd.exe always sets it) and the first
  `PSModulePath` entry (distinguishes PowerShell 7 from Windows PowerShell
  5.1).

Requires `wt.exe` on `PATH`. Not supported outside Windows/WSL.

### Windows Explorer (`Ctrl+X`)

`Ctrl+X` opens Windows Explorer at the selected project's path:

- Inside WSL: translates the Linux path to a Windows-visible path via
  `wslpath -w` (a `\\wsl$\<distro>\...` UNC path, or a native path if it's
  under `/mnt/c` etc.) before launching Explorer.
- Native Windows: opens Explorer directly at the path.

Requires `explorer.exe` on `PATH`. Not supported outside Windows/WSL.

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
| `Ctrl+E` | Open project in configured editor (new window, pw stays open) |
| `Ctrl+T` | Open a new Windows Terminal tab at this path |
| `Ctrl+X` | Open this path in Windows Explorer |
| `Ctrl+F` | Toggle favorite on highlighted project |
| `Ctrl+G` | Toggle favorites-only view |
| `1`..`9` | In favorites-only view with an empty filter: move cursor to Nth favorite (then act on it) |
| `Esc` | Go back one level, or cancel at root |
| `Ctrl+C` | Cancel immediately (any depth) |
| `Ctrl+U` | Clear filter |
| `Ctrl+D` / `PgDn` | Scroll preview down |
| `Ctrl+B` / `PgUp` | Scroll preview up |
| `Tab` | Toggle right pane between Git view and Files view |
| `?` | Toggle full keybindings help popup |

## Favorites

Press `Ctrl+F` to toggle the highlighted project as a favorite. Favorited
projects are pinned to the top of the list, sorted alphabetically, and
marked with a `★` glyph (recent-use timestamps still show alongside, if
applicable). The first 9 favorites (in that same alphabetical order) also
show their quick-highlight digit right before the star, e.g. `3★ myproject` —
that's the exact number to press for the `1`-`9` shortcut below.
Favorites persist in the same state file as recent projects
(`recent.json`).

Press `Ctrl+G` to restrict the list to favorites only (fuzzy filtering still
applies on top); the header shows `[favorites]` while active. While in
favorites-only view **with the filter empty**, pressing a bare digit `1`-`9`
moves the cursor to (highlights) the Nth favorite (alphabetical order) —
it does not select/exit by itself, so you can then press `Enter` to `cd`
into it, `Ctrl+O`/`Ctrl+E` to launch opencode/editor, `Ctrl+F` to
unfavorite it, etc. If the filter has any text typed, or favorites-only
view is off, digit keys are typed into the filter as usual.

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
| `Ctrl+D` / `PgDn` | Scroll down |
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
. C:\path\to\shell\pw-profile.ps1
```

Then use `pw` normally — it calls `pw.exe` and `cd`s into the selected directory.

### Notes

- A `cmd.exe` batch wrapper is not provided (fragile for this use case). Use PowerShell, Windows Terminal, or WSL (which uses the existing bash/zsh wrapper).
- WSL users: use the `shell/pw.bash` or `shell/pw.zsh` wrapper inside the WSL environment as normal.
