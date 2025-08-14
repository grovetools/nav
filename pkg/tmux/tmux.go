package tmux

import (
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-tmux/internal/manager"
)

// Manager manages tmux sessions and configurations
type Manager struct {
	mgr *manager.Manager
}

// NewManager creates a new tmux manager
func NewManager(configDir string, sessionsFile string) *Manager {
	return &Manager{
		mgr: manager.NewManager(configDir, sessionsFile),
	}
}

// GetSessions returns all configured tmux sessions
func (m *Manager) GetSessions() ([]models.TmuxSession, error) {
	return m.mgr.GetSessions()
}

// UpdateSessions updates all tmux sessions
func (m *Manager) UpdateSessions(sessions []models.TmuxSession) error {
	return m.mgr.UpdateSessions(sessions)
}

// UpdateSingleSession updates a single tmux session
func (m *Manager) UpdateSingleSession(key string, session models.TmuxSession) error {
	return m.mgr.UpdateSingleSession(key, session)
}

// GetAvailableProjects returns available projects from search paths
func (m *Manager) GetAvailableProjects() ([]manager.DiscoveredProject, error) {
	return m.mgr.GetAvailableProjects()
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
func (m *Manager) GetAccessHistory() (interface{}, error) {
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