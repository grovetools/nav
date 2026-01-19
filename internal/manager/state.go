package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// SessionizerState holds the persistent state for the gmux sessionizer
type SessionizerState struct {
	FocusedEcosystemPath string `yaml:"focused_ecosystem_path,omitempty"`
	WorktreesFolded      bool   `yaml:"worktrees_folded,omitempty"`
	ShowGitStatus        *bool  `yaml:"show_git_status,omitempty"`
	ShowBranch           *bool  `yaml:"show_branch,omitempty"`
	ShowNoteCounts       *bool  `yaml:"show_note_counts,omitempty"`
	ShowPlanStats        *bool  `yaml:"show_plan_stats,omitempty"`
	PathDisplayMode      *int   `yaml:"path_display_mode,omitempty"` // 0=no paths, 1=compact (~), 2=full paths
	ShowRelease          *bool  `yaml:"show_release,omitempty"`
	ShowBinary           *bool  `yaml:"show_binary,omitempty"`
	ShowLink             *bool  `yaml:"show_link,omitempty"`
	ShowCx               *bool  `yaml:"show_cx,omitempty"`
}

// CachedProject holds project data with explicit types for proper JSON serialization
type CachedProject struct {
	// Embed WorkspaceNode for core properties
	*workspace.WorkspaceNode

	// Enrichment data
	GitStatus     *git.ExtendedGitStatus `json:"git_status,omitempty"`
	NoteCounts    *NoteCounts            `json:"note_counts,omitempty"`
	PlanStats     *PlanStats             `json:"plan_stats,omitempty"`
	ReleaseInfo   *ReleaseInfo           `json:"release_info,omitempty"`
	ActiveBinary  *BinaryStatus          `json:"active_binary,omitempty"`
	CxStats       *CxStats               `json:"cx_stats,omitempty"`
	GitRemoteURL  string                 `json:"git_remote_url,omitempty"`
}

// ProjectCache holds cached project data for fast startup
type ProjectCache struct {
	Projects  []CachedProject `json:"projects"`
	Timestamp time.Time       `json:"timestamp"`
}

// LoadState loads the sessionizer state from ~/.grove/gmux/state.yml
func LoadState(configDir string) (*SessionizerState, error) {
	statePath := filepath.Join(configDir, "gmux", "state.yml")

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file yet, return empty state
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

// Save saves the sessionizer state to ~/.grove/gmux/state.yml
func (s *SessionizerState) Save(configDir string) error {
	gmuxDir := filepath.Join(configDir, "gmux")

	// Ensure directory exists
	if err := os.MkdirAll(gmuxDir, 0755); err != nil {
		return err
	}

	statePath := filepath.Join(gmuxDir, "state.yml")

	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}

	return os.WriteFile(statePath, data, 0644)
}

// LoadProjectCache loads the cached project data from ~/.grove/gmux/cache.json
func LoadProjectCache(configDir string) (*ProjectCache, error) {
	cachePath := filepath.Join(configDir, "gmux", "cache.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache yet
		}
		return nil, err
	}

	var cache ProjectCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// SaveProjectCache saves the project cache to ~/.grove/gmux/cache.json
func SaveProjectCache(configDir string, projects []SessionizeProject) error {
	gmuxDir := filepath.Join(configDir, "gmux")

	// Ensure directory exists
	if err := os.MkdirAll(gmuxDir, 0755); err != nil {
		return err
	}

	// Convert to CachedProject with explicit types
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

	cachePath := filepath.Join(gmuxDir, "cache.json")
	return os.WriteFile(cachePath, data, 0644)
}

// KeyManageCache holds cached enriched project data for key manage
type KeyManageCache struct {
	EnrichedProjects map[string]CachedProject `json:"enriched_projects"` // path -> cached project
	Timestamp        time.Time                `json:"timestamp"`
}

// LoadKeyManageCache loads the cached key manage data from ~/.grove/gmux/km-cache.json
func LoadKeyManageCache(configDir string) (*KeyManageCache, error) {
	cachePath := filepath.Join(configDir, "gmux", "km-cache.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No cache yet
		}
		return nil, err
	}

	var cache KeyManageCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// SaveKeyManageCache saves the key manage cache to ~/.grove/gmux/km-cache.json
func SaveKeyManageCache(configDir string, enrichedProjects map[string]*SessionizeProject) error {
	gmuxDir := filepath.Join(configDir, "gmux")

	// Ensure directory exists
	if err := os.MkdirAll(gmuxDir, 0755); err != nil {
		return err
	}

	// Convert to CachedProject with explicit types
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

	cachePath := filepath.Join(gmuxDir, "km-cache.json")
	return os.WriteFile(cachePath, data, 0644)
}
