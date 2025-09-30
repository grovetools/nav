This document describes the configuration files used by `gmux`, where they are located, and how `gmux` interprets them. Paths with a leading `~` are expanded to the user’s home directory.

By default, `gmux` looks for configuration under `~/.config/grove`. You can change this with the global flag `--config-dir` on any `gmux` command. Some files support fallback locations as noted below.

## project-search-paths.yaml

**Purpose**: This file defines where `gmux` discovers projects for the sessionizer (`gmux sz`) and lists any projects that should be included explicitly.

**Location Resolution** (first match wins):
1.  `<config-dir>/project-search-paths.yaml`
2.  `~/.config/tmux/project-search-paths.yaml`
3.  `~/.config/grove/project-search-paths.yaml`

### Schema

*   `search_paths` (map of string to object):
    *   `path` (string): Directory to scan for projects.
    *   `description` (string, optional): A description for the path.
    *   `enabled` (bool): Whether to include this search path.
*   `explicit_projects` (array of objects): Projects to include even if they are not under a search path.
    *   `path` (string): The path to the project.
    *   `name` (string, optional): A display name; defaults to the directory name.
    *   `description` (string, optional): A description for the project.
    *   `enabled` (bool): Whether to include this project.
*   `discovery` (object): Controls for how projects are discovered.
    *   `max_depth` (int): The maximum depth to search within each path.
    *   `min_depth` (int): The minimum depth to search.
    *   `exclude_patterns` ([]string): A list of directory name patterns to exclude from the search.

### Discovery Logic

The sessionizer discovers projects as follows:
1.  Each enabled search path is included as a project itself.
2.  Immediate subdirectories of each search path are included as projects.
3.  If a discovered project is a Git repository, `gmux` also finds any Git worktrees located in a `.grove-worktrees` subdirectory and groups them under the parent repository.
4.  All enabled `explicit_projects` are included.

### Example

```yaml
# ~/.config/grove/project-search-paths.yaml

# Search paths define the directories to scan for projects.
search_paths:
  work:
    path: ~/Work
    description: "Work projects"
    enabled: true
  personal:
    path: ~/Projects
    description: "Personal projects"
    enabled: true

# Discovery settings control the scanning process.
discovery:
  max_depth: 2
  min_depth: 0
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

# Explicit projects are always included, regardless of search paths.
explicit_projects:
  - path: ~/Code/dotfiles
    enabled: true
```

## tmux-sessions.yaml

**Purpose**: This file defines the pool of available hotkeys and maps specific keys to project paths. It is the source of truth for the `gmux key` commands and the generated tmux bindings. While it can be edited manually, using `gmux key manage` is the recommended way to modify it.

**Location**:
*   `<config-dir>/tmux-sessions.yaml` (default: `~/.config/grove/tmux-sessions.yaml`)

### Schema

*   `available_keys` ([]string): A list of all characters you want to use as session hotkeys (e.g., `a`, `s`, `d`, `f`).
*   `sessions` (map of key to object): Defines the project mapped to each key. Unmapped keys from `available_keys` are not present in this map.
    *   `path` (string): The project path that the key is mapped to.
    *   `repo` (string): The display name for the repository.
    *   `description` (string, optional): A free-text description.

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
  # Map key 'a' to the grove-tmux project
  a:
    path: "~/Work/grove-tmux"
    repo: "grove-tmux"
  # Key 's' is available but currently unmapped
```