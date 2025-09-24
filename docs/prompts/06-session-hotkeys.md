# Session Hotkeys (gmux key)

## Task
Document how to manage project-to-key bindings. Focus on the `gmux key` command and its subcommands. Analyze `cmd/gmux/key.go` and `cmd/gmux/key_manage.go`.

1.  **Interactive Manager (`gmux key manage`)**: Describe the TUI for managing keys. Explain how to navigate, map projects with fuzzy search (`e`), and unmap keys (`d`/`del`).
2.  **CLI Subcommands**: Document the purpose and usage of each subcommand:
    - `gmux key list`: Show all bindings.
    - `gmux key add`: Interactively map a project to a key.
    - `gmux key unmap`: Unmap a project.
    - `gmux key update`: Change a project's assigned key.
    - `gmux key edit`: Edit a mapped project's path.

## Output Format
Provide clean Markdown with an H1 title. Use H2 for the interactive manager and H3 for each CLI subcommand. Include command examples.