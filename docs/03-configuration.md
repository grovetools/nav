# Configuration

This document describes the configuration files used by `gmux`. Paths with a leading `~` are expanded to the user’s home directory.

By default, `gmux` looks for configuration files in `~/.config/grove`. This can be changed with the global `--config-dir` flag.

## project-search-paths.yaml

This file defines which directories `gmux` scans to find projects for the sessionizer (`gmux sz`). It also lists specific project directories to include.

**Location Resolution** (first match found is used):
1.  `<config-dir>/project-search-paths.yaml`
2.  `~/.config/tmux/project-search-paths.yaml`
3.  `~/.config/grove/project-search-paths.yaml`

### Schema

*   `search_paths` (map of string to object): Defines directories to scan for projects.
    *   `path` (string): Directory path.
    *   `description` (string, optional): A description for the path.
    *   `enabled` (bool): If `true`, this search path is used.
*   `explicit_projects` (array of objects): Defines specific project directories to include, regardless of their location.
    *   `path` (string): The path to the project.
    *   `name` (string, optional): A display name, which defaults to the directory name.
    *   `description` (string, optional): A description for the project.
    *   `enabled` (bool): If `true`, this project is included.
*   `discovery` (object): Contains settings for project discovery.
    *   `max_depth` (int): The maximum depth to search within each `path`.
    *   `min_depth` (int): The minimum depth to search.
    *   `exclude_patterns` ([]string): A list of directory name patterns to exclude from the search.

### Discovery Logic

The sessionizer discovers projects by:
1.  Including each enabled `search_path` directory itself as a project.
2.  Scanning immediate subdirectories of each enabled `search_path`.
3.  Excluding subdirectories with names matching `exclude_patterns`.
4.  If a discovered directory is a Git repository, `gmux` scans for a `.grove-worktrees` subdirectory and includes any directories within it as Git worktrees grouped under the parent repository.
5.  Including all enabled `explicit_projects`.

### Example

```yaml
# ~/.config/grove/project-search-paths.yaml

# Defines directories to scan for projects.
search_paths:
  work:
    path: ~/Work
    description: "Work projects"
    enabled: true
  personal:
    path: ~/Projects
    description: "Personal projects"
    enabled: true

# Controls the directory scanning process.
discovery:
  max_depth: 2
  min_depth: 0
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

# Defines specific project directories to always include.
explicit_projects:
  - path: ~/Code/dotfiles
    enabled: true
```

## tmux-sessions.yaml

This file defines the available hotkeys and maps them to project paths. It is read by `gmux key` commands and used to generate tmux bindings. The recommended way to modify this file is with the `gmux key manage` command.

**Location**:
*   `<config-dir>/tmux-sessions.yaml` (default: `~/.config/grove/tmux-sessions.yaml`)

### Schema

*   `available_keys` ([]string): A list of characters to be used as session hotkeys (e.g., `a`, `s`, `d`).
*   `sessions` (map of key to object): Defines the project mapped to each key. Unmapped keys from `available_keys` are present in memory but are not written to the `sessions` map in the file.
    *   `path` (string): The project path the key is mapped to.
    *   `repo` (string): The display name for the project.
    *   `description` (string, optional): A free-text description.
*   `tmux_sessionizer.script_path` (string, optional): Path to an external script to be called by the generated tmux bindings. If omitted, it defaults to `~/.local/bin/scripts/tmux-sessionizer`.

### Example

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
  # Maps key 'a' to the grove-tmux project.
  a:
    path: "~/Work/grove-tmux"
    repo: "grove-tmux"
  # Key 's' is available but currently unmapped.

# Optional: Override the script called by generated bindings.
tmux_sessionizer:
  script_path: ~/.local/bin/scripts/tmux-sessionizer
```
