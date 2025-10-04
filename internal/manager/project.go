package manager

import "github.com/mattsolo1/grove-core/git"

// DiscoveredProject holds structured information about a project found by the sessionizer.
type DiscoveredProject struct {
	Name                string // The base name of the directory (e.g., "grove-core" or "feat-new-feature")
	Path                string // The full, absolute path to the project
	ParentPath          string // For worktrees, the path to the parent repository. Empty otherwise.
	IsWorktree          bool   // True if the project is a Git worktree
	ParentEcosystemPath string // For sub-projects, the path to the parent ecosystem. Empty otherwise.
	IsEcosystem         bool   // True if the project is an ecosystem (contains submodules/repos)
	GitStatus           *git.StatusInfo

	// Claude session information (if this entry represents a Claude session)
	ClaudeSessionID       string // The Claude session ID (empty if not a Claude session)
	ClaudeSessionPID      int    // The Claude session PID
	ClaudeSessionStatus   string // The Claude session status
	ClaudeSessionDuration string // The Claude session duration
}
