# CLI Reference

Complete command reference for `nav`.

## nav

<div class="terminal">
<span class="term-bold term-fg-11">GMUX</span>
 <span class="term-italic">Grove tmux management tool</span>

 A CLI tool for managing tmux sessions and configurations
 in the Grove ecosystem.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux [command]

 <span class="term-italic term-fg-11">COMMANDS</span>
 <span class="term-bold term-fg-4">completion</span>      Generate the autocompletion script for the specified shell
 <span class="term-bold term-fg-4">history</span>         View and switch to recently accessed project sessions
 <span class="term-bold term-fg-4">key</span>             Manage tmux session key bindings
 <span class="term-bold term-fg-4">km</span>              Alias for 'key manage' - Interactively manage tmux session key mappings
 <span class="term-bold term-fg-4">launch</span>          Launch a new tmux session with optional panes
 <span class="term-bold term-fg-4">list</span>            List tmux sessions from configuration (alias for 'key list')
 <span class="term-bold term-fg-4">record-session</span>  Record the current tmux session to access history
 <span class="term-bold term-fg-4">session</span>         Manage tmux sessions
 <span class="term-bold term-fg-4">sessionize</span>      Quickly create or switch to tmux sessions from project directories
 <span class="term-bold term-fg-4">start</span>           Start a pre-configured tmux session
 <span class="term-bold term-fg-4">status</span>          Show git status for configured sessions
 <span class="term-bold term-fg-4">version</span>         Print the version information for this binary
 <span class="term-bold term-fg-4">wait</span>            Wait for a tmux session to close
 <span class="term-bold term-fg-4">windows</span>         Interactively manage windows in the current tmux session

 <span class="term-dim">Flags: --config-dir, -h/--help, -v/--version</span>

 Use "gmux [command] --help" for more information.
</div>

### nav history

<div class="terminal">
<span class="term-bold term-fg-11">GMUX HISTORY</span>
 <span class="term-italic">View and switch to recently accessed project sessions</span>

 Shows an interactive TUI listing recently accessed project
 sessions, sorted from most to least recent.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux history [flags]
 gmux history [command]

 <span class="term-italic term-fg-11">COMMANDS</span>
 <span class="term-bold term-fg-4">last</span>  Switch to the most recently accessed project session

 <span class="term-dim">Flags: -h/--help</span>

 Use "gmux history [command] --help" for more information.
</div>

#### nav history last

