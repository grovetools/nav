# Introduction

gmux is an interactive session manager for tmux. Inspired by ThePrimeagen’s tmux-sessionizer, it extends the idea with deeper, practical integration into the Grove ecosystem. gmux presents tmux as a context-aware development dashboard: it discovers projects, surfaces session/gIT activity, and provides fast, structured ways to switch, launch, and manage sessions.

By unifying project discovery, session state, Git information, and key-based navigation, gmux helps you move between work quickly and keep tmux layouts and sessions organized.

## What problem it solves

tmux is a capable multiplexer but offers limited visibility and navigation across many projects and sessions. gmux addresses this by:
- Discovering your projects and worktrees automatically
- Showing which sessions are running and which one you’re in
- Surfacing concise Git status for active sessions
- Mapping projects to hotkeys you can manage interactively
- Providing commands to launch, switch, and automate sessions consistently

The result is a live, navigable overview of your development environment, integrated with your configuration and tooling.

## Key features

- Live Sessionizer (gmux sz)
  - Automatic project discovery from configured search paths and explicit entries
  - Hierarchical Git worktree display, grouping worktrees under their parent repository
  - Real-time tmux session indicators:
    - ● for active sessions (blue for current, green for others)
  - Live Git status for active sessions with compact summaries:
    - Example: ↑1 ↓2 M:3 S:1 ?:5 +10 -4
  - Optional Claude session indicators (via grove-hooks): running (▶), idle (⏸), completed (✓), failed (✗)
  - Live refresh every 10 seconds without losing cursor or filter
  - Sorting that prioritizes active sessions and recent access
  - Direct path mode: gmux sz <path> to jump immediately

- Interactive Key Management (gmux key manage)
  - TUI to map/unmap/edit session hotkeys with live project search
  - Fuzzy project selection and quick toggling
  - Saves changes to tmux-sessions.yaml, regenerates tmux bindings, and attempts to reload tmux config

- CLI key tools (gmux key …)
  - gmux key list — show mappings (table or compact display)
  - gmux key add — add a project to a key from discovered projects
  - gmux key update — change the key for an existing mapping
  - gmux key edit — update a mapped project’s path
  - gmux key unmap — free up a key

- Advanced session control (gmux launch)
  - Launch sessions with optional window name, working directory, and multiple panes
  - Pane syntax supports per-pane working directories: command@/workdir
  - Works both inside and outside tmux

- Scripting and automation
  - gmux session exists|kill|capture — check, terminate, or capture pane contents
  - gmux wait — block until a session closes (useful in scripts)
  - gmux start — start a pre-configured session by key
  - gmux status — compact Git status summary for configured repositories

- Grove ecosystem integration
  - Configuration files under ~/.config/grove (with ~/.config/tmux as a fallback)
  - Auto-generated tmux bindings file to source from your tmux config
  - Optional grove-hooks integration to surface Claude session state in the sessionizer

Example commands:
- Open the live sessionizer: gmux sz
- Manage keys interactively: gmux key manage
- Launch a session with panes: gmux launch dev --pane "vim" --pane "go test -v"
- Start a configured session by key: gmux start a

These capabilities combine to make tmux session navigation faster, more consistent, and better aligned with how projects are organized and used.