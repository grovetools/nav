package manager

import (
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/workspace"
)

// SessionizeProject is an enriched ProjectInfo for the sessionize TUI.
// It embeds the core ProjectInfo type and adds application-specific fields.
type SessionizeProject struct {
	workspace.ProjectInfo

	// Application-specific enrichment
	// Note: GitStatus and ClaudeSession are already in ProjectInfo
	// We only need to expose them through helper methods if needed
}

// GetGitStatus returns the git status as *git.StatusInfo for backward compatibility
func (p *SessionizeProject) GetGitStatus() *git.StatusInfo {
	if extStatus, ok := p.GitStatus.(*workspace.ExtendedGitStatus); ok && extStatus != nil {
		return extStatus.StatusInfo
	}
	return nil
}

// GetExtendedGitStatus returns the extended git status with line changes
func (p *SessionizeProject) GetExtendedGitStatus() *workspace.ExtendedGitStatus {
	if extStatus, ok := p.GitStatus.(*workspace.ExtendedGitStatus); ok {
		return extStatus
	}
	return nil
}

// Legacy type alias for backward compatibility
// Deprecated: Use SessionizeProject instead
type DiscoveredProject = SessionizeProject
