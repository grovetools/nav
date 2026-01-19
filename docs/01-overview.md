
<!-- placeholder for animated gif -->

*   **Session Interface (`gmux sz`)**: A terminal interface that lists projects discovered from configured search paths. The list displays active `tmux` sessions and Git status for those sessions, and refreshes its data sources periodically.
*   **Key Mapping (`gmux key manage`)**: A terminal interface to map project paths to single-character keys. It includes a fuzzy finder to search for unmapped projects from the discovered list.
*   **Hotkey Generation**: Creates a `tmux` configuration file with `bind-key` commands for each mapped project. This allows switching to projects using a keyboard shortcut from within `tmux`.
*   **Session Creation (`gmux launch`)**: A command to create `tmux` sessions with specified window names, a session-wide working directory, and multiple panes. Working directories can also be set on a per-pane basis.
*   **Scripting Commands**: Subcommands (`session exists`, `session kill`, `wait`) for checking and controlling `tmux` session state from shell scripts.
*   **Worktree Discovery**: Finds Git worktrees located in `.grove-worktrees` subdirectories and lists them hierarchically under their parent repository in the sessionizer interface.

## Ecosystem Integration

`gmux` uses other components of the Grove ecosystem to function.

*   **Configuration**: It reads configuration files from the `~/.config/grove/` directory.
*   **`grove-hooks` Execution**: If the `grove-hooks` binary is found in the `PATH`, `gmux` executes it to fetch and display the status of active Claude AI sessions.
*   **`grove` Meta-CLI**: The `grove` meta-CLI is the intended tool for installing and managing the `gmux` binary.

## How It Works

The sessionizer (`gmux sz`) is a terminal application that runs background commands every 10 seconds to gather information.

It executes `tmux` commands to list running sessions and find their working directories. For each active session in a Git repository, it runs `git` commands to get the branch status, file counts, and line changes. It also re-scans project directories defined in `project-search-paths.yaml` and reloads key mappings from `tmux-sessions.yaml`. The terminal interface redraws only if the fetched data differs from its current state.

When key mappings are changed, `gmux` updates `tmux-sessions.yaml` and regenerates a bindings file (`generated-bindings.conf`). It then attempts to execute `tmux source-file` to apply the changes in the current `tmux` server.

## Installation

Install via the Grove meta-CLI:
```bash
grove install tmux
```

Verify installation:
```bash
gmux version
```

Requires the `grove` meta-CLI. See the [Grove Installation Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md) if you don't have it installed.
