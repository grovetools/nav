# Advanced Topics

This section covers practical automation patterns and a technical overview of how the live sessionizer updates data.

## Scripting with gmux

The gmux CLI is script-friendly. The following examples show how to combine `gmux session`, `gmux launch`, and `gmux wait` to automate tmux environments.

Note: `gmux launch` creates sessions; it does not attach. Use `tmux attach-session -t <name>` to attach when needed.

### 1) Idempotent “ensure session” script
Start a session only if it doesn’t already exist. This is useful in login shells or project entry scripts.

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION="dev-env"

if gmux session exists "$SESSION" >/dev/null 2>&1; then
  echo "Session '$SESSION' already exists."
else
  # Launch with a named window and a few panes
  gmux launch "$SESSION" \
    --window-name "main" \
    --working-dir "$HOME/Work/my-app" \
    --pane "nvim" \
    --pane "npm run dev@frontend" \
    --pane "go run .@backend"
  echo "Session '$SESSION' created."
fi

# Attach if not currently inside tmux
if [[ -z "${TMUX:-}" ]]; then
  tmux attach-session -t "$SESSION"
fi
```

Key points:
- `gmux session exists` returns exit code 0 if the session exists, 1 otherwise.
- `--pane "cmd[@workdir]"` lets you run a command and optionally set a working directory per pane.

### 2) Start a session and wait for it to finish
Use `gmux wait` to block until a session ends. This is helpful in CI or when you want to run follow-up tasks after the session closes.

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION="batch-job"

# Create a short-lived batch session that runs a script
gmux launch "$SESSION" \
  --window-name "job" \
  --working-dir "$HOME/Work/batch" \
  --pane "./run_job.sh"

# Wait until the session closes (script finishes). Add a timeout if appropriate.
if gmux wait "$SESSION" --poll-interval 500ms --timeout 10m; then
  echo "Session '$SESSION' completed."
else
  echo "Session '$SESSION' did not complete in time." >&2
  exit 1
fi

# Optional: post-processing
./post_process_results.sh
```

Key points:
- `--poll-interval` controls how often `gmux wait` checks for the session.
- `--timeout` returns a non-zero status if the session hasn’t closed by the deadline.

### 3) Controlled teardown with traps
Ensure sessions are cleaned up when a script exits or receives a signal.

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION="ephemeral-env"
cleanup() {
  # Kill only if it still exists
  if gmux session exists "$SESSION" >/dev/null 2>&1; then
    gmux session kill "$SESSION" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

gmux launch "$SESSION" \
  --window-name "work" \
  --working-dir "$PWD" \
  --pane "nvim" \
  --pane "make test"

# Optionally attach; script exits when you detach/kill the session
if [[ -z "${TMUX:-}" ]]; then
  tmux attach-session -t "$SESSION"
else
  echo "Launched '$SESSION' inside tmux."
fi

# When the script exits, trap will clean up the session if it’s still running.
```

### 4) Wait on a development session before continuing a pipeline
Run a script that pauses until a developer closes the session.

```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION="review"

if ! gmux session exists "$SESSION" >/dev/null 2>&1; then
  gmux launch "$SESSION" \
    --window-name "review" \
    --working-dir "$HOME/Work/repo" \
    --pane "git log --oneline --graph --decorate --all" \
    --pane "bash"
fi

echo "Attach with: tmux attach -t $SESSION"
echo "Waiting for '$SESSION' to close..."
gmux wait "$SESSION" --poll-interval 1s
echo "Continuing pipeline..."
```

## Live Update Architecture

The live sessionizer (`gmux sz`) is built with Bubble Tea (github.com/charmbracelet/bubbletea) and maintains a responsive TUI that refreshes multiple data sources every 10 seconds without disrupting user interaction.

### Update cadence and background commands
The model initializes a periodic tick and several background fetches:

- tea.Tick: Schedules a tick every 10 seconds.
- fetchGitStatusCmd: Collects Git status for open tmux sessions.
- fetchClaudeSessionsCmd: Reads active Claude sessions (via grove-hooks) in JSON.
- fetchProjectsCmd: Re-discovers projects from configured search paths.
- fetchRunningSessionsCmd: Lists currently running tmux sessions.
- fetchKeyMapCmd: Reloads key bindings from tmux-sessions.yaml.

On each tick, these commands re-run in a batch to refresh state:
- gitStatusUpdateMsg
- claudeSessionUpdateMsg
- projectsUpdateMsg
- runningSessionsUpdateMsg
- keyMapUpdateMsg

This design isolates I/O and system queries from the UI thread and keeps updates predictable.

### Data model and rendering
Key model fields (sessionizeModel):
- projects, filtered: Full project list and the currently filtered view.
- keyMap: Mapping of project path -> assigned key.
- runningSessions: Set of active tmux session names.
- gitStatusMap: Extended Git status by path, including added/deleted line counts.
- claudeStatusMap, claudeDurationMap: Claude session state by path (when grove-hooks is available).
- cursor, filterInput: User interaction state preserved across updates.

Rendering notes:
- The UI uses indicators for session state:
  - Green or blue dot (●) for active tmux sessions (blue marks the currently attached session).
  - Claude status symbols (▶ running, ⏸ idle, ✓ completed, ✗ failed) when available.
- Git status summary is compact (e.g., ↑1 ↓2 M:3 S:1 ?:5 +10 -4) and appears for active sessions.
- Worktrees are displayed hierarchically with a “└─” prefix and are filtered by activity (see sorting).

### Minimizing visual disruption
To reduce flicker or “flashing,” the model compares old and new data and only updates when necessary:
- Git changes: extendedGitStatusEqual guards against unnecessary re-renders by comparing upstream counts, file counts, and line deltas.
- Claude changes: maps are reconciled and replaced only when a material change is detected.

### Project discovery and grouping
Project discovery runs on each refresh:
- Search paths are read from configuration; explicit projects are included even if outside search paths.
- Git worktrees under .grove-worktrees are discovered and grouped under their parent repository.
- Default view prioritizes groups with active sessions; parent repositories are always shown, while inactive worktrees are hidden to reduce noise.
- When the filter is active, all matching projects are shown but sorted with active groups first and by match quality (exact name, prefix, contains, path match).

### Preserving user context
The sessionizer preserves cursor position and filter text across updates:
- Selected path is saved before list changes and restored after refresh when possible.
- Filter input remains focused and intact so incremental typing continues smoothly.

### Concurrency and system access
- Git status for open sessions is fetched concurrently using goroutines and a wait group for efficiency.
- tmux interactions (session lists, path lookups, switching) use a tmux client abstraction.
- Claude session data is fetched via grove-hooks if detected in PATH or at ~/.grove/bin/grove-hooks. If not present, the Claude column remains empty without errors.

### Key mapping synchronization
- Key bindings are reloaded on each refresh and can be edited inline from the TUI (ctrl+e to assign, ctrl+x to clear).
- On mapping changes, tmux bindings are regenerated and tmux config can be reloaded automatically when running inside tmux.

This architecture balances responsiveness with stability: periodic, batched refreshes keep the view current, while selective updates and preserved UI state maintain a smooth interactive experience.