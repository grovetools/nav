package tmux

import (
	"os/exec"

	"github.com/grovetools/core/pkg/models"
	coretmux "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"

	"github.com/grovetools/nav/internal/manager"
)

// Command creates an exec.Cmd for tmux that respects GROVE_TMUX_SOCKET.
// This ensures all tmux operations use the same socket as the tmux client.
// Re-exported from grove-core for consistency across the ecosystem.
func Command(args ...string) *exec.Cmd {
	return coretmux.Command(args...)
}

// Manager manages tmux sessions and configurations
type Manager struct {
	mgr *manager.Manager
}

// NewManager creates a new tmux manager
func NewManager(configDir string) (*Manager, error) {
	mgr, err := manager.NewManager(configDir)
	if err != nil {
		return nil, err
	}
	return &Manager{
		mgr: mgr,
	}, nil
}

// GetSessions returns all configured tmux sessions
func (m *Manager) GetSessions() ([]models.TmuxSession, error) {
	return m.mgr.GetSessions()
}

// UpdateSessions updates all tmux sessions
func (m *Manager) UpdateSessions(sessions []models.TmuxSession) error {
	return m.mgr.UpdateSessions(sessions)
}

// UpdateSessionsAndLocks updates all tmux sessions and locked keys
func (m *Manager) UpdateSessionsAndLocks(sessions []models.TmuxSession, lockedKeys []string) error {
	return m.mgr.UpdateSessionsAndLocks(sessions, lockedKeys)
}

// TakeSnapshot captures the current state for undo/redo operations.
// Call this before making any destructive changes to mappings or groups.
func (m *Manager) TakeSnapshot() {
	m.mgr.TakeSnapshot()
}

// Undo reverts the last data change.
func (m *Manager) Undo() error {
	return m.mgr.Undo()
}

// Redo re-applies a previously undone change.
func (m *Manager) Redo() error {
	return m.mgr.Redo()
}

// GetLockedKeys returns the list of locked keys
func (m *Manager) GetLockedKeys() []string {
	return m.mgr.GetLockedKeys()
}

// UpdateSingleSession updates a single tmux session
func (m *Manager) UpdateSingleSession(key string, session models.TmuxSession) error {
	return m.mgr.UpdateSingleSession(key, session)
}

// GetAvailableProjects returns available projects from search paths
func (m *Manager) GetAvailableProjects() ([]manager.DiscoveredProject, error) {
	return m.mgr.GetAvailableProjects()
}

// GetAvailableProjectsWithOptions returns available projects with custom enrichment options
func (m *Manager) GetAvailableProjectsWithOptions(enrichOpts interface{}) ([]manager.DiscoveredProject, error) {
	return m.mgr.GetAvailableProjectsWithOptions(enrichOpts)
}

// GetAvailableProjectsSorted returns available projects sorted by last access time
func (m *Manager) GetAvailableProjectsSorted() ([]manager.DiscoveredProject, error) {
	return m.mgr.GetAvailableProjectsSorted()
}

// RecordProjectAccess records that a project was accessed
func (m *Manager) RecordProjectAccess(path string) error {
	return m.mgr.RecordProjectAccess(path)
}

// GetAccessHistory returns the project access history
func (m *Manager) GetAccessHistory() (*workspace.AccessHistory, error) {
	return m.mgr.GetAccessHistory()
}

// GetEnabledSearchPaths returns the list of enabled search paths
func (m *Manager) GetEnabledSearchPaths() ([]string, error) {
	return m.mgr.GetEnabledSearchPaths()
}

// RegenerateBindings regenerates tmux key bindings
func (m *Manager) RegenerateBindings() error {
	return m.mgr.RegenerateBindings()
}

// GetGitStatuses returns git status for all configured repositories
func (m *Manager) GetGitStatuses() (map[string]models.GitStatus, error) {
	return m.mgr.GetGitStatuses()
}

// GetGitStatus returns git status for a specific path and repository
func (m *Manager) GetGitStatus(path, repo string) models.GitStatus {
	return m.mgr.GetGitStatus(path, repo)
}

// Sessionize creates or switches to a tmux session for the given path
func (m *Manager) Sessionize(path string) error {
	return m.mgr.Sessionize(path)
}

// DetectTmuxKeyForPath detects the tmux session key for a given working directory
func (m *Manager) DetectTmuxKeyForPath(workingDir string) string {
	return m.mgr.DetectTmuxKeyForPath(workingDir)
}

