package manager

import (
	"sort"
	"time"

	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// SortProjectsByAccess sorts projects by last access time (most recent first)
// For worktrees, uses the parent's access time to keep groups together
func SortProjectsByAccess(h *workspace.AccessHistory, projects []DiscoveredProject) []DiscoveredProject {
	// Create a copy to avoid modifying the original
	sorted := make([]DiscoveredProject, len(projects))
	copy(sorted, projects)

	// Helper function to get the group's last access time
	getGroupAccessTime := func(p DiscoveredProject) *time.Time {
		// For worktrees, use parent's access time
		pathToCheck := p.Path
		if p.IsWorktree() && p.ParentProjectPath != "" {
			pathToCheck = p.ParentProjectPath
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
		if projectI.IsWorktree() && projectI.ParentProjectPath != "" {
			groupI = projectI.ParentProjectPath
		}
		groupJ := projectJ.Path
		if projectJ.IsWorktree() && projectJ.ParentProjectPath != "" {
			groupJ = projectJ.ParentProjectPath
		}

		if groupI == groupJ {
			// Same group - parent repos come before worktrees
			isWorktreeI := projectI.IsWorktree()
			isWorktreeJ := projectJ.IsWorktree()
			if isWorktreeI != isWorktreeJ {
				return !isWorktreeI
			}
			// Both are worktrees or both are repos - sort alphabetically by name
			return projectI.Name < projectJ.Name
		}

		// Different groups with same access time - maintain original order
		return i < j
	})

	return sorted
}
