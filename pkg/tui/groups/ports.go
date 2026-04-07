// Package groups hosts the extracted nav workspace-groups management
// TUI. It depends only on the small Store interface defined here, so it
// can be embedded by any host that satisfies the methods. Standalone
// nav supplies *tmux.Manager.
package groups

// Store is the narrow interface the groups TUI needs from its host. The
// nav binary's *tmux.Manager satisfies it implicitly.
type Store interface {
	GetAllGroups() []string
	GetGroupSessionCount(name string) int
	GetDefaultIcon() string
	GetGroupIcon(name string) string
	GetPrefixForGroup(name string) string
	IsGroupExplicitlyInactive(name string) bool

	TakeSnapshot()
	Undo() error
	Redo() error

	SetActiveGroup(name string)
	SetLastAccessedGroup(name string) error
	CreateGroup(name, prefix string) error
	DeleteGroup(name string) error
	RenameGroup(oldName, newName string) error
	SetGroupPrefix(name, prefix string) error
	SetGroupOrder(name string, order int) error
}
