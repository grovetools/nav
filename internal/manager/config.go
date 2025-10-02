package manager

// TmuxConfig represents the 'tmux' section in grove.yml.
// Static configuration only - session mappings are stored separately in gmux/sessions.yml
type TmuxConfig struct {
	AvailableKeys    []string                   `yaml:"available_keys"`
	SearchPaths      map[string]SearchPathConfig `yaml:"search_paths,omitempty"`
	ExplicitProjects []ExplicitProject           `yaml:"explicit_projects,omitempty"`
	Discovery        DiscoveryConfig             `yaml:"discovery,omitempty"`
}

// TmuxSessionsFile represents the sessions file stored in ~/.config/grove/gmux/sessions.yml
// This is separate from grove.yml to avoid polluting version control with dynamic state
type TmuxSessionsFile struct {
	Sessions map[string]TmuxSessionConfig `yaml:"sessions"`
}

// TmuxSessionConfig defines the configuration for a single session mapped to a key.
type TmuxSessionConfig struct {
	Path        string `yaml:"path"`
	Repository  string `yaml:"repository,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// DiscoveryConfig holds settings for how projects are discovered.
type DiscoveryConfig struct {
	MaxDepth        int      `yaml:"max_depth"`
	MinDepth        int      `yaml:"min_depth"`
	FileTypes       []string `yaml:"file_types,omitempty"`
	ExcludePatterns []string `yaml:"exclude_patterns,omitempty"`
}
