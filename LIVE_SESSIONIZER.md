# Live Sessionizer Feature

The `gmux sz` command now features live updates that automatically refresh every 10 seconds.

## What Updates Live

1. **Project Discovery**
   - New projects added to search paths appear automatically
   - Deleted projects are removed from the list
   - No need to restart the sessionizer

2. **Tmux Session Status**
   - Session indicators (●) update in real-time
   - Shows when sessions are started or stopped
   - Blue indicator (●) for current session, green (●) for others

3. **Key Mappings**
   - Changes to `tmux-sessions.yaml` are reflected immediately
   - Add, modify, or remove key mappings without restarting

4. **Git Status** (for active sessions)
   - Updates every 10 seconds for all running tmux sessions
   - Shows commit counts, modifications, staged files, etc.

5. **Claude Session Status** (if grove-hooks installed)
   - Live updates of Claude AI session states
   - Shows running (▶), idle (⏸), completed (✓), or failed (✗) status

## How It Works

The sessionizer now:
- Performs initial data load on startup
- Refreshes all data sources every 10 seconds
- Preserves your cursor position and filter text during updates
- Handles gracefully when projects are added/removed

## Testing the Feature

1. Start `gmux sz`
2. In another terminal:
   - Create a new project directory in your search paths
   - Start/stop tmux sessions with `tmux new -s test`
   - Edit `~/.config/tmux/tmux-sessions.yaml`
3. Watch the sessionizer update automatically!

The implementation follows the Bubble Tea pattern with separate commands for each data source and message types to handle the updates.