# Configuration Reference

Create a detailed reference for the `gmux` configuration files.

## Task
Analyze `internal/manager/manager.go` and `cmd/gmux/sessionize.go` to document the configuration files.

1.  **`project-search-paths.yaml`**:
    - Explain the purpose of this file: to discover projects for the sessionizer.
    - Document the structure: `search_paths`, `explicit_projects`, and `discovery` settings (`max_depth`, `exclude_patterns`).
    - Provide a full, commented example.

2.  **`tmux-sessions.yaml`**:
    - Explain the purpose: to map projects to specific hotkeys.
    - Document the structure: `available_keys` and the `sessions` map.
    - Explain that this file is best managed via `gmux key manage`.

## Output Format
- Use H2 headings for each file name.
- Provide clear explanations and complete YAML examples.