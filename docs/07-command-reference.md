# gmux Command Reference

This document covers the remaining gmux CLI commands. Each section explains the purpose, arguments, flags, and provides usage examples.

Global options
- --config-dir string: Configuration directory (default: $HOME/.config/grove)
- --sessions-file string: Sessions file path (default: <config-dir>/tmux-sessions.yaml)

Note: Some commands (e.g., launch, session, wait) do not use the configuration files, but they still accept the global flags via Cobra.

## gmux launch

Launch a new tmux session, optionally naming the first window, setting a working directory, and creating panes with commands.

Usage
- gmux launch <session-name> [flags]

Flags
- --window-name string: Name for the initial window
- --working-dir string: Working directory for the session
- --pane string: Add a pane with a command; can be repeated. Format: "command[@workdir]"

Notes
- Each --pane can specify a per-pane working directory by appending @/path/to/dir.
- The command prints instructions to attach after creation; it does not auto-attach.

Examples
- Simple session:
  ```
  gmux launch dev-session
  ```
- With window name and working directory:
  ```
  gmux launch dev-session --window-name coding --working-dir ~/Projects/app
  ```
- Multiple panes:
  ```
  gmux launch dev-session --pane "vim main.go" --pane "go test -v" --pane "htop"
  ```
- Panes with per-pane workdirs:
  ```
  gmux launch dev-session --pane "npm run dev@/app/frontend" --pane "go run .@/app/backend"
  ```

## gmux session

Manage tmux sessions. Subcommands: exists, kill, capture.

### gmux session exists

Check if a tmux session exists.

Usage
- gmux session exists <session-name>

Behavior and exit codes
- Prints “Session '<name>' exists” and exits 0 if found.
- Prints “Session '<name>' does not exist” and exits 1 if not found.

Example
```
gmux session exists my-session
```

### gmux session kill

Kill a tmux session.

Usage
- gmux session kill <session-name>

Example
```
gmux session kill my-session
```

### gmux session capture

Capture content from a tmux pane or session.

Usage
- gmux session capture <target>

Target formats
- Session: session-name
- Window: session-name:0
- Pane: session-name:0.1
- Any tmux target string accepted by capture-pane

Example
```
gmux session capture my-session:0.0
```

## gmux status

Show Git status for repositories configured in tmux-sessions.yaml. This reads sessions from the configured sessions file and prints a table of Repository and Status.

Usage
- gmux status [--config-dir ...] [--sessions-file ...]

Output
- Compact status string per repository. Typical tokens include:
  - ✓: clean
  - ?N: N untracked files
  - MN: N modified files
  - ●N: N staged files
  - ↑N / ↓N: ahead/behind upstream
  - ⇡N / ⇣N: ahead/behind main (or master) branch
  - +N / -N: total added/deleted lines (from diff and staged diff)

Example
```
gmux status --config-dir ~/.config/grove
```

## gmux wait

Block until a tmux session closes. Useful for scripting.

Usage
- gmux wait <session-name> [flags]

Flags
- --poll-interval duration: How often to check (default: 1s)
- --timeout duration: Maximum time to wait (0 = no timeout; default: 0s)

Behavior and exit codes
- Prints progress and exits 0 when the session closes.
- Exits non-zero on timeout or errors.

Examples
```
# Wait indefinitely
gmux wait my-session

# Wait up to 30s, polling every 500ms
gmux wait my-session --timeout 30s --poll-interval 500ms
```

## gmux start

Start a pre-configured tmux session by key using tmux-sessions.yaml.

Usage
- gmux start <key> [--config-dir ...] [--sessions-file ...]

Behavior
- Session name: grove-<key>
- Working directory: uses the session’s path; if not set and a repository name exists, falls back to ~/REPO.
- If the session already exists, it reports that and exits successfully.
- Does not auto-attach; prints instructions to attach manually.

Examples
```
# Using default config location
gmux start a

# Using an explicit config directory
gmux start a --config-dir ~/my-config
```

## gmux version

Print version information for the binary.

Usage
- gmux version [--json]

Flags
- --json: Output version information as JSON

Examples
```
gmux version
gmux version --json
```