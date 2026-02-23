package manager

//go:generate sh -c "cd ../.. && go run ./tools/schema-generator/"

// TmuxConfig represents the 'nav' section in grove config (legacy name retained for compatibility).
// For backwards compatibility, the 'tmux' section is also supported.
// This struct only contains static configuration specific to nav itself.
// Project discovery is handled by grove-core's DiscoveryService.
type TmuxConfig struct {
	Prefix              string              `yaml:"prefix,omitempty" toml:"prefix,omitempty" jsonschema:"description=Prefix key for nav bindings. Options: '<prefix>' (default), '<prefix> X' (sub-table under prefix), 'C-g' (dedicated root key), or '' (direct root with modifiers)." jsonschema_extras:"x-layer=global,x-priority=69"`
	AvailableKeys       []string            `yaml:"available_keys" toml:"available_keys" jsonschema:"description=Keys available for tmux pane shortcuts" jsonschema_extras:"x-layer=global,x-priority=70,x-important=true"`
	ShowChildProcesses  bool                `yaml:"show_child_processes,omitempty" toml:"show_child_processes" jsonschema:"description=Show child processes in pane list" jsonschema_extras:"x-layer=global,x-priority=71"`
	Groups              map[string]GroupRef `yaml:"groups,omitempty" toml:"groups,omitempty" jsonschema:"description=Workspace groups for multiple key prefixes"`
	ConfirmKeyUpdates *bool `yaml:"confirm_key_updates,omitempty" toml:"confirm_key_updates,omitempty" jsonschema:"description=Show confirmation prompts for bulk key update operations (L/U). Defaults to true." jsonschema_extras:"x-layer=global,x-priority=72"`
}

// GroupRef defines a workspace group with its own prefix.
type GroupRef struct {
	Prefix   string                       `yaml:"prefix" toml:"prefix"`
	Icon     string                       `yaml:"icon,omitempty" toml:"icon,omitempty"`
	Persist  interface{}                  `yaml:"persist,omitempty" toml:"persist,omitempty"`
	Sessions map[string]TmuxSessionConfig `yaml:"sessions,omitempty" toml:"sessions,omitempty"`
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
// Supports both shorthand (o = "/path") and full table (o = { path = "/path" }) formats.
type TmuxSessionConfig struct {
	Path string `yaml:"path" toml:"path"`
}

// UnmarshalTOML implements custom unmarshaling to support shorthand string format.
// Accepts both: o = "/path" and o = { path = "/path" }
func (t *TmuxSessionConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case string:
		t.Path = v
		return nil
	case map[string]interface{}:
		if path, ok := v["path"].(string); ok {
			t.Path = path
		}
		return nil
	default:
		return nil
	}
}

// UnmarshalYAML implements custom unmarshaling to support shorthand string format.
// Accepts both: o: "/path" and o: { path: "/path" }
func (t *TmuxSessionConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try string first (shorthand)
	var path string
	if err := unmarshal(&path); err == nil {
		t.Path = path
		return nil
	}

	// Fall back to struct (full format)
	type plain TmuxSessionConfig
	return unmarshal((*plain)(t))
}
