package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-core/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// SessionizerState holds the persistent state for the gmux sessionizer
type SessionizerState struct {
	FocusedEcosystemPath string  `yaml:"focused_ecosystem_path,omitempty"`
	WorktreesFolded      bool    `yaml:"worktrees_folded,omitempty"`
	ShowGitStatus        *bool   `yaml:"show_git_status,omitempty"`
	ShowBranch           *bool   `yaml:"show_branch,omitempty"`
	ShowClaudeSessions   *bool   `yaml:"show_claude_sessions,omitempty"`
	ShowNoteCounts       *bool   `yaml:"show_note_counts,omitempty"`
	PathDisplayMode      *int    `yaml:"path_display_mode,omitempty"` // 0=no paths, 1=compact (~), 2=full paths
	ViewMode             *string `yaml:"view_mode,omitempty"`         // "tree" or "table"
}

// CachedProject holds project data with explicit types for proper JSON serialization
type CachedProject struct {
	Name                string                       `json:"name"`
	Path                string                       `json:"path"`
	ParentPath          string                       `json:"parent_path,omitempty"`
	IsWorktree          bool                         `json:"is_worktree"`
	WorktreeName        string                       `json:"worktree_name,omitempty"`
	ParentEcosystemPath string                       `json:"parent_ecosystem_path,omitempty"`
	IsEcosystem         bool                         `json:"is_ecosystem"`
	GitStatus           *workspace.ExtendedGitStatus `json:"git_status,omitempty"`
	ClaudeSession       *workspace.ClaudeSessionInfo `json:"claude_session,omitempty"`
	NoteCounts          *workspace.NoteCounts        `json:"note_counts,omitempty"`
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
		cached := CachedProject{
			Name:                p.Name,
			Path:                p.Path,
			ParentPath:          p.ParentPath,
			IsWorktree:          p.IsWorktree,
			WorktreeName:        p.WorktreeName,
			ParentEcosystemPath: p.ParentEcosystemPath,
			IsEcosystem:         p.IsEcosystem,
			ClaudeSession:       p.ClaudeSession,
			NoteCounts:          p.NoteCounts,
		}
		// Extract ExtendedGitStatus from interface{}
		if extStatus, ok := p.GitStatus.(*workspace.ExtendedGitStatus); ok {
			cached.GitStatus = extStatus
		}
		cachedProjects[i] = cached
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
