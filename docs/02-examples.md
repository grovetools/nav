# Practical Examples

This guide provides practical examples for the main workflows in `grove-tmux` (`gmux`), from interactive session management to scripting.

## Example 1: The Live Sessionizer (`gmux sz`)

The `gmux sz` command is the primary entry point for interactive session management. It launches a Terminal User Interface (TUI) that provides a live dashboard of all your projects.

#### 1. Launch the Sessionizer

Open the sessionizer from your terminal:
```bash
gmux sz
```

This presents a filterable list of all projects discovered from your configured search paths. The list updates automatically every 10 seconds to reflect changes in session status, Git repositories, and key mappings.

#### 2. Understand the UI

Each line in the sessionizer provides a dense summary of a project's state:

```
  a ● ▶ my-feature  ~/Work/my-repo/.grove-worktrees/my-feature  ↑1 M:3 +10 -4
  s ●   my-repo     ~/Work/my-repo                              ✓
    ○   another     ~/Personal/another-project
```

Let's break down the components of the first line:

-   **`a`**: The hotkey mapped to this session. If unmapped, this space is blank.
-   **`●`**: The tmux session indicator.
    -   `●` (Blue): You are currently attached to this session.
    -   `●` (Green): This session is running in the background.
-   **`▶`**: The Claude AI session status indicator (requires `grove-hooks`).
    -   `▶`: Running
    -   `⏸`: Idle
    -   `✓`: Completed
    -   `✗`: Failed
-   **`my-feature`**: The project name, typically the directory name. A `└─` prefix indicates a Git worktree.
-   **`~/Work/...`**: The full path to the project directory.
-   **`↑1 M:3 +10 -4`**: A compact summary of the Git status for the running session.
    -   `↑1`: 1 commit ahead of the remote branch.
    -   `M:3`: 3 modified files.
    -   `+10 -4`: A total of 10 lines added and 4 lines deleted across staged and unstaged changes.
    -   `✓`: A clean repository.

#### 3. Navigate and Sessionize

-   **Filter**: Start typing to filter the list by project name or path in real-time.
-   **Navigate**: Use the `Up`/`Down` arrow keys (or `k`/`j`) to move the selection.
-   **Select**: Press `Enter` on a project to create or switch to its tmux session. `gmux` handles the underlying `tmux` commands automatically.

## Example 2: Managing Session Hotkeys (`gmux key manage`)

The `gmux key manage` command (aliased as `gmux km`) opens an interactive TUI for mapping projects to single-character hotkeys, allowing for rapid session switching.

#### 1. Launch the Key Manager

```bash
gmux key manage
```

This opens a table showing all available keys (from `tmux-sessions.yaml`), the project currently mapped to each key, and the project's path.

#### 2. Map a Project to a Key

1.  **Navigate**: Use the arrow keys to select an unmapped key (where the "Repository" and "Path" columns are empty).
2.  **Open Project Search**: Press `e` to open a fuzzy-searchable list of all discovered projects that are not yet mapped to a key.
3.  **Find and Select**: Type to filter the project list, use the arrow keys to highlight the desired project, and press `Enter` to confirm the selection.
4.  The TUI will return to the main table, which now shows the project mapped to your chosen key.

#### 3. Unmap a Key

1.  **Navigate**: Use the arrow keys to select a key that is already mapped to a project.
2.  **Unmap**: Press `d` or `spacebar`. The "Repository" and "Path" for that key will be cleared, making the key available again.

#### 4. Save and Exit

Press `q` or `Esc` to exit. Your changes are automatically saved to `~/.config/grove/tmux-sessions.yaml`, the tmux bindings file is regenerated, and `gmux` attempts to reload your tmux configuration.

## Example 3: Scripting with `gmux`

`gmux` commands are designed to be scriptable. This example shows a simple shell script that sets up a temporary development environment for a specific task and waits for it to be closed.

**`setup_and_wait.sh`**
```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION_NAME="debug-api"
PROJECT_PATH="$HOME/Work/my-api"

# 1. Launch a new tmux session if it doesn't already exist.
#    This session has two panes: one for the editor and one for running tests.
if ! gmux session exists "$SESSION_NAME" >/dev/null 2>&1; then
  echo "Launching new session: $SESSION_NAME"
  gmux launch "$SESSION_NAME" \
    --working-dir "$PROJECT_PATH" \
    --pane "nvim ." \
    --pane "go test -v ./..."
else
  echo "Session '$SESSION_NAME' already exists."
fi

# 2. Inform the user how to attach.
echo "Attach to the session with: tmux attach-session -t $SESSION_NAME"
echo "The script will continue after you close the session."
echo

# 3. Wait for the session to be closed.
#    The script will block here until `tmux kill-session -t debug-api` is run.
gmux wait "$SESSION_NAME"

# 4. Continue with subsequent tasks.
echo "Session '$SESSION_NAME' has been closed. Running cleanup tasks..."
# ./run_cleanup.sh
```

This script uses `gmux launch` to create a complex, multi-pane session and `gmux wait` to pause execution until the development task is complete, demonstrating how `gmux` can be integrated into automated workflows.