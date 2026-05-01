# wt - Git Worktree CLI

A Go-based CLI tool for managing Git worktrees with automatic tmux session management and editor integration.

## Installation

```bash
# Build and install
go install .
```

Or build a local binary:

```bash
go build -o wt .
```

## Configuration

### Configuration Hierarchy

Priority (highest to lowest):

1. **Environment Variables**
2. **Local Project Config** — `.wt.toml` in repository root
3. **Global User Config** — `~/.config/wt/config.toml`
4. **Built-in Defaults**

### Configuration Options

#### `worktrees_location`
**Default:** `~/projects/worktrees`  
Base directory where worktrees are created.

#### `copy_files`
**Default:** `[]`  
Files/patterns to copy from the main repository into new worktrees. Supports glob patterns (`config/*.json`, `.env*`, `scripts/*`).

#### `tmux_windows`
**Default:** `[]`  
Windows to create when a new tmux session starts. Commands run in the worktree directory.

### Environment Variables

- `WORKTREE_BASE_DIR` — Overrides `worktrees_location`
- `XDG_CONFIG_HOME` — Changes the global config directory (default: `~/.config`)

### Configuration Examples

**Global** (`~/.config/wt/config.toml`):
```toml
worktrees_location = "~/projects/worktrees"
copy_files = [".env.example", ".vscode/settings.json"]
```

**Project** (`.wt.toml` in repo root):
```toml
copy_files = ["config/local.json"]

tmux_windows = [
    { name = "editor",   command = "nvim ." },
    { name = "server",   command = "pnpm dev" },
    { name = "tests",    command = "pnpm test --watch" },
    { name = "terminal", command = "" }
]
```

### Configuration Merging

- `copy_files` and `tmux_windows` arrays are merged (local appends to global; `copy_files` deduplicates)
- `worktrees_location` in local config overrides global

---

## Commands

### Global Flags

- `--no-editor` — Don't open the editor
- `--no-tmux` — Don't create/switch tmux sessions
- `-v, --verbose` — Verbose output for debugging

### List worktrees

```bash
wt list
```

Lists all worktrees for the current repository. Use `--all` to list across all projects, `--json` for machine-readable output, `--path-only` for paths only.

### Create worktree

```bash
# New branch
wt new <branch-name>

# Interactive — prompts for branch name
wt new

# From existing branch (name derived from branch)
wt new --from origin/release-1.2

# From existing branch with explicit worktree name
wt new release-1.2 --from origin/release-1.2

# Interactive branch selection
wt new --from :pick
```

If a worktree already exists for the branch, `wt` offers to switch to it instead of creating a duplicate.

### Open worktree

```bash
wt open

# Browse across all projects
wt open --all

# Filter to a specific project
wt open --project <project-name>
```

When inside a repository, lists that repo's worktrees directly. Outside a repository, shows a project picker first. If only one option exists, selection is skipped automatically.

### Delete worktree

```bash
# Interactive selection with confirmation
wt delete

# Delete specific worktree
wt delete <worktree-name>

# Skip confirmation
wt delete <worktree-name> --force
```

Prevents deletion of the main worktree. Kills the associated tmux session. Does **not** delete the underlying Git branch.

### Track idle Claude agent sessions

`wt idle` tracks which tmux panes have a Claude agent waiting for input, so you can jump directly to the right session without hunting through `tmux ls`.

#### Hook setup

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "wt idle register" }]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [{ "type": "command", "command": "wt idle active" }]
      }
    ]
  }
}
```

Both hooks exit 0 silently when not inside tmux, so they are safe to add globally.

#### Commands

```bash
# List sessions currently waiting for input
wt idle list

# Include active sessions (adds STATUS column)
wt idle list --all

# Switch to a session by ID or name (marks it active)
wt idle switch 1
wt idle switch my-session

# Remove records for sessions that no longer exist in tmux
wt idle clean

# Called automatically by hooks — also available manually
wt idle register   # mark current pane as waiting
wt idle active     # mark current pane as active
```

Session state is stored at `~/.local/share/wt/idle-sessions.json`.

#### tmux picker

Add a popup picker to your `~/.tmux.conf`:

```tmux
bind I display-popup \
  -w 75% \
  -h 40% \
  -E "~/.dotfiles/.scripts/idle-session-picker"
```

Picker script (`~/.dotfiles/.scripts/idle-session-picker`):

```bash
#!/usr/bin/env bash

output=$(wt idle list 2>/dev/null)

if [ -z "$output" ] || [ "$output" = "No sessions." ]; then
  echo "No idle Claude sessions."
  sleep 1.5
  exit 0
fi

header=$(echo "$output" | head -1)
rows=$(echo "$output" | tail -n +2 | grep -v '^$' | grep -v '^Note:')

if [ -z "$rows" ]; then
  echo "No idle Claude sessions."
  sleep 1.5
  exit 0
fi

selected=$(echo "$rows" | fzf \
  --header="$header" \
  --reverse \
  --prompt="Claude idle > " \
  --no-sort \
  --height=100%)

[ -z "$selected" ] && exit 0

id=$(echo "$selected" | awk '{print $1}')
wt idle switch "$id"
```

---

## Development

```bash
go mod download   # install dependencies
go run . <cmd>    # run without building
go test ./...     # run tests
go fmt ./...      # format code
```

### GitHub PR Workflow

1. Create a feature branch from `main`.
2. Run `go fmt ./...` and `go test ./...` before pushing.
3. Open a PR against `main` with context and test results.

## Requirements

- Git
- Go 1.21+
- tmux (optional — for session management and idle tracking)
- `fzf` (optional — only required for the `idle-session-picker` script above)
- `$EDITOR` (optional — for editor integration)
- Claude Code (optional — for `wt idle` hook integration)
