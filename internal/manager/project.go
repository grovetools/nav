package manager

import (
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// SessionizeProject is an enriched WorkspaceNode for the sessionize TUI.
// It embeds the core WorkspaceNode type and adds application-specific enrichment fields.
type SessionizeProject struct {
	*workspace.WorkspaceNode

	// Application-specific enrichment fields
	GitStatus    *git.ExtendedGitStatus
	NoteCounts   *models.NoteCounts
	PlanStats    *models.PlanStats
	ReleaseInfo  *models.ReleaseInfo
	ActiveBinary *models.BinaryStatus
	CxStats      *models.CxStats
	GitRemoteURL string

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
