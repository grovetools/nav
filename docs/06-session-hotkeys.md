# Session Hotkeys (gmux key)

Session hotkeys let you map single-key bindings (for example, a, s, d, …) to specific projects. These mappings are stored in your sessions file (tmux-sessions.yaml), used to generate tmux bindings, and optionally reloaded into a running tmux session.

This page explains the interactive manager and each CLI subcommand under gmux key.

Notes
- Configuration directory: defaults to ~/.config/grove; override with --config-dir.
- Sessions file: defaults to <config-dir>/tmux-sessions.yaml; override with --sessions-file.
- Generated tmux bindings: written to <config-dir>/generated-bindings.conf. Source this file from your ~/.tmux.conf:
  tmux
  # ~/.tmux.conf
  source-file ~/.config/grove/generated-bindings.conf
- Available keys come from available_keys in tmux-sessions.yaml. You can only map these keys.

## Interactive Manager (gmux key manage)

gmux key manage opens a TUI to view and modify key-to-project mappings. It presents a table with:
- Key: the single-character binding
- Repository: the project name (usually the directory basename)
- Path: the full project path (empty if the key is currently unmapped)

Changes are saved back to tmux-sessions.yaml. The manager regenerates bindings and attempts to reload your tmux configuration automatically if you are inside a tmux session.

Launch:
```bash
gmux key manage
# or the alias:
gmux km
# with explicit config:
gmux key manage --config-dir ~/.config/grove
```

Navigation and actions
- Up/Down or k/j: move selection
- e: map/edit using fuzzy project search
  - Opens a filterable list of discovered projects (from project-search-paths.yaml)
  - Already-mapped projects are excluded from selection
  - Press Enter to assign the selected project to the highlighted key
- Space: quick toggle — unmap the selected key if it is currently mapped
- d or Delete: unmap the selected key
- o or Enter: switch to the selected mapped session (inside tmux). If the session does not exist, it is created first.
- s or Ctrl+s: save changes and exit (regenerate bindings and try to reload tmux config)
- q, Esc, or Ctrl+c: save-and-quit (same as above)
- ?: toggle inline help

Behavior
- Saves to tmux-sessions.yaml and regenerates <config-dir>/generated-bindings.conf.
- Attempts tmux source-file ~/.tmux.conf when inside tmux; prints a message if reload fails or if not in tmux.
- The fuzzy search lists projects discovered from the configured search paths and explicit projects; see project-search-paths.yaml.

Prerequisites
- Ensure you have a valid project-search-paths.yaml so the manager can discover projects to assign.
- Ensure available_keys is defined in tmux-sessions.yaml; only those keys can be mapped.

## CLI Subcommands

### gmux key list
List session hotkey mappings.

Output styles:
- Table (default): Key, Repository, Path
- Compact: key: repo for mapped keys only

Examples:
```bash
# Default table view
gmux key list

# Compact view (only mapped keys)
gmux key list --style compact

# With explicit config directory
gmux key list --config-dir ~/.config/grove --style table
```

Notes:
- Repository is taken from the sessions file when present; otherwise, the base of the path may be shown in compact mode.
- The same --style flag also works with the top-level alias gmux list.

### gmux key add
Interactively map a discovered project to a free key.

What it does:
- Loads available_keys and current mappings
- Shows only free keys
- Scans your configured search paths (project-search-paths.yaml)
- Lets you choose a project and then pick a free key
- Saves, regenerates bindings, and prints a reminder to reload tmux configuration

Examples:
```bash
# Start interactive add flow
gmux key add

# With explicit config
gmux key add --config-dir ~/.config/grove
```

If no projects are discovered, the command prints instructions to set up:
- ~/.config/grove/project-search-paths.yaml
- or ~/.config/tmux/project-search-paths.yaml

### gmux key unmap
Unmap a project from a key, making the key available again.

Usage:
- gmux key unmap [key]
- If no key is provided, the command lists mapped keys and prompts you interactively.

Examples:
```bash
# Unmap key 'a'
gmux key unmap a

# Interactive prompt to choose which key to unmap
gmux key unmap

# With explicit config
gmux key unmap a --config-dir ~/.config/grove
```

Behavior:
- Clears path/repository/description for the specified key
- Saves, regenerates bindings, and attempts to reload tmux config automatically if inside tmux

### gmux key update
Change which key a mapped session uses.

Usage:
- gmux key update [current-key]
- If current-key is not provided, you’ll be shown all sessions and prompted to pick a key to update.

Flow:
- Lists available (free) keys from available_keys
- Validates the new key is in available_keys and not already in use (unless you are reusing the same key)
- Updates the mapping, saves, regenerates bindings, and prints a reminder to reload tmux configuration

Examples:
```bash
# Change key 'a' to another available key
gmux key update a

# Interactive selection of the session to update
gmux key update

# With explicit config
gmux key update a --config-dir ~/.config/grove
```

### gmux key edit
Edit the details of a mapped session — specifically the path (repository name is derived from the path).

Usage:
- gmux key edit [key]
- If key is omitted, you’ll be shown a table and prompted to enter a key to edit.

Flow:
- Prompts for a new path (press Enter to keep current)
- Saves updates to tmux-sessions.yaml
- Regenerates bindings if the path changed, and prints a reminder to reload tmux configuration

Examples:
```bash
# Edit mapping for key 's'
gmux key edit s

# Interactive selection
gmux key edit

# With explicit config
gmux key edit s --config-dir ~/.config/grove
```

## Additional considerations

- Binding generation: gmux writes bindings to <config-dir>/generated-bindings.conf. The command regenerates this file whenever you add, update, edit, or unmap.
- Sourcing in tmux: Add source-file ~/.config/grove/generated-bindings.conf to your ~/.tmux.conf so the bindings are effective. The interactive manager and unmap command will try to reload tmux automatically when running inside tmux; other subcommands print a reminder to reload.
- Key space: Only keys listed under available_keys in tmux-sessions.yaml can be used. Edit this list directly if you need a different set of keys.
- Project discovery: The interactive manager and gmux key add depend on project-search-paths.yaml. Configure your search paths and explicit projects there so they appear for selection.