// Package sessionizer hosts the extracted nav sessionizer TUI. It depends
// only on small interfaces (Store, SessionDriver, SessionStateProvider)
// rather than nav's internal manager package, so it can be embedded by
// any binary that supplies the required behaviors — including the
// terminal multiplexer.
package sessionizer

import (
	"context"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nav/pkg/api"
)

// SessionDriver is the Cat 2 (execution) surface — anything that
// creates, switches, or destroys a session. The tmux implementation lives
// in nav/cmd/nav. Terminal will provide an in-process implementation.
type SessionDriver interface {
	Launch(ctx context.Context, sessionName, workingDir string) error
	SwitchTo(ctx context.Context, sessionName string) error
	Kill(ctx context.Context, sessionName string) error
	// ClosePopup dismisses the popup that the TUI is running inside (if any).
	// Implementations that don't run inside a popup should return nil.
	ClosePopup() error
}

// SessionStateProvider is the Cat 3 (live state feedback) surface —
// what's currently running, what's the active session, etc.
type SessionStateProvider interface {
	ListActive(ctx context.Context) ([]string, error)
	Exists(ctx context.Context, sessionName string) (bool, error)
}

// Store is the Cat 1 (selection + mutation) surface that the sessionizer
// reads while presenting the project list. It is intentionally narrow:
// only the methods the sessionizer model actually calls today. The nav
// binary's *tmux.Manager satisfies it implicitly.
type Store interface {
	// Project discovery / sorting
	GetAvailableProjects() ([]api.Project, error)
	GetAccessHistory() (*workspace.AccessHistory, error)
	RecordProjectAccess(path string) error

	// Session list mutation (used by 'e' to remap a session key on the fly)
	GetSessions() ([]models.TmuxSession, error)
	UpdateSessions(sessions []models.TmuxSession) error

	// Group state
	GetGroups() []string
	GetActiveGroup() string
	SetActiveGroup(group string)
	FindGroupForPath(path string) string
	GetGroupIcon(group string) string
	GetDefaultIcon() string
	SetLastAccessedGroup(group string) error
	CreateGroup(name, prefix string) error

	// Undo/redo (TUI just binds keys to these — never inspects state)
	TakeSnapshot()
	Undo() error
	Redo() error

	// Binding regeneration (terminal will supply a no-op)
	RegenerateBindings() error
}
