package main

import (
	"context"

	tmuxclient "github.com/grovetools/core/pkg/tmux"

	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
)

// Compile-time checks that the nav/pkg/tmux Manager satisfies the
// Store interfaces of every TUI package it backs. If any of these ever
// fails, the corresponding TUI and its Store contract have drifted —
// update one side or the other.
var (
	_ sessionizer.Store = (*tmux.Manager)(nil)
	_ keymanage.Store   = (*tmux.Manager)(nil)
)

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

// Compile-time checks that TmuxDriver satisfies the sessionizer +
// keymanage driver ports. Both interfaces have the same Launch /
// SwitchTo / Exists / ClosePopup surface; sessionizer additionally
// requires Kill and ListActive, which TmuxDriver also exposes.
var (
	_ sessionizer.SessionDriver        = (*TmuxDriver)(nil)
	_ sessionizer.SessionStateProvider = (*TmuxDriver)(nil)
	_ keymanage.SessionDriver          = (*TmuxDriver)(nil)
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

// WindowsDriver adapts a *tmuxclient.Client to the windows.SessionDriver
// interface used by the extracted windows TUI. The move-window operation
// dispatches via the package-level tmuxclient.Command() helper so the
// GROVE_TMUX_SOCKET env var is still honored.
type WindowsDriver struct {
	client *tmuxclient.Client
}

// newWindowsDriver constructs a WindowsDriver.
func newWindowsDriver(client *tmuxclient.Client) *WindowsDriver {
	return &WindowsDriver{client: client}
}

// Compile-time check that WindowsDriver satisfies the windows port.
var _ windows.SessionDriver = (*WindowsDriver)(nil)

// ListWindows enumerates the windows of the given session.
func (d *WindowsDriver) ListWindows(ctx context.Context, sessionName string) ([]tmuxclient.Window, error) {
	return d.client.ListWindowsDetailed(ctx, sessionName)
}

// CapturePane captures a preview of the given target (session:window).
func (d *WindowsDriver) CapturePane(ctx context.Context, target string) (string, error) {
	return d.client.CapturePane(ctx, target)
}

// KillWindow destroys the given target.
func (d *WindowsDriver) KillWindow(ctx context.Context, target string) error {
	return d.client.KillWindow(ctx, target)
}

// RenameWindow renames the given target.
func (d *WindowsDriver) RenameWindow(ctx context.Context, target, newName string) error {
	return d.client.RenameWindow(ctx, target, newName)
}

// MoveWindow shells out to `tmux move-window -s SRC -t DST` via the
// package-level Command helper so GROVE_TMUX_SOCKET is honored. Matches
// the pre-extraction behavior.
func (d *WindowsDriver) MoveWindow(_ context.Context, srcTarget, dstTarget string) error {
	cmd := tmuxclient.Command("move-window", "-s", srcTarget, "-t", dstTarget)
	return cmd.Run()
}
