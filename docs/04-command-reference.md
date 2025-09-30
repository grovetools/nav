This document provides a comprehensive reference for the `gmux` command-line interface, covering all subcommands and their options.

## `gmux sessionize` (alias: `sz`)

The interactive project switcher and sessionizer.

### Syntax

```bash
gmux sz [path]
```

### Description

Launches a live, interactive TUI (Terminal User Interface) to browse and switch between discovered projects. The TUI displays project names, paths, running tmux session indicators, Git status, and (if `grove-hooks` is installed) Claude AI session status. The view updates automatically every 10 seconds.

If an optional `[path]` argument is provided, it bypasses the TUI and directly creates or switches to a tmux session for that specific path.

### Example

```bash
# Open the interactive sessionizer TUI
gmux sz

# Directly create or switch to a session for a specific project
gmux sz ~/Work/grove-tmux
```

### `gmux sessionize add`

Adds a specific project path to the `explicit_projects` list in your configuration.

#### Syntax

```bash
gmux sz add [path]
```

#### Description

This command is for adding a project that is outside of your configured `search_paths`. If no `[path]` is given, it adds the current working directory.

#### Example

```bash
# Add the project located at ~/Code/special-project
gmux sz add ~/Code/special-project
```

### `gmux sessionize remove`

Removes a project from the `explicit_projects` list.

#### Syntax

```bash
gmux sz remove [path]
```

#### Description

Removes a previously added explicit project. If no `[path]` is given, it removes the current working directory.

#### Example

```bash
# Remove the project located at ~/Code/special-project
gmux sz remove ~/Code/special-project
```

## `gmux key`

Manages tmux session hotkey bindings.

### `gmux key list`

Lists all configured session key bindings.

#### Syntax

```bash
gmux key list [--style <style>]
```

#### Description

Displays the mapping between hotkeys and projects. The default output is a table.

**Flags**:
*   `--style <style>`: Set the output style. Options are `table` (default) or `compact`.

#### Example

```bash
# Show all key mappings in a table
gmux key list

# Show only mapped keys in a compact format
gmux key list --style compact
```

### `gmux key manage` (alias: `km`)

Opens an interactive TUI to manage session key bindings.

#### Syntax

```bash
gmux key manage
```

#### Description

Provides a full-screen interface to view all available keys, see which projects they are mapped to, and interactively map, unmap, or edit bindings using a fuzzy project search. Changes are saved automatically on exit.

#### Example

```bash
# Open the interactive key manager
gmux key manage
```

### `gmux key add`

Interactively maps a discovered project to an available key.

#### Syntax

```bash
gmux key add
```

#### Description

Starts a guided workflow that shows a list of unmapped projects discovered from your configured search paths, and then prompts you to select an available key to assign.

#### Example

```bash
# Start the interactive flow to add a new key mapping
gmux key add
```

### `gmux key unmap`

Unmaps a project from a key, making the key available again.

#### Syntax

```bash
gmux key unmap [key]
```

#### Description

If a `[key]` is provided, it unmaps it directly. If run without arguments, it interactively prompts you to choose which mapped key to unmap.

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

Prompts you to select a new, available key for an already mapped project. If `[current-key]` is omitted, it prompts you to select which mapping to update.

#### Example

```bash
# Change the key for the session currently mapped to 'a'
gmux key update a
```

### `gmux key edit`

Edits the details (like the path) of a mapped session.

#### Syntax

```bash
gmux key edit [key]
```

#### Description

Allows you to change the file path associated with a specific key. If run without a `[key]`, it prompts you to select which mapping to edit.

#### Example

```bash
# Edit the details for the session mapped to the 's' key
gmux key edit s
```

## `gmux launch`

Launches a new tmux session with advanced options.

### Syntax

```bash
gmux launch <session-name> [--window-name <name>] [--working-dir <path>] [--pane <command>]...
```

### Description

Creates a new tmux session. It can specify the initial window name, a working directory, and create multiple panes, each with an optional command and per-pane working directory.

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

Performs direct manipulation of tmux sessions.

### `gmux session exists`

Checks if a tmux session exists.

#### Syntax

```bash
gmux session exists <session-name>
```

#### Description

Exits with code 0 if the session exists, and 1 if it does not. Useful for scripting.

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

The `<target>` can be a session name (e.g., `my-session`) or a specific pane target (e.g., `my-session:0.1`).

#### Example

```bash
# Capture the content of the first pane in the first window of 'my-session'
gmux session capture my-session:0.0
```

## `gmux status`

Shows the Git status for all repositories configured in `tmux-sessions.yaml`.

### Syntax

```bash
gmux status
```

### Example

```bash
gmux status
```

## `gmux list`

Lists all configured session key bindings. This is an alias for `gmux key list`.

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

Useful for scripts that need to wait for a user to finish work in a temporary session.

**Flags**:
*   `--timeout <duration>`: Maximum time to wait (e.g., `10m`, `30s`). `0s` means wait forever.
*   `--poll-interval <duration>`: How often to check if the session still exists (e.g., `1s`, `500ms`).

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

Finds the project mapped to the given `<key>` in your `tmux-sessions.yaml` and launches a new tmux session for it. The session is named `grove-<key>`.

### Example

```bash
# Start the session configured for the 'a' key
gmux start a
```

## `gmux version`

Prints the version information for the `gmux` binary.

### Syntax

```bash
gmux version [--json]
```

### Flags

*   `--json`: Output the version information in JSON format.

### Example

```bash
gmux version
```