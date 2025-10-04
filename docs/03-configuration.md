# Configuration

This document describes the configuration files used by `gmux`. Paths with a leading `~` are expanded to the user's home directory.

By default, `gmux` looks for configuration files in `~/.config/grove`. This can be changed with the global `--config-dir` flag.

## grove.yml (Static Configuration)

`gmux` uses the unified Grove configuration system. Configuration is stored in `grove.yml`, which supports layered configuration (global, project, and override files).

**Location**: `~/.config/grove/grove.yml` (or project-specific `grove.yml` files)

**Note**: Session key mappings (which change frequently) are stored separately in `~/.config/grove/gmux/sessions.yml` to keep version-controlled config files clean.

### Project Discovery

**As of this version**, project discovery is managed centrally by `grove-core`'s DiscoveryService. Projects are discovered from paths defined in the global `groves` configuration section.

The `groves` section in your global `~/.config/grove/grove.yml` defines:
- Root directories to search for projects and ecosystems

The `tmux` section defines:
- Available hotkeys for session switching

### Schema

**Global Configuration** (`~/.config/grove/grove.yml`):

*   `groves` (map of string to object): Defines root directories to scan for projects.
    *   `path` (string): Directory path to scan.
    *   `description` (string, optional): A description for the grove.
    *   `enabled` (bool): If `true`, this path is scanned for projects.

*   `explicit_projects` (array of objects): Defines specific projects to include without discovery.
    *   `path` (string): The path to the project.
    *   `name` (string, optional): A display name, defaults to the directory name.
    *   `description` (string, optional): A description for the project.
    *   `enabled` (bool): If `true`, this project is included.

*   `tmux` (object): Tmux-specific configuration.
    *   `available_keys` ([]string): A list of characters to be used as session hotkeys (e.g., `a`, `s`, `d`).

### Discovery Logic

The sessionizer discovers projects using `grove-core`'s DiscoveryService:
1.  Scans all enabled `groves` paths for projects with `grove.yml` files.
2.  Identifies ecosystems (directories with `grove.yml` containing a `workspaces` key).
3.  Discovers projects (directories with `grove.yml` but no `workspaces` key).
4.  For each project, scans for `.grove-worktrees` subdirectories and includes worktrees hierarchically.
5.  Includes non-Grove directories (directories with `.git` but no `grove.yml`).
6.  Adds all enabled `explicit_projects` (useful for including specific directories like dotfiles).

### Migration from Old Configuration

**If you previously used `search_paths`, `explicit_projects`, or `discovery` settings**, these are now deprecated. Migrate to the new `groves` configuration:

**Old configuration** (deprecated):
```yaml
tmux:
  search_paths:
    work:
      path: ~/Work
      enabled: true
```

**New configuration**:
```yaml
groves:
  work:
    path: ~/Work
    enabled: true
    description: "Work projects"
```

### Example

```yaml
# ~/.config/grove/grove.yml

version: "1.0"

# Project discovery paths (global configuration)
groves:
  work:
    path: ~/Work
    description: "Work projects"
    enabled: true
  personal:
    path: ~/Projects
    description: "Personal projects"
    enabled: true

# Explicit projects (specific directories to include)
explicit_projects:
  - path: ~/.config
    name: "dotfiles"
    description: "Configuration files"
    enabled: true

# Tmux configuration (static settings only)
tmux:
  # Available hotkeys for session switching
  available_keys: [a, s, d, f, g, h, j, k, l]
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
