# Examples

This guide provides examples for common gmux workflows.

## Example 1: Interactive Session Management (`gmux sz`)

The `gmux sz` command launches a terminal interface to browse and select projects. The list is generated from configured search paths and updates every 10 seconds to reflect changes to tmux sessions, Git status, and key mappings.

#### Launch the Sessionizer

Open the interface from your terminal:
```bash
gmux sz
```

#### Interface Elements

Each line provides a summary of a project's state:
```
  a ● ▶ my-feature  ~/Work/my-repo/.grove-worktrees/my-feature  ↑1 M:3 S:1 +10 -4
  s ●   my-repo     ~/Work/my-repo                              ✓
    ●   another     ~/Personal/another-project
```

-   **Hotkey**: The first character (`a`) is the hotkey mapped to this project.
-   **Session Indicator (`●`)**:
    -   Blue (`●`): The current tmux session.
    -   Green (`●`): A running tmux session in the background.
-   **Claude Indicator (`▶`)**: Shows Claude AI session status if `grove-hooks` is installed (`▶` running, `⏸` idle, `✓` completed, `✗` failed).
-   **Project Name**: `my-feature` is the directory name. A `└─` prefix indicates a Git worktree.
-   **Path**: The full path to the project directory.
-   **Git Status**: A summary for running sessions.
    -   `↑N`: Commits ahead of remote.
    -   `M:N`: Modified files.
    -   `S:N`: Staged files.
    -   `+N -N`: Lines added/deleted across staged and unstaged changes.
    -   `✓`: The repository is clean.

#### Navigation and Selection

-   **Filter**: Type characters to filter the list by project name or path.
-   **Move**: Use `Up`/`Down` arrow keys or `k`/`j`.
-   **Select**: Press `Enter` to create a new tmux session or switch to an existing one for the selected project.

## Example 2: Interactive Key Mapping (`gmux key manage`)

The `gmux key manage` command (or `gmux km`) opens a terminal interface for mapping projects to single-character hotkeys defined in `tmux-sessions.yaml`.

#### Launch the Key Manager

```bash
gmux key manage
```

The interface shows a table of available keys, the project mapped to each key, and the project's path.

#### Mapping and Unmapping

-   **Map a Project**:
    1.  Select an unmapped key using the arrow keys.
    2.  Press `e` to open a searchable list of unmapped projects.
    3.  Type to filter, select a project, and press `Enter` to confirm.
-   **Unmap a Project**:
    1.  Select a mapped key.
    2.  Press `d` or `spacebar` to clear the mapping.

#### Save and Exit

Press `q` or `Esc` to exit. Changes are saved to `tmux-sessions.yaml`, tmux bindings are regenerated, and the tmux configuration is reloaded if possible.

## Example 3: Scripting

`gmux` commands can be used in scripts for automation. This script sets up a tmux environment for a task and waits for the session to be closed before continuing.

**`setup-debug-session.sh`**
```bash
#!/usr/bin/env bash
set -euo pipefail

SESSION_NAME="debug-api"
PROJECT_PATH="$HOME/Work/my-api"

# Launch a new tmux session if it does not already exist.
# The session has two panes: one for an editor and one for running tests.
if ! gmux session exists "$SESSION_NAME" >/dev/null 2>&1; then
  echo "Launching new session: $SESSION_NAME"
  gmux launch "$SESSION_NAME" \
    --working-dir "$PROJECT_PATH" \
    --pane "nvim ." \
    --pane "go test -v ./..."
else
  echo "Session '$SESSION_NAME' already exists."
fi

# Inform the user how to attach.
echo "Attach to the session with: tmux attach-session -t $SESSION_NAME"
echo "The script will continue after the session is closed."
echo

# Block until the session is closed (e.g., via `tmux kill-session`).
gmux wait "$SESSION_NAME"

# Continue with subsequent tasks.
echo "Session '$SESSION_NAME' closed. Running cleanup tasks..."
# ./run_cleanup.sh
```

This script uses `gmux launch` to create a multi-pane layout and `gmux wait` to pause execution until the task is complete.