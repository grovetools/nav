# Command Reference

This document provides a reference for the `gmux` command-line interface, covering all subcommands and their options.

## `gmux sessionize` (alias: `sz`)

Launches an interactive interface to browse and switch between projects.

### Syntax

```bash
gmux sz [path]
```

### Description

When run without arguments, `gmux sz` starts a terminal user interface (TUI) that lists projects discovered from configured search paths. The list displays project names, paths, and status indicators for running tmux sessions and Git repositories. The view refreshes its data sources every 10 seconds.

If an optional `[path]` argument is provided, the TUI is bypassed, and the command directly creates or switches to a tmux session for that path.

### Example

```bash
# Open the interactive sessionizer TUI
gmux sz

# Directly create or switch to a session for a specific project
gmux sz ~/Work/grove-tmux
```

### `gmux sessionize add`

Adds a project path to the `explicit_projects` list in the configuration file.

#### Syntax

```bash
gmux sz add [path]
```

#### Description

This command adds a project that is outside of the configured `search_paths`. If no `[path]` is provided, it adds the current working directory.

#### Example

```bash
# Add the project at ~/Code/special-project to the configuration
gmux sz add ~/Code/special-project
```

### `gmux sessionize remove`

Removes a project from the `explicit_projects` list in the configuration file.

#### Syntax

```bash
gmux sz remove [path]
```

#### Description

This command removes a project previously added with `gmux sz add`. If no `[path]` is provided, it removes the current working directory.

#### Example

```bash
# Remove the project at ~/Code/special-project from the configuration
gmux sz remove ~/Code/special-project
```

## `gmux key`

Manages tmux session hotkey bindings.

### `gmux key list`

Lists configured session key bindings.

#### Syntax

```bash
gmux key list [--style <style>]
```

#### Description

Displays the mapping between hotkeys and projects from `tmux-sessions.yaml`. The default output is a table.

**Flags**:
*   `--style <style>`: Set the output format. Options are `table` (default) or `compact`.

#### Example

```bash
# Show all key mappings in a table
gmux key list

# Show only mapped keys in a compact format
gmux key list --style compact
```

### `gmux key manage` (alias: `km`)

Opens a TUI to manage session key bindings.

#### Syntax

```bash
gmux key manage
```

#### Description

Provides a terminal interface to view available keys, see which projects they are mapped to, and interactively map or unmap bindings. It includes a project search interface. Changes are saved to the configuration file on exit.

#### Example

```bash
# Open the interactive key manager
gmux key manage
```

### `gmux key add`

Maps a discovered project to an available key.

#### Syntax

```bash
gmux key add
```

#### Description

Starts an interactive workflow that shows a list of unmapped projects discovered from configured search paths, then prompts the user to select an available key to assign.

#### Example

```bash
# Start the interactive flow to add a new key mapping
gmux key add
```

### `gmux key unmap`

Unmaps a project from a key.

#### Syntax

```bash
gmux key unmap [key]
```

#### Description

Removes the project path mapping for a specific key, making the key available again. If `[key]` is not provided, it prompts for a selection from currently mapped keys.

#### Example

```bash
# Unmap the project assigned to the 'd' key
gmux key unmap d
```

### `gmux key update`

Changes the key for an existing session mapping.

#### Syntax

```bash
gmux key update [current-key]
```

#### Description

Prompts for a new, available key for an already mapped project. If `[current-key]` is omitted, it prompts for a selection of which mapping to update.

#### Example

```bash
# Change the key for the session currently mapped to 'a'
gmux key update a
```

### `gmux key edit`

Edits the path of a mapped session.

#### Syntax

```bash
gmux key edit [key]
```

#### Description

Allows changing the file path associated with a specific key. If a `[key]` is not provided, it prompts for a selection of which mapping to edit.

#### Example

```bash
# Edit the details for the session mapped to the 's' key
gmux key edit s
```

## `gmux launch`

Launches a new tmux session with specified options.

### Syntax

```bash
gmux launch <session-name> [--window-name <name>] [--working-dir <path>] [--pane <command>]...
```

### Description

Creates a new tmux session. It can specify the initial window name, a working directory for the session, and create multiple panes.

**Flags**:
*   `--window-name <name>`: Name for the initial window.
*   `--working-dir <path>`: Working directory for the new session.
*   `--pane <command>`: Adds a pane. Can be used multiple times. Supports `command@/path/to/workdir` syntax to set a working directory for a specific pane.

### Example

```bash
# Launch a session with two panes, each in a different directory
gmux launch my-app --pane "npm run dev@/app/frontend" --pane "go run .@/app/backend"
```

## `gmux session`

Performs direct operations on tmux sessions.

### `gmux session exists`

Checks if a tmux session exists.

#### Syntax

```bash
gmux session exists <session-name>
```

#### Description

Exits with code 0 if the session exists, and 1 if it does not. Intended for use in scripts.

#### Example

```bash
if gmux session exists my-session; then
  echo "Session is running."
fi
```

### `gmux session kill`

Terminates a tmux session.

#### Syntax

```bash
gmux session kill <session-name>
```

#### Example

```bash
# Kill the session named 'dev-old'
gmux session kill dev-old
```

### `gmux session capture`

Captures the visible content of a tmux pane and prints it to standard output.

#### Syntax

```bash
gmux session capture <target>
```

#### Description

The `<target>` can be a session name (`my-session`) or a specific pane target (`my-session:0.1`).

#### Example

```bash
# Capture the content of the first pane in the 'my-session' session
gmux session capture my-session:0.0
```

## `gmux status`

Shows the Git status for repositories configured in `tmux-sessions.yaml`.

### Syntax

```bash
gmux status
```

### Example

```bash
gmux status
```

## `gmux list`

Lists configured session key bindings. This is an alias for `gmux key list`.

### Syntax

```bash
gmux list [--style <style>]
```

### Example

```bash
# Show configured keys in a compact format
gmux list --style compact
```

## `gmux wait`

Blocks execution until a specified tmux session is closed.

### Syntax

```bash
gmux wait <session-name> [--timeout <duration>] [--poll-interval <duration>]
```

### Description

Polls at a regular interval to check if a session still exists. Exits with status 0 when the session closes. Exits with a non-zero status if a timeout is reached.

**Flags**:
*   `--timeout <duration>`: Maximum time to wait (e.g., `10m`, `30s`). `0s` means wait indefinitely.
*   `--poll-interval <duration>`: How often to check if the session exists (e.g., `1s`, `500ms`).

### Example

```bash
# Wait up to 5 minutes for the 'review-task' session to close
gmux wait review-task --timeout 5m
```

## `gmux start`

Starts a pre-configured tmux session by its hotkey.

### Syntax

```bash
gmux start <key>
```

### Description

Finds the project mapped to the given `<key>` in `tmux-sessions.yaml` and launches a new tmux session for it. The session is named `grove-<key>`.

### Example

```bash
# Start the session configured for the 'a' key
gmux start a
```

## `gmux version`

Prints version information for the `gmux` binary.

### Syntax

```bash
gmux version [--json]
```

### Flags

*   `--json`: Output version information in JSON format.

### Example

```bash
gmux version
```
