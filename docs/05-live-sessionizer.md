# Live Sessionizer (gmux sz)

The gmux sessionizer is an interactive TUI for creating and switching tmux sessions from your project directories. It provides live visibility into running sessions, Git status, and (optionally) active Claude AI sessions. The interface updates automatically every 10 seconds without losing your cursor position or filter text.

This guide explains the UI and all interactive key bindings.

## What Updates Live

Every ~10 seconds, the sessionizer refreshes:

- Project discovery in your configured search paths
- Tmux session indicators (●) for running sessions
- Key mappings from tmux-sessions.yaml
- Git status for all running tmux sessions
- Claude AI session status (when grove-hooks is installed)

The TUI only updates when data changes to avoid flicker. Updates run in the background and do not interrupt navigation.

## UI Breakdown

The sessionizer screen consists of:

- A filter input at the top
- A scrolling list of projects and worktrees
- A help/status footer with key hints
- Search path summary at the bottom

Example of a single line in the list (spacing simplified for clarity):

  a ● ▶ feat-branch  /home/user/Work/repo/.grove-worktrees/feat-branch  ↑1 M:3 S:1 +10 -4

Breakdown of elements:

- Key mapping: the first token (e.g., a) shows the assigned hotkey. If a project has no assigned key, two spaces are reserved to keep alignment.
- Session indicator (●):
  - ● in green: the project’s tmux session is running
  - ● in blue: this is the current tmux session
  - Shown only when running inside tmux
- Claude AI status (optional): a single symbol next to the session indicator when grove-hooks is installed:
  - ▶ running
  - ⏸ idle
  - ✓ completed
  - ✗ failed/error
- Worktree prefix:
  - └─ indicates an entry is a Git worktree under a parent repository
- Project name and path:
  - Name is styled; path is shown in a subdued color
- Git status summary (for running sessions only):
  - Upstream: ↑N (ahead), ↓N (behind)
  - Working tree: M:N modified, S:N staged, ?:N untracked
  - Line stats: +N added, -N deleted (staged and unstaged combined)
  - A clean repo with upstream configured is shown as ✓
- Claude duration (optional): displayed at the end when available

Notes and behavior:

- Default view (no filter):
  - Parent repositories are listed
  - Worktrees are shown only if their session is running (keeps the list focused)
  - Groups (repo + its worktrees) with active sessions are prioritized
- Filtered view (typing in the filter):
  - All matching repositories and worktrees are shown
  - Results are ordered by match quality: exact name, prefix, name contains, then path contains
  - Groups with running sessions appear before inactive groups

Session naming: When launching or switching, the session name is derived from the project directory basename with dots replaced by underscores (e.g., repo.name → repo_name).

Git and tmux awareness:

- Running session detection and Git status updates are active when inside tmux (TMUX is set). Outside tmux, these indicators are omitted, but you can still select a project to open a session.

## Key Bindings

The sessionizer supports two modes: normal mode and key-editing mode. The help footer shows common shortcuts.

### Normal Mode

- Navigation
  - Up/Down or k/j: move selection
  - Ctrl+P / Ctrl+N: move selection (alternative)
- Select
  - Enter: create or switch to a tmux session for the selected project
    - Outside tmux: creates a new session and attaches
    - Inside tmux: creates the session if needed and switches client
- Edit key mapping
  - Ctrl+E: enter key-editing mode for the selected project
- Clear key mapping
  - Ctrl+X: remove the project’s key assignment (keeps the key slot open)
- Copy path
  - Ctrl+Y: copy the selected project path to clipboard
    - macOS: uses pbcopy
    - Linux: uses xclip (clipboard selection) or xsel (clipboard) if available
- Close session
  - Ctrl+D: kill the tmux session for the selected project (if it exists)
    - If you are currently attached to that session, the sessionizer tries to switch you to another session first to avoid abruptly detaching
- Quit
  - Esc or Ctrl+C: exit the sessionizer

### Key-Editing Mode (opened by Ctrl+E)

This mode lets you assign a hotkey to the selected project.

- Navigation
  - Up/Down: move through available keys
- Assign by selection
  - Enter: assign the currently highlighted key to the selected project
- Assign by direct key input
  - Press the desired key (e.g., a, s, d) to assign it immediately (if valid)
- Cancel
  - Esc: return to normal mode without changes

Behavior and persistence:

- Assigning a key updates tmux-sessions.yaml, regenerates ~/.config/grove/generated-bindings.conf, and attempts to reload your tmux config
- Clearing a key removes the mapping for that project and leaves the key unassigned
- If assigning a key already used by another project, the key is moved to the selected project (the previous project loses that mapping)

## Filtering and Ordering

- Type in the top input to filter by name or path
- Match priority: exact name > prefix > name contains > path contains
- Default view (no filter) emphasizes projects with active sessions and hides inactive worktrees
- The list preserves cursor position during live updates when possible

## Live Update Mechanics (for reference)

- The interface schedules periodic updates (every ~10s) to:
  - Re-scan projects from configured search paths
  - Refresh tmux running session list
  - Reload key mappings from tmux-sessions.yaml
  - Fetch Git status for all running sessions
  - Fetch Claude session status (via grove-hooks)
- Updates only apply if there are actual changes (reduces redraws and flicker)

## Practical Tips

- Claude indicators require grove-hooks to be installed and available on PATH (or in ~/.grove/bin).
- Git status and running session indicators are shown only when inside tmux.
- Session names are derived from directory names; avoid collisions by using distinct basenames.
- Use gmux key manage for a full-screen manager focused on key mapping across many projects.
- To open a specific path directly (bypassing TUI), you can run: gmux sz /path/to/project

## Summary of Key Bindings

Normal mode:
- Up/Down, Ctrl+P/Ctrl+N: navigate
- Enter: open/switch session
- Ctrl+E: edit key mapping
- Ctrl+X: clear key mapping
- Ctrl+Y: copy path
- Ctrl+D: close session
- Esc/Ctrl+C: quit

Key-editing mode:
- Up/Down: navigate keys
- Enter: assign highlighted key
- Press key directly: assign immediately
- Esc: cancel and return to normal mode

This covers the live sessionizer behavior and interactions so you can efficiently navigate projects, manage hotkeys, and monitor session and repository state from within tmux.