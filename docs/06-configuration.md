## Tmux Settings

| Property | Description |
| :--- | :--- |
| `available_keys` | (array of strings, optional) <br> Defines the specific set of single-character keys available for assignment as session hotkeys. By customizing this list, you can restrict the pool of keys used for quick-switching between projects to a specific set that suits your workflow (e.g., home row keys only). |
| `show_child_processes` | (boolean, optional) <br> Controls whether the interactive window selector attempts to detect and display the active child process running within a window's pane. When enabled, the interface will show specific running commands (like `vim` or `node`) rather than just the shell name, providing better context for identifying windows. |

```toml
[tmux]
available_keys = ["a", "s", "d", "f", "j", "k", "l", ";"]
show_child_processes = true
```