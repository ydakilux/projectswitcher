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

`make build` (and `make build-windows`) build a versioned binary: they pass
the version from `git describe --tags --abbrev=0` to the build via
`-ldflags -X pw/internal/version.Version=...`, falling back to the
constant baked into `internal/version/version.go` if no git tag is found.
A plain `go build -o pw .` always uses that baked-in constant.

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

### Markdown live preview (`Ctrl+L`, files view only)

If [`md-to-pdf`](https://dev.azure.com/movu-robotics/Sandbox/_git/mdToPdfGenerator)
(the `mdToPdfGenerator` package) is found on `PATH` at startup (or via
`PW_MDTOPDF`, see below), highlighting a `.md` file in the Files view (`Tab`
to switch to it) and pressing `Ctrl+L`:

1. Opens that file in the configured editor (same command as `Ctrl+E`, but
   pointed at the single file).
2. Runs `md-to-pdf serve <file>` in the background and waits (up to 5s) for
   it to print its "server ready" line, from which pw parses the real
   listen URL (whatever port it actually bound — `md-to-pdf`'s own
   `--open` flag is intentionally **not** used, since it silently fails to
   open a browser on some WSL setups without `xdg-open`/`wslu`).
3. Once the URL is known, pw asks the OS to open it in a browser via
   `explorer.exe` (WSL/native Windows) or `xdg-open`/`open` (Linux/macOS).

pw stays open the whole time, no `cd`. The status/help bar reports one of:
a startup/parse error from `md-to-pdf`, the resolved preview URL plus a
browser-launch error if opening it failed, or the URL on apparent success.
Note that "opened" only means the OS accepted the request to launch a
browser (`explorer.exe`'s own exit code is unreliable and ignored) — pw
cannot confirm a browser window actually appeared. The `md-to-pdf serve`
process is left running detached in the background; pw does not track,
stop, or otherwise manage its lifetime (it keeps running, on its bound
port, after pw exits) — kill it manually if needed. If that port is
already in use, `md-to-pdf` may fail to bind or bind to a different port;
either way pw reports whatever `md-to-pdf` prints.

This shortcut only applies to `.md` files and is hidden entirely (not shown
in the help bar/popup) when `md-to-pdf` isn't found. Resolution order:
`PW_MDTOPDF` env var (exact binary name/path) > `md-to-pdf` on `PATH`;
if neither resolves via `PATH` lookup, the feature is silently disabled.

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

The bottom status/help bar is adaptive: shortcuts are packed in by priority
only as far as they fit the terminal width, whole segments are dropped
(never cut mid-word), and a trailing `…` marks truncation. `? help` always
comes first so the full keybindings popup stays discoverable even on a
narrow terminal, followed by `^a update` whenever a newer version is
available.

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
| `Ctrl+K` | Files view: create a new directory in the current folder |
| `Ctrl+L` | Files view, `.md` file: open in editor + start `md-to-pdf` live preview (only if `md-to-pdf` is installed) |
| `Ctrl+A` | Open the "Update available" modal (only shown/active when a newer release was detected) |
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

## Update check

On startup, pw asynchronously checks GitHub for the latest released
version of `ydakilux/projectswitcher` (3s timeout, never blocks the UI).
Any failure (offline, rate-limited, etc.) is silently ignored.

If a newer version is available, a passive indicator appears right-aligned
on the Git/Files tab line (styled in the same blue as the path breadcrumb):
`v0.6.1 → v0.7.0`. It stays visible for the rest of the session. Press
`Ctrl+A` to open a dismissible "Update available" modal with the current →
new version, the commands to update (`cd /path/to/projectswitcher` then
`git pull && make install`), and a link to the
[releases page](https://github.com/ydakilux/projectswitcher/releases).
Dismiss it with `Esc` or any other key. `Ctrl+A` only does anything (and
only shows up in the help bar/popup) when an update was actually detected.

All popups (keybindings help, update available, new-directory prompt) size
themselves from the current terminal dimensions and always stay inside the
app frame, whatever your terminal size.

Set `PW_NO_UPDATE_CHECK=1` to disable the check entirely.

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
