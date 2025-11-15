package manager

import (
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// ExtendedGitStatus holds git status including line changes. This is now defined locally.
type ExtendedGitStatus struct {
	*git.StatusInfo
	LinesAdded   int
	LinesDeleted int
}

// ClaudeSessionInfo holds information about a Claude session. This is now defined locally.
type ClaudeSessionInfo struct {
	ID       string
	PID      int
	Status   string
	Duration string
}

// NoteCounts holds counts of notes. This is now defined locally.
type NoteCounts struct {
	Current    int
	Issues     int
	Inbox      int
	Completed  int
	Review     int
	InProgress int
	Other      int
}

// PlanStats holds stats about grove-flow plans. This is now defined locally.
type PlanStats struct {
	TotalPlans int
	ActivePlan string
	Running    int
	Pending    int
	Completed  int
	Failed     int
	Todo       int
	Hold       int
	Abandoned  int
	PlanStatus string `json:"plan_status,omitempty"` // Status of the plan itself (e.g., "hold", "finished")
}

// SessionizeProject is an enriched WorkspaceNode for the sessionize TUI.
// It embeds the core WorkspaceNode type and adds application-specific enrichment fields.
type SessionizeProject struct {
	*workspace.WorkspaceNode

	// Application-specific enrichment fields
	GitStatus     *ExtendedGitStatus
	ClaudeSession *ClaudeSessionInfo
	NoteCounts    *NoteCounts
	PlanStats     *PlanStats

	// EnrichmentStatus tracks the loading state of different data types (e.g., "git:loading", "git:done")
	EnrichmentStatus map[string]string `json:"-"` // Don't save in cache

	// ContextStatus holds the rule status (H, C, X) for the project.
	ContextStatus string `json:"context_status,omitempty"`
}

// GetGitStatus returns the git status as *git.StatusInfo for backward compatibility
func (p *SessionizeProject) GetGitStatus() *git.StatusInfo {
	if p.GitStatus != nil {
		return p.GitStatus.StatusInfo
	}
	return nil
}

// GetExtendedGitStatus returns the extended git status with line changes
func (p *SessionizeProject) GetExtendedGitStatus() *ExtendedGitStatus {
	return p.GitStatus
}

// Legacy type alias for backward compatibility
// Deprecated: Use SessionizeProject instead
type DiscoveredProject = SessionizeProject
