// Package tmux provides a general-purpose Go client for managing tmux sessions.
// This package is designed to be self-contained and can be used by any Go application
// that needs to programmatically control tmux sessions.
//
// The package provides functionality to:
//   - Create and manage tmux sessions
//   - Launch sessions with complex multi-pane configurations
//   - Monitor sessions and wait for them to close
//   - Capture pane content
//   - Check session existence
//
// Example usage:
//
//	client, err := tmux.NewClient()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	opts := tmux.LaunchOptions{
//	    SessionName: "my-session",
//	    Panes: []tmux.PaneOptions{
//	        {Command: "vim main.go"},
//	        {Command: "go test -v"},
//	    },
//	}
//
//	if err := client.Launch(ctx, opts); err != nil {
//	    log.Fatal(err)
//	}
package tmux