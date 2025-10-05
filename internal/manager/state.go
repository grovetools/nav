package manager

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SessionizerState holds the persistent state for the gmux sessionizer
type SessionizerState struct {
	FocusedEcosystemPath string `yaml:"focused_ecosystem_path,omitempty"`
	WorktreesFolded      bool   `yaml:"worktrees_folded,omitempty"`
	ShowGitStatus        *bool  `yaml:"show_git_status,omitempty"`
	ShowClaudeSessions   *bool  `yaml:"show_claude_sessions,omitempty"`
	ShowFullPaths        *bool  `yaml:"show_full_paths,omitempty"`
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
