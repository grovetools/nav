package manager

import (
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
)


// NoteCounts holds counts of notes. This is now defined locally.
type NoteCounts struct {
	Current    int
	Issues     int
	Inbox      int
	Docs       int
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

// ReleaseInfo holds release tag and commit information.
type ReleaseInfo struct {
	LatestTag    string `json:"latest_tag"`
	CommitsAhead int    `json:"commits_ahead"`
}

// BinaryStatus holds the active status of a project's binary.
type BinaryStatus struct {
	ToolName       string `json:"tool_name"`        // The binary/tool name (e.g., "gmux", "flow")
	IsDevActive    bool   `json:"is_dev_active"`
	LinkName       string `json:"link_name"`
	CurrentVersion string `json:"current_version"` // Current installed version (e.g., "main-374a674")
}

// CxStats holds token counts from grove-context.
type CxStats struct {
	Files  int   `json:"total_files"`
	Tokens int   `json:"total_tokens"`
	Size   int64 `json:"total_size"`
}

// CxPerLineStat holds per-line stats from cx stats --per-line output.
type CxPerLineStat struct {
	LineNumber  int      `json:"lineNumber"`
	Rule        string   `json:"rule"`
	FileCount   int      `json:"fileCount"`
	TotalTokens int      `json:"totalTokens"`
	TotalSize   int64    `json:"totalSize"`
	SkipReason  string   `json:"skipReason,omitempty"`
}

// SessionizeProject is an enriched WorkspaceNode for the sessionize TUI.
// It embeds the core WorkspaceNode type and adds application-specific enrichment fields.
type SessionizeProject struct {
	*workspace.WorkspaceNode

	// Application-specific enrichment fields
	GitStatus     *git.ExtendedGitStatus
	NoteCounts    *NoteCounts
	PlanStats     *PlanStats
	ReleaseInfo   *ReleaseInfo
	ActiveBinary  *BinaryStatus
	CxStats       *CxStats
	GitRemoteURL  string

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
func (p *SessionizeProject) GetExtendedGitStatus() *git.ExtendedGitStatus {
	return p.GitStatus
}

// Legacy type alias for backward compatibility
// Deprecated: Use SessionizeProject instead
type DiscoveredProject = SessionizeProject
