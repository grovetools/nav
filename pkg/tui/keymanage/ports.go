// Package keymanage hosts the extracted nav key-management TUI. It
// depends only on the small Store and SessionDriver interfaces defined
// here, so it can be embedded by any host that satisfies the methods.
// Standalone nav supplies *tmux.Manager + a thin tmuxclient adapter.
package keymanage

import (
	"context"

	"github.com/grovetools/core/pkg/models"
)

// Store is the narrow interface the keymanage TUI needs from its host.
// Every method corresponds to a behavior the TUI drives while managing
// session key bindings (reads, mutations, undo/redo, groups). The nav
// binary's *tmux.Manager satisfies it implicitly.
type Store interface {
	// Session reads/writes
	GetSessions() ([]models.TmuxSession, error)
	UpdateSessionsAndLocks(sessions []models.TmuxSession, lockedKeys []string) error
	GetLockedKeys() []string

	// Undo/redo/snapshots
	TakeSnapshot()
	Undo() error
	Redo() error

	// Groups — reads
	GetActiveGroup() string
	GetGroups() []string
	GetAllGroups() []string
	GetGroupIcon(name string) string
	GetDefaultIcon() string
	GetPrefix() string
	ConfirmKeyUpdates() bool

	// Groups — mutations
	SetActiveGroup(group string)
	SetLastAccessedGroup(group string) error
	CreateGroup(name, prefix string) error
	DeleteGroup(name string) error

	// Bindings regeneration (terminal may supply a no-op)
	RegenerateBindings() error

	// History
	RecordProjectAccess(path string) error
}

// SessionDriver is the Cat 2 (execution) surface — anything that
// creates, switches, or verifies a tmux session. Standalone nav wires
// a tmuxclient-backed adapter; terminal will supply an in-process
// implementation (or nil if no session launching is supported).
type SessionDriver interface {
	Launch(ctx context.Context, sessionName, workingDir string) error
	SwitchTo(ctx context.Context, sessionName string) error
	Exists(ctx context.Context, sessionName string) (bool, error)
	// ClosePopup dismisses the popup the TUI may be running inside;
	// implementations that don't run inside a popup should return nil.
	ClosePopup() error
}