// GetAvailableKeys returns all available keys from configuration
func (m *Manager) GetAvailableKeys() []string {
	return m.mgr.GetAvailableKeys()
}

// UpdateSessionKey updates the key for a specific session
func (m *Manager) UpdateSessionKey(oldKey, newKey string) error {
	return m.mgr.UpdateSessionKey(oldKey, newKey)
}

// GetPrefix returns the configured prefix for nav bindings
func (m *Manager) GetPrefix() string {
	return m.mgr.GetPrefix()
}

// SetActiveGroup switches the manager to operate on a different workspace group
func (m *Manager) SetActiveGroup(group string) {
	m.mgr.SetActiveGroup(group)
}

// GetActiveGroup returns the name of the currently active workspace group
func (m *Manager) GetActiveGroup() string {
	return m.mgr.GetActiveGroup()
}

// GetGroups returns a list of all available workspace groups
func (m *Manager) GetGroups() []string {
	return m.mgr.GetGroups()
}

// GetPrefixForGroup returns the configured prefix for a specific group
func (m *Manager) GetPrefixForGroup(group string) string {
	return m.mgr.GetPrefixForGroup(group)
}

// GetGroupConfig returns the configuration for a specific group
func (m *Manager) GetGroupConfig(group string) (manager.GroupRef, bool) {
	return m.mgr.GetGroupConfig(group)
}

// GetGroupIcon returns the configured icon for a specific group.
func (m *Manager) GetGroupIcon(group string) string {
	return m.mgr.GetGroupIcon(group)
}

// IsGroupExplicitlyInactive reports whether the group is configured with
// active=false. Used by the extracted groups TUI.
func (m *Manager) IsGroupExplicitlyInactive(group string) bool {
	return m.mgr.IsGroupExplicitlyInactive(group)
}

// ConfirmKeyUpdates returns whether to show confirmation prompts for bulk key update operations
func (m *Manager) ConfirmKeyUpdates() bool {
	return m.mgr.ConfirmKeyUpdates()
}

// GetAllGroups returns a list of all available workspace groups, including deactivated ones
func (m *Manager) GetAllGroups() []string {
	return m.mgr.GetAllGroups()
}

// CreateGroup creates a new group
func (m *Manager) CreateGroup(name, prefix string) error {
	return m.mgr.CreateGroup(name, prefix)
}

// DeleteGroup removes a group from config and state
func (m *Manager) DeleteGroup(name string) error {
	return m.mgr.DeleteGroup(name)
}

// SetGroupActive sets the active state of a group
func (m *Manager) SetGroupActive(name string, active bool) error {
	return m.mgr.SetGroupActive(name, active)
}

// FindGroupForPath finds which group contains a session with the given path
func (m *Manager) FindGroupForPath(path string) string {
	return m.mgr.FindGroupForPath(path)
}

// SetLastAccessedGroup updates the access history
func (m *Manager) SetLastAccessedGroup(group string) error {
	return m.mgr.SetLastAccessedGroup(group)
}

// GetLastAccessedGroup returns the most recently accessed group
func (m *Manager) GetLastAccessedGroup() string {
	return m.mgr.GetLastAccessedGroup()
}

// RenameGroup renames a group, updating both static config and state
func (m *Manager) RenameGroup(oldName, newName string) error {
	return m.mgr.RenameGroup(oldName, newName)
}

// SetGroupOrder sets the display order for a group
func (m *Manager) SetGroupOrder(name string, order int) error {
	return m.mgr.SetGroupOrder(name, order)
}

// SetGroupPrefix sets the prefix key for a group
func (m *Manager) SetGroupPrefix(name, prefix string) error {
	return m.mgr.SetGroupPrefix(name, prefix)
}

// GetGroupSessionCount returns the number of sessions in a group
func (m *Manager) GetGroupSessionCount(name string) int {
	return m.mgr.GetGroupSessionCount(name)
}

// GetDefaultIcon returns the configured icon for the default group
func (m *Manager) GetDefaultIcon() string {
	return m.mgr.GetDefaultIcon()
}

// GetResolvedFeatures returns the resolved feature flags for the TUI
func (m *Manager) GetResolvedFeatures() manager.ResolvedFeatures {
	return m.mgr.GetResolvedFeatures()
}
