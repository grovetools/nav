# Examples and Workflows

Generate practical examples for `grove-tmux` (`gmux`).

## Requirements
Create examples that cover the main workflows, integrating key features.

1.  **Example 1: The Live Sessionizer (`gmux sz`)**:
    - Provide a detailed walkthrough of the `gmux sz` TUI.
    - Explain each UI element: session indicators (`●`), key bindings, Git status, and Claude AI status (`▶`).
    - Show how to navigate, filter, and select a project to sessionize.

2.  **Example 2: Managing Session Hotkeys (`gmux key manage`)**:
    - Provide a walkthrough of the `gmux key manage` TUI.
    - Show how to map a project to an available key using fuzzy search.
    - Show how to unmap a key.
    - Explain that changes are saved automatically on exit.

3.  **Example 3: Scripting with gmux**:
    - Show a simple shell script example combining `gmux launch` and `gmux wait` to set up a development environment and wait for a task to complete.