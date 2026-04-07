// Package api exposes nav types that cross package boundaries — most
// importantly the enriched workspace struct consumed by the sessionizer
// TUI. Types here are safe to import from outside the nav module.
package api

import (
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
)

// Project is an enriched WorkspaceNode surfaced by the nav sessionizer UI.
// It embeds the core WorkspaceNode and layers in git status, note/plan
// stats, release info, binary status, and cx stats. All enrichment types
// come from core/pkg/models so downstream consumers (terminal, daemon)
// can pass Projects around without translation.
type Project struct {
	*workspace.WorkspaceNode

	// Application-specific enrichment fields
	GitStatus    *git.ExtendedGitStatus
	NoteCounts   *models.NoteCounts
	PlanStats    *models.PlanStats
	ReleaseInfo  *models.ReleaseInfo
	ActiveBinary *models.BinaryStatus
	CxStats      *models.CxStats
	GitRemoteURL string

	// EnrichmentStatus tracks the loading state of different data types
	// (e.g., "git:loading", "git:done"). Not persisted in caches.
	EnrichmentStatus map[string]string `json:"-"`

	// ContextStatus holds the rule status (H, C, X) for the project.
	ContextStatus string `json:"context_status,omitempty"`
}

// GetGitStatus returns the git status as *git.StatusInfo for backward compatibility.
func (p *Project) GetGitStatus() *git.StatusInfo {
	if p.GitStatus != nil {
		return p.GitStatus.StatusInfo
	}
	return nil
}

// GetExtendedGitStatus returns the extended git status with line changes.
func (p *Project) GetExtendedGitStatus() *git.ExtendedGitStatus {
	return p.GitStatus
}
