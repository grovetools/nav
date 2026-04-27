package manager

//go:generate sh -c "cd ../.. && go run ./tools/schema-generator/"

import (
	"github.com/grovetools/core/pkg/models"

	"github.com/grovetools/nav/pkg/api"
)

// Type aliases for the extracted nav binding types now living in core/pkg/models.
// These preserve backwards compatibility with all existing nav code.
type (
	TmuxSessionConfig = models.NavSessionConfig
	GroupState        = models.NavGroupState
	TmuxSessionsFile  = models.NavSessionsFile
)

// NavFeatures contains granular feature toggles that can override mode presets.
// Use pointers so we can distinguish between "not set" and "explicitly false".
type NavFeatures struct {
	Groups       *bool `yaml:"groups,omitempty" toml:"groups,omitempty" jsonschema:"description=Enable workspace groups for multiple key prefixes"`
	Ecosystems   *bool `yaml:"ecosystems,omitempty" toml:"ecosystems,omitempty" jsonschema:"description=Enable ecosystem navigation and focus mode"`
	Integrations *bool `yaml:"integrations,omitempty" toml:"integrations,omitempty" jsonschema:"description=Enable Grove integrations (Flow plans, Notebooks, Context)"`
	Worktrees    *bool `yaml:"worktrees,omitempty" toml:"worktrees,omitempty" jsonschema:"description=Enable Git worktree support and folding"`
}

// ResolvedFeatures contains the final boolean values for each feature after
// evaluating the mode preset and applying any granular overrides.
// It is a type alias for api.Features so the exported sessionizer package
// can consume the same struct without depending on internal/manager.
type ResolvedFeatures = api.Features

// TmuxConfig represents the 'nav' section in grove config (legacy name retained for compatibility).
// For backwards compatibility, the 'tmux' section is also supported.
// This struct only contains static configuration specific to nav itself.
// Project discovery is handled by grove-core's DiscoveryService.
type TmuxConfig struct {
	Mode               string              `yaml:"mode,omitempty" toml:"mode,omitempty" jsonschema:"description=Mode preset: 'bare' (pure sessionizer), 'advanced' (groups + worktrees), 'grove' (all features). Defaults to 'grove'.,enum=bare,enum=advanced,enum=grove" jsonschema_extras:"x-layer=global,x-priority=68"`
	Features           *NavFeatures        `yaml:"features,omitempty" toml:"features,omitempty" jsonschema:"description=Granular feature overrides that take precedence over mode preset"`
	Prefix             string              `yaml:"prefix,omitempty" toml:"prefix,omitempty" jsonschema:"description=Prefix key for nav bindings. Options: '<prefix>' (default), '<prefix> X' (sub-table under prefix), 'C-g' (dedicated root key), or '' (direct root with modifiers)." jsonschema_extras:"x-layer=global,x-priority=69"`
	DefaultIcon        string              `yaml:"default_icon,omitempty" toml:"default_icon,omitempty" jsonschema:"description=Icon for the default group. Defaults to home icon."`
	AvailableKeys      []string            `yaml:"available_keys" toml:"available_keys" jsonschema:"description=Keys available for tmux pane shortcuts" jsonschema_extras:"x-layer=global,x-priority=70,x-important=true"`
	ShowChildProcesses bool                `yaml:"show_child_processes,omitempty" toml:"show_child_processes" jsonschema:"description=Show child processes in pane list" jsonschema_extras:"x-layer=global,x-priority=71"`
	Groups             map[string]GroupRef `yaml:"groups,omitempty" toml:"groups,omitempty" jsonschema:"description=Workspace groups for multiple key prefixes"`
	ConfirmKeyUpdates  *bool               `yaml:"confirm_key_updates,omitempty" toml:"confirm_key_updates,omitempty" jsonschema:"description=Show confirmation prompts for bulk key update operations (L/U). Defaults to true." jsonschema_extras:"x-layer=global,x-priority=72"`
}

// GroupRef defines a workspace group with its own prefix.
type GroupRef struct {
	Prefix   string                       `yaml:"prefix" toml:"prefix"`
	Icon     string                       `yaml:"icon,omitempty" toml:"icon,omitempty"`
	Persist  interface{}                  `yaml:"persist,omitempty" toml:"persist,omitempty"`
	Sessions map[string]TmuxSessionConfig `yaml:"sessions,omitempty" toml:"sessions,omitempty"`
	Active   *bool                        `yaml:"active,omitempty" toml:"active,omitempty"`
	Order    int                          `yaml:"order,omitempty" toml:"order,omitempty"` // Display order in group list
}

// Note: GroupState, TmuxSessionsFile, and TmuxSessionConfig are now type aliases
// for models.NavGroupState, models.NavSessionsFile, and models.NavSessionConfig
// defined at the top of this file. The canonical types live in core/pkg/models/nav.go.
