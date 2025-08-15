package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ProjectAccess tracks when a project was last accessed
type ProjectAccess struct {
	Path         string    `json:"path"`
	LastAccessed time.Time `json:"last_accessed"`
	AccessCount  int       `json:"access_count"`
}

// AccessHistory manages project access history
type AccessHistory struct {
	Projects map[string]*ProjectAccess `json:"projects"`
}

// LoadAccessHistory loads the access history from disk
func LoadAccessHistory(configDir string) (*AccessHistory, error) {
	historyFile := filepath.Join(configDir, "gmux", "access-history.json")

	data, err := os.ReadFile(historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty history if file doesn't exist
			return &AccessHistory{
				Projects: make(map[string]*ProjectAccess),
			}, nil
		}
		return nil, err
	}

	var history AccessHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	if history.Projects == nil {
		history.Projects = make(map[string]*ProjectAccess)
	}

	return &history, nil
}

// Save saves the access history to disk
func (h *AccessHistory) Save(configDir string) error {
	historyDir := filepath.Join(configDir, "gmux")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return err
	}

	historyFile := filepath.Join(historyDir, "access-history.json")

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0644)
}

// RecordAccess records that a project was accessed
func (h *AccessHistory) RecordAccess(path string) {
	if h.Projects == nil {
		h.Projects = make(map[string]*ProjectAccess)
	}

	if access, exists := h.Projects[path]; exists {
		access.LastAccessed = time.Now()
		access.AccessCount++
	} else {
		h.Projects[path] = &ProjectAccess{
			Path:         path,
			LastAccessed: time.Now(),
			AccessCount:  1,
		}
	}
}

// SortProjectsByAccess sorts projects by last access time (most recent first)
// For worktrees, uses the parent's access time to keep groups together
func (h *AccessHistory) SortProjectsByAccess(projects []DiscoveredProject) []DiscoveredProject {
	// Create a copy to avoid modifying the original
	sorted := make([]DiscoveredProject, len(projects))
	copy(sorted, projects)

	// Helper function to get the group's last access time
	getGroupAccessTime := func(p DiscoveredProject) *time.Time {
		// For worktrees, use parent's access time
		pathToCheck := p.Path
		if p.IsWorktree && p.ParentPath != "" {
			pathToCheck = p.ParentPath
		}

		if access, exists := h.Projects[pathToCheck]; exists {
			return &access.LastAccessed
		}
		return nil
	}

	sort.Slice(sorted, func(i, j int) bool {
		projectI := sorted[i]
		projectJ := sorted[j]

		// Get group access times
		accessTimeI := getGroupAccessTime(projectI)
		accessTimeJ := getGroupAccessTime(projectJ)

		// If neither group has been accessed, maintain original order
		if accessTimeI == nil && accessTimeJ == nil {
			return i < j
		}

		// If only one group has been accessed, it comes first
		if accessTimeI != nil && accessTimeJ == nil {
			return true
		}
		if accessTimeI == nil && accessTimeJ != nil {
			return false
		}

		// Both groups have been accessed
		// First, compare group access times
		if !accessTimeI.Equal(*accessTimeJ) {
			return accessTimeI.After(*accessTimeJ)
		}

		// Same group access time - check if they're in the same group
		groupI := projectI.Path
		if projectI.IsWorktree && projectI.ParentPath != "" {
			groupI = projectI.ParentPath
		}
		groupJ := projectJ.Path
		if projectJ.IsWorktree && projectJ.ParentPath != "" {
			groupJ = projectJ.ParentPath
		}

		if groupI == groupJ {
			// Same group - parent repos come before worktrees
			if projectI.IsWorktree != projectJ.IsWorktree {
				return !projectI.IsWorktree
			}
			// Both are worktrees or both are repos - sort alphabetically by name
			return projectI.Name < projectJ.Name
		}

		// Different groups with same access time - maintain original order
		return i < j
	})

	return sorted
}

// GetLastAccessed returns the last accessed time for a project
func (h *AccessHistory) GetLastAccessed(path string) *time.Time {
	if access, exists := h.Projects[path]; exists {
		return &access.LastAccessed
	}
	return nil
}
