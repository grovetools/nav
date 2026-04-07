package manager

import (
	"github.com/grovetools/nav/pkg/api"
)

// State / cache types now live in nav/pkg/api so the exported sessionizer
// package can persist UI state without depending on internal/manager. The
// aliases below are kept so cmd/nav and other in-tree callers continue to
// compile during the migration.

type SessionizerState = api.SessionizerState
type CachedProject = api.CachedProject
type ProjectCache = api.ProjectCache
type KeyManageCache = api.KeyManageCache

func LoadState(configDir string) (*SessionizerState, error) {
	return api.LoadState(configDir)
}

func LoadProjectCache(configDir string) (*ProjectCache, error) {
	return api.LoadProjectCache(configDir)
}

func SaveProjectCache(configDir string, projects []SessionizeProject) error {
	return api.SaveProjectCache(configDir, projects)
}

func LoadKeyManageCache(configDir string) (*KeyManageCache, error) {
	return api.LoadKeyManageCache(configDir)
}

func SaveKeyManageCache(configDir string, enrichedProjects map[string]*SessionizeProject) error {
	return api.SaveKeyManageCache(configDir, enrichedProjects)
}
