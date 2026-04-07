package main

import (
	"context"

	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
)

// Compile-time check that the nav/pkg/tmux Manager satisfies the
// sessionizer.Store interface. If this ever fails, the sessionizer TUI
// and its Store contract have drifted — update one side or the other.
var _ sessionizer.Store = (*tmux.Manager)(nil)

// TmuxDriver wraps a *tmuxclient.Client and satisfies both
// sessionizer.SessionDriver and sessionizer.SessionStateProvider. It is
// the default backend wired in by the standalone nav binary — terminal
// will provide its own driver when embedding the sessionizer.
type TmuxDriver struct {
	client *tmuxclient.Client
}

// NewTmuxDriver constructs a TmuxDriver. The caller owns client lifetime.
func NewTmuxDriver(client *tmuxclient.Client) *TmuxDriver {
	return &TmuxDriver{client: client}
}

// Compile-time checks that TmuxDriver satisfies the sessionizer ports.
var (
	_ sessionizer.SessionDriver        = (*TmuxDriver)(nil)
	_ sessionizer.SessionStateProvider = (*TmuxDriver)(nil)
)

// Launch starts a new tmux session with the given name and working directory.
func (d *TmuxDriver) Launch(ctx context.Context, sessionName, workingDir string) error {
	return d.client.Launch(ctx, tmuxclient.LaunchOptions{
		SessionName:      sessionName,
		WorkingDirectory: workingDir,
	})
}

// SwitchTo switches the current tmux client to the given session.
func (d *TmuxDriver) SwitchTo(ctx context.Context, sessionName string) error {
	return d.client.SwitchClientToSession(ctx, sessionName)
}

// Kill destroys the given tmux session.
func (d *TmuxDriver) Kill(ctx context.Context, sessionName string) error {
	return d.client.KillSession(ctx, sessionName)
}

// ClosePopup dismisses the tmux popup the TUI is running inside, if any.
// Errors are intentionally swallowed — matching the pre-extraction
// behavior in sessionize.go where the ClosePopupCmd was best-effort.
func (d *TmuxDriver) ClosePopup() error {
	_ = d.client.ClosePopupCmd().Run()
	return nil
}

// ListActive returns the names of currently running tmux sessions.
func (d *TmuxDriver) ListActive(ctx context.Context) ([]string, error) {
	return d.client.ListSessions(ctx)
}

// Exists reports whether a session with the given name exists.
func (d *TmuxDriver) Exists(ctx context.Context, sessionName string) (bool, error) {
	return d.client.SessionExists(ctx, sessionName)
}
