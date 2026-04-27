// Package bindings provides standalone functions for managing nav binding state.
// These are extracted from the nav Manager so the daemon can import them directly.
package bindings

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"gopkg.in/yaml.v3"
)

// DefaultPath returns the default path for the nav sessions state file.
func DefaultPath() string {
	return filepath.Join(paths.StateDir(), "nav", "sessions.yml")
}

// Load reads a NavSessionsFile from the given YAML path.
// Returns an empty NavSessionsFile (not nil) if the file doesn't exist.
func Load(path string) (*models.NavSessionsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.NavSessionsFile{
				Sessions: make(map[string]models.NavSessionConfig),
			}, nil
		}
		return nil, fmt.Errorf("failed to read sessions file: %w", err)
	}

	var file models.NavSessionsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse sessions file: %w", err)
	}
	if file.Sessions == nil {
		file.Sessions = make(map[string]models.NavSessionConfig)
	}
	return &file, nil
}

// Save writes a NavSessionsFile to the given YAML path.
func Save(path string, file *models.NavSessionsFile) error {
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}
