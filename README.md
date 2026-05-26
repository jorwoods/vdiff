# vdiff

A keyboard-driven terminal UI for browsing git commit diffs. Select a commit on
the left, read its diff on the right — with search, visual line selection, and
clipboard copy built in.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) /
[Bubbles](https://github.com/charmbracelet/bubbles) /
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## Install

```sh
go install vdiff@latest
```

Or build from source:

```sh
git clone https://github.com/yourname/vdiff
cd vdiff
go build -o vdiff .
```

Requires Go 1.22+.

---

## Usage

```sh
vdiff
```

Opens `git log` in the current repository. Use `c` to switch to any other git
log command at runtime.

---

## Layout

```
┌─ git log ──────────────┐ ┌─ diff ──────────────────────────────────────────┐
│ abc12 Add feature      │ │ commit abc12def                                 │
│ def34 Fix auth bug     │ │ Author: Jordan <jordan@example.com>             │
│ 0a1b2 Add unit tests   │ │ Date:   Mon Jan 1 00:00:00 2024                 │
│ …                      │ │                                                 │
│                        │ │     Add feature                                 │
│                        │ │                                                 │
│                        │ │ diff --git a/foo.go b/foo.go                    │
│                        │ │ @@ -1,3 +1,4 @@                                 │
│                        │ │  context line                                   │
│                        │ │ -removed line                                   │
│                        │ │ +added line                                     │
└────────────────────────┘ └─────────────────────────────────────────────────┘
j/k: navigate  /: filter  s: stash  c: command  tab: diff  q: quit
```

The list pane takes up roughly one-third of the terminal width; the diff pane
takes the rest. The active pane is highlighted with a coloured border. `tab`
switches focus between the two.

---

## Keybindings

### List pane

| Key | Action |
|-----|--------|
| `j` / `k` | Move selection down / up |
| `↓` / `↑` | Move selection down / up |
| `/` | Fuzzy-filter the commit list (live, press `esc` to clear) |
| `s` | Switch to `git stash list` |
| `c` | Open the command prompt |
| `tab` | Move focus to the diff pane |
| `q` / `ctrl+c` | Quit |

### Diff pane

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll one line down / up |
| `↓` / `↑` | Scroll one line down / up |
| `d` / `u` | Scroll half a page down / up |
| `/` | Search within the diff (live highlighting) |
| `n` / `N` | Jump to next / previous search match |
| `esc` | Clear search highlight (or cancel visual selection) |
| `v` | Start visual line selection at the current position |
| `y` | Copy selected lines to the clipboard (visual mode only) |
| `tab` | Move focus back to the list pane |
| `q` / `ctrl+c` | Quit |

### Command prompt (`c`)

| Key | Action |
|-----|--------|
| `enter` | Run the command |
| `esc` | Cancel and return to normal mode |

Any `git log` invocation is accepted. `--oneline` is added automatically if no
`--format` / `--pretty` flag is present. A `-- <paths>` file filter is
extracted and forwarded to each `git show` call so diffs are also scoped.

Examples:

```
git log -n 100
git log --author=Jordan
git log -- internal/auth/auth.go
git log -n 50 -- src/
git stash list
git stash show -p stash@{0}
```

---

## Diff colours

| Colour | Meaning |
|--------|---------|
| Green | Added line (`+`) |
| Red | Removed line (`-`) |
| Bold yellow | File header (`--- a/…` / `+++ b/…`) |
| Yellow | Commit metadata (`commit`, `Author:`, `Date:`) |
| Cyan | Hunk header (`@@`) |
| Dark amber background | Search match |
| Bright yellow background | Current search match |
| Grey background | Visual selection |
| Reversed | Visual cursor line |
