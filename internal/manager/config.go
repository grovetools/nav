package manager

//go:generate sh -c "cd ../.. && go run ./tools/schema-generator/"

// TmuxConfig represents the 'tmux' section in grove.yml.
// This struct now only contains static configuration specific to gmux itself.
// Project discovery is now handled by grove-core's DiscoveryService.
type TmuxConfig struct {
	AvailableKeys []string `yaml:"available_keys"`
}

// TmuxSessionsFile represents the sessions file stored in ~/.config/grove/gmux/sessions.yml
// This is separate from grove.yml to avoid polluting version control with dynamic state
type TmuxSessionsFile struct {
	Sessions   map[string]TmuxSessionConfig `yaml:"sessions"`
	LockedKeys []string                     `yaml:"locked_keys,omitempty"`
}

// TmuxSessionConfig defines the configuration for a single session mapped to a key.
type TmuxSessionConfig struct {
	Path        string `yaml:"path"`
	Repository  string `yaml:"repository,omitempty"`
	Description string `yaml:"description,omitempty"`
}
