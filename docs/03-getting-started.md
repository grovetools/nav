# Getting Started

This guide walks you through the initial setup and first use of gmux.

Prerequisites
- tmux installed and available in your PATH
- gmux installed (from a release or built from source)

1. First-Run Setup (gmux sz)

On first run, gmux guides you through configuring where to find your projects. Start the sessionizer:

```bash
gmux sz
```

If gmux cannot find a project-search-paths.yaml file, it will prompt you to set up your project directories:

- Enter one or more directories you keep projects in (e.g., ~/Work, ~/Projects, ~/Code).
- If a directory does not exist, gmux can create it for you.
- Provide an optional description for each directory.
- When finished (press Enter on an empty prompt), gmux writes a configuration file:

```
~/.config/grove/project-search-paths.yaml
```

You can edit this file later. A typical file looks like:

```yaml
# ~/.config/grove/project-search-paths.yaml
search_paths:
  work:
    path: ~/Work
    description: "Work projects"
    enabled: true
  personal:
    path: ~/Projects
    description: "Personal projects"
    enabled: true

discovery:
  max_depth: 2
  min_depth: 0
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

explicit_projects: []
```

Notes
- gmux expands ~ to your home directory.
- The sessionizer will also discover Git worktrees under .grove-worktrees directories next to a repository.

2. Tmux Integration

gmux generates tmux key bindings from your session key mappings and writes them to:

```
~/.config/grove/generated-bindings.conf
```

Include this file from your tmux configuration:

```tmux
# ~/.tmux.conf
source-file ~/.config/grove/generated-bindings.conf
```

Reload tmux to apply the change (from inside tmux):

```bash
tmux source-file ~/.tmux.conf
```

Important details
- The generated-bindings.conf file is created/updated when you map or edit keys (via gmux key manage or inside the sessionizer with key editing).
- Do not edit generated-bindings.conf directly; update your mappings and regenerate instead.
- When gmux updates bindings, it attempts to reload your tmux config automatically if you are inside tmux. If not, run the source-file command above.

3. First Launch

Run the sessionizer to see your projects:

```bash
gmux sz
```

Basic usage
- Use the arrow keys to select a project and press Enter to create or switch to its tmux session.
- To assign a session hotkey while browsing:
  - Press Ctrl+E in the sessionizer to open key editing, then press a key to assign.
  - Alternatively, use the dedicated key manager:
    ```bash
    gmux key manage
    ```
  - After mapping keys, gmux regenerates bindings and, when possible, reloads tmux automatically.

Direct launch (optional)
If you already know the path, you can launch or switch directly:

```bash
gmux sz /path/to/project
```

That’s it. With your project paths configured, tmux sourcing the generated bindings, and keys assigned to your frequent projects, gmux provides a fast way to navigate and manage development sessions.