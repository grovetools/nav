# Configuration

This document describes the configuration files used by `gmux`. Paths with a leading `~` are expanded to the user's home directory.

By default, `gmux` looks for configuration files in `~/.config/grove`. This can be changed with the global `--config-dir` flag.

## grove.yml (Static Configuration)

`gmux` uses the unified Grove configuration system. Static tmux configuration is stored in the `tmux` section of `grove.yml`, which supports layered configuration (global, project, and override files).

**Location**: `~/.config/grove/grove.yml` (or project-specific `grove.yml` files)

The `tmux` section defines:
- Available hotkeys
- Project search paths
- Explicit projects to include
- Project discovery settings

**Note**: Session key mappings (which change frequently) are stored separately in `~/.config/grove/gmux/sessions.yml` to keep version-controlled config files clean.

### Schema

The `tmux` section in `grove.yml` contains:

*   `available_keys` ([]string): A list of characters to be used as session hotkeys (e.g., `a`, `s`, `d`).
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
# ~/.config/grove/grove.yml

version: "1.0"

# Tmux configuration (static settings only)
tmux:
  # Available hotkeys for session switching
  available_keys: [a, s, d, f, g, h, j, k, l]

  # Directories to scan for projects
  search_paths:
    work:
      path: ~/Work
      description: "Work projects"
      enabled: true
    personal:
      path: ~/Projects
      description: "Personal projects"
      enabled: true

  # Controls the directory scanning process
  discovery:
    max_depth: 2
    min_depth: 0
    exclude_patterns:
      - node_modules
      - .cache
      - target
      - build
      - dist

  # Specific project directories to always include
  explicit_projects:
    - path: ~/Code/dotfiles
      name: "dotfiles"
      enabled: true
```

## gmux/sessions.yml (Dynamic State)

This file stores session key mappings (which change frequently when you map/unmap projects). It is automatically managed by `gmux key` commands and is stored separately from `grove.yml` to avoid polluting version control.

**Location**: `~/.config/grove/gmux/sessions.yml`

The recommended way to modify this file is with the `gmux key manage` command, which provides an interactive interface for mapping projects to keys.

### Schema

*   `sessions` (map of key to object): Defines the project mapped to each key. Unmapped keys from `available_keys` are not written to this file.
    *   `path` (string): The project path the key is mapped to.
    *   `repository` (string): The display name for the project.
    *   `description` (string, optional): A free-text description.

### Example

```yaml
# ~/.config/grove/gmux/sessions.yml
sessions:
  # Maps key 'a' to the grove-tmux project
  a:
    path: "~/Work/grove-tmux"
    repository: "grove-tmux"
    description: ""

  # Maps key 'd' to another project
  d:
    path: "~/Projects/my-app"
    repository: "my-app"
    description: "My awesome application"

  # Keys 's', 'f', 'g', etc. are available but currently unmapped
```

### Version Control Recommendation

Add `gmux/sessions.yml` to your `.gitignore` to prevent session mappings from polluting your version control history:

```gitignore
# In ~/.config/grove/.gitignore
gmux/sessions.yml
gmux/access-history.json
```
