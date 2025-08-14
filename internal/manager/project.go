package manager

// DiscoveredProject holds structured information about a project found by the sessionizer.
type DiscoveredProject struct {
	Name       string // The base name of the directory (e.g., "grove-core" or "feat-new-feature")
	Path       string // The full, absolute path to the project
	ParentPath string // For worktrees, the path to the parent repository. Empty otherwise.
	IsWorktree bool   // True if the project is a Git worktree
}