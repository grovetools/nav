// Package windows hosts the extracted nav tmux-windows browser TUI. It
// depends only on the small SessionDriver interface defined here plus
// core/pkg/tmux for window types, so it can be embedded by any host
// that can enumerate, preview, rename, close, and reorder tmux windows.
// Standalone nav supplies *tmuxclient.Client via a thin adapter.
package windows

import (
	"context"

	tmuxclient "github.com/grovetools/core/pkg/tmux"
)

// SessionDriver is the narrow interface the windows TUI needs from its
// host. It covers every tmux operation the model actually performs —
// enumerating windows, capturing previews, renaming, killing, and
// reordering. Standalone nav implements it by wrapping *tmuxclient.Client.
type SessionDriver interface {
	// ListWindows returns every window in the given session with the
	// detailed metadata the browser renders.
	ListWindows(ctx context.Context, sessionName string) ([]tmuxclient.Window, error)

	// CapturePane captures a text preview of the given target
	// (typically "session:window-index").
	CapturePane(ctx context.Context, target string) (string, error)

	// KillWindow destroys the window identified by target.
	KillWindow(ctx context.Context, target string) error

	// RenameWindow renames the window identified by target.
	RenameWindow(ctx context.Context, target string, newName string) error

	// MoveWindow moves srcTarget to dstTarget. It is called repeatedly
	// during a reorder operation, first to shuffle every window to a
	// temporary high index and then back down into its final position.
	MoveWindow(ctx context.Context, srcTarget, dstTarget string) error
}
