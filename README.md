<p align="center">
  <img src="https://grovetools.ai/docs/nav/images/nav-logo-with-text-dark.svg" alt="Grove Nav">
</p>

<!-- DOCGEN:OVERVIEW:START -->

`nav` is a command-line tool for managing tmux sessions, windows, and navigation between project directories within the Grove ecosystem.

### Features

*   **Project Sessionizer (`nav sessionize` or `sz`)**: A terminal interface that lists projects discovered via grove `core`. It aggregates metadata including Git status, `nb` note counts, `flow` plan statistics, and `cx` token counts. Supports filtering, ecosystem focusing (`@`), and hierarchical worktree display.
*   **Key Mapping (`nav key manage` or `km`)**: A terminal interface for assigning single-character hotkeys to specific project paths. These mappings generate global tmux bindings for rapid access.
*   **Session History (`nav history` or `h`)**: A terminal interface listing accessed project sessions, sorted by recency. The `nav last` (or `l`) command switches to the most recently used session without opening the interface.
*   **Window Management (`nav windows`)**: A terminal interface for identifying and manipulating windows within the current session. Features include filtering by name or process, renaming, closing, and moving windows, with a pane content preview.
*   **Hotkey Generation**: Generates a tmux configuration file containing `bind-key` commands for mapped projects.
*   **Session Automation (`nav launch`, `nav wait`)**: Commands to create sessions with specific layouts/panes or block execution until a session closes.

### How It Works

**Discovery & Configuration**
`nav` utilizes the `DiscoveryService` from `core` to locate projects based on the `groves` configuration in `~/.config/grove/grove.yml`. It supports standard repositories, ecosystems, and Git worktrees.

**Data & Caching**
*   **Static Configuration**: Reads search paths and settings from `~/.config/grove/grove.yml`.
*   **Session State**: Key mappings are stored in `~/.local/state/nav/sessions.yml` (platform dependent).
*   **Cache**: Project metadata (Git status, note counts) is cached in `~/.cache/nav/cache.json` to improve startup performance.
*   **Enrichment**: The sessionizer executes background subprocesses (`git`, `nb`, `cx`, `grove`) to populate status columns asynchronously.

**Tmux Integration**
*   **Bindings**: `nav` generates `~/.cache/nav/generated-bindings.conf`. Users source this file in `~/.tmux.conf`.
*   **Hooks**: The generated configuration includes a `client-session-changed` hook that executes `nav record-session`, maintaining the access history.
*   **Execution**: Operations interact with the tmux server via the `tmux` binary. The tool respects the `GROVE_TMUX_SOCKET` environment variable for socket isolation.

### Installation

Install via the Grove meta-CLI:

```bash
grove install nav
```

Verify installation:

```bash
nav version
```

### Usage Examples

**Interactive Sessionizer**
```bash
# Open project picker
nav sz

# Open project picker focused on current working directory's ecosystem
nav sz .
```

**Key Management**
```bash
# Open interactive key manager
nav km

# Map current directory to 'w' key
nav key update w
```

### Limitations

*   **Tmux Dependency**: Core functionality requires a running tmux server.
*   **Terminal Support**: TUI features require a terminal emulator with support for standard ANSI escape sequences.
*   **Enrichment Performance**: Metadata columns (e.g., Git status, `nb` counts) rely on external binaries; performance depends on the execution speed of these underlying tools.

<!-- DOCGEN:OVERVIEW:END -->


<!-- DOCGEN:TOC:START -->

See the [documentation](docs/) for detailed usage instructions:
- [Overview](docs/01-overview.md)
- [Examples](docs/02-examples.md)
- [Command Reference](docs/04-command-reference.md)
- [Configuration](docs/06-configuration.md)

<!-- DOCGEN:TOC:END -->
