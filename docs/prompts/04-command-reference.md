# Command Reference

Generate a comprehensive command reference for `grove-tmux` (`gmux`).

## Requirements
Analyze the `cmd/gmux/` directory to document every command, subcommand, and flag.

### Commands to Document
- `gmux sessionize` (alias `sz`): The interactive project switcher.
- `gmux key` (and subcommands `list`, `manage`, `add`, `unmap`, `update`, `edit`): The session hotkey manager.
- `gmux launch`: Launching complex sessions with panes.
- `gmux session` (and subcommands `exists`, `kill`, `capture`): Direct session manipulation.
- `gmux status`: Viewing Git status for all configured projects.
- `gmux list`: A simple list of configured hotkeys.
- `gmux wait`: Waiting for a session to close.
- `gmux start`: Starting a pre-configured session by its key.
- `gmux version`: Displaying version info.

## Output Format
- Use H2 headings for each top-level command and H3 for subcommands.
- Provide syntax, a description, and a usage example for each.