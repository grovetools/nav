package manager

//go:generate sh -c "cd ../.. && go run ./tools/schema-generator/"

// TmuxConfig represents the 'nav' section in grove config (legacy name retained for compatibility).
// For backwards compatibility, the 'tmux' section is also supported.
// This struct only contains static configuration specific to nav itself.
// Project discovery is handled by grove-core's DiscoveryService.
type TmuxConfig struct {
	Prefix             string              `yaml:"prefix,omitempty" toml:"prefix,omitempty" jsonschema:"description=Prefix key for nav bindings. Options: '<prefix>' (default), '<prefix> X' (sub-table under prefix), 'C-g' (dedicated root key), or '' (direct root with modifiers)." jsonschema_extras:"x-layer=global,x-priority=69"`
	AvailableKeys      []string            `yaml:"available_keys" toml:"available_keys" jsonschema:"description=Keys available for tmux pane shortcuts" jsonschema_extras:"x-layer=global,x-priority=70,x-important=true"`
	ShowChildProcesses bool                `yaml:"show_child_processes,omitempty" toml:"show_child_processes" jsonschema:"description=Show child processes in pane list" jsonschema_extras:"x-layer=global,x-priority=71"`
	Groups             map[string]GroupRef `yaml:"groups,omitempty" toml:"groups,omitempty" jsonschema:"description=Workspace groups for multiple key prefixes"`
}

// GroupRef defines a workspace group with its own prefix.
type GroupRef struct {
	Prefix string `yaml:"prefix" toml:"prefix"`
}

// GroupState holds the dynamic session state for a workspace group.
type GroupState struct {
	Sessions   map[string]TmuxSessionConfig `yaml:"sessions"`
	LockedKeys []string                     `yaml:"locked_keys,omitempty"`
}

// TmuxSessionsFile represents the sessions file stored in ~/.config/grove/gmux/sessions.yml
// This is separate from grove.yml to avoid polluting version control with dynamic state
type TmuxSessionsFile struct {
	Sessions   map[string]TmuxSessionConfig `yaml:"sessions"`
	LockedKeys []string                     `yaml:"locked_keys,omitempty"`
	Groups     map[string]GroupState        `yaml:"groups,omitempty"`
}

// TmuxSessionConfig defines the configuration for a single session mapped to a key.
type TmuxSessionConfig struct {
	Path        string `yaml:"path"`
	Repository  string `yaml:"repository,omitempty"`
	Description string `yaml:"description,omitempty"`
}
