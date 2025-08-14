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
func (h *AccessHistory) SortProjectsByAccess(projects []string) []string {
	// Create a copy to avoid modifying the original
	sorted := make([]string, len(projects))
	copy(sorted, projects)
	
	sort.Slice(sorted, func(i, j int) bool {
		// Get access info for both projects
		accessI, hasI := h.Projects[sorted[i]]
		accessJ, hasJ := h.Projects[sorted[j]]
		
		// If neither has been accessed, maintain original order
		if !hasI && !hasJ {
			return i < j
		}
		
		// If only one has been accessed, it comes first
		if hasI && !hasJ {
			return true
		}
		if !hasI && hasJ {
			return false
		}
		
		// Both have been accessed, sort by most recent
		return accessI.LastAccessed.After(accessJ.LastAccessed)
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