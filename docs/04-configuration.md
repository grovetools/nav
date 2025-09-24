# Configuration Reference

This document describes the configuration files used by gmux, where they are located, and how gmux interprets them. Paths with a leading ~ are expanded to the user’s home directory.

By default, gmux looks for configuration under ~/.config/grove. You can change this with the global flag --config-dir on any gmux command. Some files support fallback locations as noted below.

## project-search-paths.yaml

Purpose: Define where gmux discovers projects for the sessionizer (gmux sz), and list any projects that should be included explicitly.

Location resolution (first match wins):
- <config-dir>/project-search-paths.yaml
- ~/.config/tmux/project-search-paths.yaml
- ~/.config/grove/project-search-paths.yaml

Schema
- search_paths (map of string -> object)
  - path (string): Directory to scan for projects. The directory itself is included as a project if it exists.
  - description (string, optional): Free-text description.
  - enabled (bool): Enable or disable this search path.
- explicit_projects (array of objects): Projects that should be included even if they are not under any search path.
  - path (string): Absolute or ~-prefixed path to the project.
  - name (string, optional): Display name. If omitted, the directory name is used.
  - description (string, optional)
  - enabled (bool): Include/exclude this project.
- discovery (object): Future-facing scan controls.
  - max_depth (int): Intended maximum scan depth.
  - min_depth (int): Intended minimum scan depth.
  - file_types ([]string): Intended markers (e.g., .git, go.mod).
  - exclude_patterns ([]string): Intended directory name patterns to exclude.
  Notes:
  - Current discovery behavior scans the search path itself plus its immediate subdirectories, and also detects Git worktrees under <repo>/.grove-worktrees. The discovery fields are present for configuration stability and may not all be actively enforced yet.

How discovery works today
- Each enabled search path is included as a project (if it exists).
- Immediate subdirectories (excluding dot-prefixed directories) are included as projects.
- If a subdirectory is a Git repository, gmux also discovers worktrees in <repo>/.grove-worktrees and lists them under the parent repository.
- explicit_projects are always included (if enabled), regardless of search_paths.

Example
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

  experiments:
    path: ~/Code
    description: "Code experiments and learning"
    enabled: false

discovery:
  # These options are present for completeness. Not all are actively enforced yet.
  max_depth: 2
  min_depth: 0
  file_types:
    - .git
    - go.mod
    - package.json
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

explicit_projects:
  - path: ~/important-project
    name: "Important Project"
    description: "Explicitly added project that lives elsewhere"
    enabled: true
```

Notes
- You can add/remove explicit projects via:
  - gmux sessionize add [path]
  - gmux sessionize remove [path]
- Worktrees: Any directories under <repo>/.grove-worktrees are surfaced as worktrees and shown under their parent repository in the sessionizer.

## tmux-sessions.yaml

Purpose: Define the pool of available hotkeys and the mapping from keys to projects. Used by gmux key and for generating tmux key bindings.

Location:
- <config-dir>/tmux-sessions.yaml (default ~/.config/grove/tmux-sessions.yaml)

Schema
- available_keys ([]string): The list of allowed keys you want to use for sessions. Typically single characters like a, s, d, f, ...
- sessions (map: key -> object): Only keys that are currently mapped appear here. Unmapped keys stay in available_keys but are omitted from sessions.
  - path (string): Project path (supports ~). If empty, the key is considered unmapped.
  - repo (string): Display name. Often the repository name. Some commands will derive a name from the path if not provided.
  - description (string, optional): Free-text description.
- Optional advanced setting:
  - tmux_sessionizer.script_path (string): Path to an external “sessionizer” script that will be invoked by generated tmux bindings with the project path as its sole argument. Default is ~/.local/bin/scripts/tmux-sessionizer.

Behavior notes
- gmux reads available_keys to know which key slots exist.
- Mapping/unmapping keys through gmux key manage, gmux key add, gmux key unmap, or gmux key update updates this file and regenerates tmux bindings.
- Paths are expanded (~ is supported). Commands that rely on a working directory will skip entries with missing or invalid paths.
- Some features (like gmux status) only consider sessions with both path and repo set.

Example
```yaml
# ~/.config/grove/tmux-sessions.yaml

available_keys:
  - a
  - s
  - d
  - f
  - g
  - h
  - j
  - k
  - l

sessions:
  a:
    path: ~/Work/grove-tmux
    repo: grove-tmux
    description: "Grove tmux integration"

  s:
    path: ~/Projects/dotfiles
    repo: dotfiles

# Optional: override the script used by generated tmux bindings
tmux_sessionizer:
  script_path: ~/.local/bin/scripts/tmux-sessionizer
```

About tmux_sessionizer.script_path
- The generated tmux key bindings run: <script_path> <project-path>.
- Ensure the script exists and is executable, or configure script_path accordingly.
- A minimal wrapper script compatible with gmux could look like:
  ```bash
  #!/usr/bin/env bash
  # ~/.local/bin/scripts/tmux-sessionizer
  set -euo pipefail
  gmux sz "$1"
  ```
  Make it executable: chmod +x ~/.local/bin/scripts/tmux-sessionizer

## generated-bindings.conf

Purpose: The tmux bindings file generated from tmux-sessions.yaml. It contains bind-key entries that call the sessionizer script for each mapped key.

Location:
- <config-dir>/generated-bindings.conf (default ~/.config/grove/generated-bindings.conf)

Ownership
- This file is auto-generated by gmux. Do not edit it manually—changes will be overwritten.
- Edit tmux-sessions.yaml (or use gmux key … commands) instead; gmux will regenerate this file.

Activation
- Source the generated file from your tmux config:
  ```
  # ~/.tmux.conf
  source-file ~/.config/grove/generated-bindings.conf
  ```
- gmux attempts to reload tmux configuration automatically after changes if it is running inside tmux. If not, reload manually.

Contents
- For each mapped key, gmux writes a line similar to:
  ```
  bind-key -r a run-shell "~/.local/bin/scripts/tmux-sessionizer ~/Work/grove-tmux"
  ```
  The script path is controlled by tmux_sessionizer.script_path in tmux-sessions.yaml (with the default shown above).

Practical workflow
- Define the keys you want to use in available_keys.
- Map projects to keys via:
  - Interactive TUI: gmux key manage
  - CLI: gmux key add, gmux key unmap, gmux key update, gmux key edit
- gmux regenerates generated-bindings.conf and optionally reloads tmux.
- Use your configured keys in tmux to switch to projects via the sessionizer script.