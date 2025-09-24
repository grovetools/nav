# Advanced Topics

## Task
Document advanced usage patterns and technical details for power users.

1.  **Scripting with gmux**: Provide examples of how `gmux session`, `gmux launch`, and `gmux wait` can be combined in shell scripts to automate development environment setup.
2.  **Live Update Architecture**: Briefly explain how the live sessionizer works. Reference `cmd/gmux/sessionize.go` and describe the use of `tea.Tick` and background commands (`fetchGitStatusCmd`, `fetchClaudeSessionsCmd`, etc.) to refresh data every 10 seconds.

## Output Format
Provide clean Markdown with an H1 title.