<div class="terminal">
<span class="term-bold term-fg-11">GMUX HISTORY LAST</span>
 <span class="term-italic">Switch to the most recently accessed project session</span>

 Switches to the most recently used project session without
 showing the interactive UI.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux history last [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for last
</div>

### nav key

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY</span>
 <span class="term-italic">Manage tmux session key bindings</span>

 Commands for managing tmux session key bindings including
 updating keys and editing session details.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key [command]

 <span class="term-italic term-fg-11">COMMANDS</span>
 <span class="term-bold term-fg-4">add</span>     Add a new session from available projects in search paths
 <span class="term-bold term-fg-4">edit</span>    Edit the details of a tmux session (path, repository, description)
 <span class="term-bold term-fg-4">list</span>    List all configured session keys
 <span class="term-bold term-fg-4">manage</span>  Interactively manage tmux session key mappings
 <span class="term-bold term-fg-4">unmap</span>   Unmap a session from its key binding
 <span class="term-bold term-fg-4">update</span>  Update the key binding for a tmux session

 <span class="term-dim">Flags: -h/--help</span>

 Use "gmux key [command] --help" for more information.
</div>

#### nav key add

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY ADD</span>
 <span class="term-italic">Add a new session from available projects in search paths</span>

 Discover projects from configured search paths and quickly
 map them to available keys.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key add [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for add
</div>

#### nav key edit

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY EDIT</span>
 <span class="term-italic">Edit the details of a tmux session (path, repository,</span>
 <span class="term-italic">description)</span>

 Edit the path, repository name, and description for an
 existing tmux session. If no key is provided, shows all
 sessions for selection.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key edit [key] [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for edit
</div>

#### nav key list

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY LIST</span>
 <span class="term-italic">List all configured session keys</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key list [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>   help for list
 <span class="term-fg-5">    --style</span>  Output style: table or compact<span class="term-dim"> (default: table)</span>
</div>

#### nav key manage

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY MANAGE</span>
 <span class="term-italic">Interactively manage tmux session key mappings</span>

 Open an interactive table to map/unmap sessions to keys.
 Use arrow keys to navigate, 'e' to map CWD to an empty
 key, and space to unmap. Changes are auto-saved on exit.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key manage [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for manage
</div>

#### nav key unmap

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY UNMAP</span>
 <span class="term-italic">Unmap a session from its key binding</span>

 Remove the mapping for a specific key, making it available
 for future use.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key unmap [key] [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for unmap
</div>

#### nav key update

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KEY UPDATE</span>
 <span class="term-italic">Update the key binding for a tmux session</span>

 Update the key binding for an existing tmux session. If no
 key is provided, shows all sessions for selection.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux key update [current-key] [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for update
</div>

### nav km

<div class="terminal">
<span class="term-bold term-fg-11">GMUX KM</span>
 <span class="term-italic">Alias for 'key manage' - Interactively manage tmux session</span>
 <span class="term-italic">key mappings</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux km [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for km
</div>

### nav launch

<div class="terminal">
<span class="term-bold term-fg-11">GMUX LAUNCH</span>
 <span class="term-italic">Launch a new tmux session with optional panes</span>

 Launch a new tmux session with support for multiple panes.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux launch &lt;session-name&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>         help for launch
 <span class="term-fg-5">    --pane</span>         Add a pane with command (can be used multiple times). Format: 'command[@workdir]'
 <span class="term-fg-5">    --window-name</span>  Name for the initial window
 <span class="term-fg-5">    --working-dir</span>  Working directory for the session

 <span class="term-italic term-fg-11">EXAMPLES</span>
 <span class="term-dim"># Simple session</span>
   <span class="term-fg-6">gmux</span> <span class="term-fg-4">launch</span> dev-session

 <span class="term-dim"># Session with window name and working directory</span>
   <span class="term-fg-6">gmux</span> <span class="term-fg-4">launch</span> dev-session <span class="term-fg-5">--window-name</span> coding <span class="term-fg-5">--working-dir</span> /path/to/project

 <span class="term-dim"># Session with multiple panes</span>
   <span class="term-fg-6">gmux</span> <span class="term-fg-4">launch</span> dev-session <span class="term-fg-5">--pane</span> "vim main.go" <span class="term-fg-5">--pane</span> "go test <span class="term-fg-5">-v"</span> <span class="term-fg-5">--pane</span> "htop"

 <span class="term-dim"># Complex panes with working directories (format: command[@workdir])</span>
   <span class="term-fg-6">gmux</span> <span class="term-fg-4">launch</span> dev-session <span class="term-fg-5">--pane</span> "npm run dev@/app/frontend" <span class="term-fg-5">--pane</span> "go run .@/app/backend"
</div>

### nav list

<div class="terminal">
<span class="term-bold term-fg-11">GMUX LIST</span>
 <span class="term-italic">List tmux sessions from configuration (alias for 'key</span>
 <span class="term-italic">list')</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux list [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>   help for list
 <span class="term-fg-5">    --style</span>  Output style: table or compact<span class="term-dim"> (default: table)</span>
</div>

### nav session

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSION</span>
 <span class="term-italic">Manage tmux sessions</span>

 Commands for managing tmux sessions including checking
 existence, killing sessions, and capturing pane content.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux session [command]

 <span class="term-italic term-fg-11">COMMANDS</span>
 <span class="term-bold term-fg-4">capture</span>  Capture content from a tmux pane
 <span class="term-bold term-fg-4">exists</span>   Check if a tmux session exists
 <span class="term-bold term-fg-4">kill</span>     Kill a tmux session

 <span class="term-dim">Flags: -h/--help</span>

 Use "gmux session [command] --help" for more information.
</div>

#### nav session capture

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSION CAPTURE</span>
 <span class="term-italic">Capture content from a tmux pane</span>

 Capture content from a tmux pane. Target can be
 session-name, session-name:window.pane, etc.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux session capture &lt;target&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for capture
</div>

#### nav session exists

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSION EXISTS</span>
 <span class="term-italic">Check if a tmux session exists</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux session exists &lt;session-name&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for exists
</div>

#### nav session kill

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSION KILL</span>
 <span class="term-italic">Kill a tmux session</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux session kill &lt;session-name&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for kill
</div>

### nav sessionize

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSIONIZE</span>
 <span class="term-italic">Quickly create or switch to tmux sessions from project</span>
 <span class="term-italic">directories</span>

 Discover projects from configured search paths and quickly
 create or switch to tmux sessions. Shows Claude session
 status indicators when grove-hooks is installed.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux sessionize [flags]
 gmux sessionize [command]

 <span class="term-italic term-fg-11">COMMANDS</span>
 <span class="term-bold term-fg-4">add</span>     [DEPRECATED] Add an explicit project to sessionizer
 <span class="term-bold term-fg-4">remove</span>  [DEPRECATED] Remove an explicit project from sessionizer

 <span class="term-dim">Flags: -h/--help</span>

 Use "gmux sessionize [command] --help" for more information.
</div>

#### nav sessionize add

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSIONIZE ADD</span>
 <span class="term-italic">[DEPRECATED] Add an explicit project to sessionizer</span>

 This command is deprecated. Project discovery is now
 managed via the global grove.yml 'groves' configuration.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux sessionize add [path] [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for add
</div>

#### nav sessionize remove

<div class="terminal">
<span class="term-bold term-fg-11">GMUX SESSIONIZE REMOVE</span>
 <span class="term-italic">[DEPRECATED] Remove an explicit project from sessionizer</span>

 This command is deprecated. Project discovery is now
 managed via the global grove.yml 'groves' configuration.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux sessionize remove [path] [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for remove
</div>

### nav start

<div class="terminal">
<span class="term-bold term-fg-11">GMUX START</span>
 <span class="term-italic">Start a pre-configured tmux session</span>

 Start a tmux session using configuration from
 tmux-sessions.yaml.
 
 The session will be created with the name 'grove-&lt;key&gt;'
 and will automatically
 change to the configured directory for that session.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux start &lt;key&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for start
</div>

### nav status

<div class="terminal">
<span class="term-bold term-fg-11">GMUX STATUS</span>
 <span class="term-italic">Show git status for configured sessions</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux status [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for status
</div>

### nav version

<div class="terminal">
<span class="term-bold term-fg-11">GMUX VERSION</span>
 <span class="term-italic">Print the version information for this binary</span>

 <span class="term-italic term-fg-11">USAGE</span>
 gmux version [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for version
 <span class="term-fg-5">    --json</span>  Output version information in JSON format
</div>

### nav wait

<div class="terminal">
<span class="term-bold term-fg-11">GMUX WAIT</span>
 <span class="term-italic">Wait for a tmux session to close</span>

 Block until the specified tmux session closes. Useful for
 scripting and automation.
 	
 The command will poll at regular intervals to check if the
 session still exists.
 When the session closes, the command exits with status 0.
 If the timeout is reached or an error occurs, it exits
 with non-zero status.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux wait &lt;session-name&gt; [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>           help for wait
 <span class="term-fg-5">    --poll-interval</span>  How often to check if session exists<span class="term-dim"> (default: 1s)</span>
 <span class="term-fg-5">    --timeout</span>        Maximum time to wait (0 = no timeout)<span class="term-dim"> (default: 0s)</span>
</div>

### nav windows

<div class="terminal">
<span class="term-bold term-fg-11">GMUX WINDOWS</span>
 <span class="term-italic">Interactively manage windows in the current tmux session</span>

 Launches a TUI to list, filter, and manage windows in the
 current tmux session.

 <span class="term-italic term-fg-11">USAGE</span>
 gmux windows [flags]

 <span class="term-italic term-fg-11">FLAGS</span>
 <span class="term-fg-5">-h, --help</span>  help for windows
</div>

