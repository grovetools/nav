package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// SessionizerState holds the persistent state for the nav sessionizer.
// It is serialized to nav/state.yml and survives across runs.
type SessionizerState struct {
	FocusedEcosystemPath string   `yaml:"focused_ecosystem_path,omitempty"`
	WorktreesFolded      bool     `yaml:"worktrees_folded,omitempty"`
	FoldedPaths          []string `yaml:"folded_paths,omitempty"`
	ShowGitStatus        *bool    `yaml:"show_git_status,omitempty"`
	ShowBranch           *bool    `yaml:"show_branch,omitempty"`
	ShowNoteCounts       *bool    `yaml:"show_note_counts,omitempty"`
	ShowPlanStats        *bool    `yaml:"show_plan_stats,omitempty"`
	PathDisplayMode      *int     `yaml:"path_display_mode,omitempty"`
	ShowRelease          *bool    `yaml:"show_release,omitempty"`
	ShowBinary           *bool    `yaml:"show_binary,omitempty"`
	ShowLink             *bool    `yaml:"show_link,omitempty"`
	ShowCx               *bool    `yaml:"show_cx,omitempty"`
	ShowTaskResults      *bool    `yaml:"show_task_results,omitempty"`
}

// CachedProject holds project data with explicit types for proper JSON serialization.
type CachedProject struct {
	*workspace.WorkspaceNode

	GitStatus    *git.ExtendedGitStatus `json:"git_status,omitempty"`
	NoteCounts   *models.NoteCounts     `json:"note_counts,omitempty"`
	PlanStats    *models.PlanStats      `json:"plan_stats,omitempty"`
	ReleaseInfo  *models.ReleaseInfo    `json:"release_info,omitempty"`
	ActiveBinary *models.BinaryStatus   `json:"active_binary,omitempty"`
	CxStats      *models.CxStats        `json:"cx_stats,omitempty"`
	GitRemoteURL string                 `json:"git_remote_url,omitempty"`
}

// ProjectCache holds cached project data for fast startup.
type ProjectCache struct {
	Projects  []CachedProject `json:"projects"`
	Timestamp time.Time       `json:"timestamp"`
}

// LoadState loads the sessionizer state from the nav state directory.
func LoadState(configDir string) (*SessionizerState, error) {
	statePath := filepath.Join(paths.StateDir(), "nav", "state.yml")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionizerState{}, nil
		}
		return nil, err
	}

	var state SessionizerState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save persists the sessionizer state to the nav state directory.
func (s *SessionizerState) Save(configDir string) error {
	navDir := filepath.Join(paths.StateDir(), "nav")
	if err := os.MkdirAll(navDir, 0o755); err != nil {
		return err
	}

	statePath := filepath.Join(navDir, "state.yml")
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath, data, 0o600)
}

// LoadProjectCache loads the cached project data from the nav cache directory.
func LoadProjectCache(configDir string) (*ProjectCache, error) {
	cachePath := filepath.Join(paths.CacheDir(), "nav", "cache.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cache ProjectCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveProjectCache saves the project cache to the nav cache directory.
func SaveProjectCache(configDir string, projects []Project) error {
	navDir := filepath.Join(paths.CacheDir(), "nav")
	if err := os.MkdirAll(navDir, 0o755); err != nil {
		return err
	}

	cachedProjects := make([]CachedProject, len(projects))
	for i, p := range projects {
		cachedProjects[i] = CachedProject{
			WorkspaceNode: p.WorkspaceNode,
			GitStatus:     p.GetExtendedGitStatus(),
			NoteCounts:    p.NoteCounts,
			PlanStats:     p.PlanStats,
			ReleaseInfo:   p.ReleaseInfo,
			ActiveBinary:  p.ActiveBinary,
			CxStats:       p.CxStats,
			GitRemoteURL:  p.GitRemoteURL,
		}
	}

	cache := ProjectCache{
		Projects:  cachedProjects,
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	cachePath := filepath.Join(navDir, "cache.json")
	return os.WriteFile(cachePath, data, 0o600)
}

// KeyManageCache holds cached enriched project data for the key manage TUI.
type KeyManageCache struct {
	EnrichedProjects map[string]CachedProject `json:"enriched_projects"`
	Timestamp        time.Time                `json:"timestamp"`
}

// LoadKeyManageCache loads the cached key manage data.
func LoadKeyManageCache(configDir string) (*KeyManageCache, error) {
	cachePath := filepath.Join(paths.CacheDir(), "nav", "km-cache.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cache KeyManageCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveKeyManageCache saves the key manage cache.
func SaveKeyManageCache(configDir string, enrichedProjects map[string]*Project) error {
	navDir := filepath.Join(paths.CacheDir(), "nav")
	if err := os.MkdirAll(navDir, 0o755); err != nil {
		return err
	}

	cachedProjects := make(map[string]CachedProject)
	for path, p := range enrichedProjects {
		cachedProjects[path] = CachedProject{
			WorkspaceNode: p.WorkspaceNode,
			GitStatus:     p.GetExtendedGitStatus(),
			NoteCounts:    p.NoteCounts,
			PlanStats:     p.PlanStats,
			ReleaseInfo:   p.ReleaseInfo,
			ActiveBinary:  p.ActiveBinary,
			CxStats:       p.CxStats,
			GitRemoteURL:  p.GitRemoteURL,
		}
	}

	cache := KeyManageCache{
		EnrichedProjects: cachedProjects,
		Timestamp:        time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	cachePath := filepath.Join(navDir, "km-cache.json")
	return os.WriteFile(cachePath, data, 0o600)
}

// Features captures resolved feature toggles for the sessionizer UI.
// This was previously called manager.ResolvedFeatures.
type Features struct {
	Groups       bool
	Ecosystems   bool
	Integrations bool
	Worktrees    bool
}
