// Package config owns nav's grove.toml configuration. nav reads the full
// ecosystem config at startup (core/config.LoadDefault) and pulls its own
// settings out of the shared `[nav]` extension section via
// cfg.UnmarshalExtension("nav", &NavConfig{}) — the same pattern grove-anthropic,
// notify, and tend use for their sections. Keeping the struct here means nav
// settings live in nav, with zero coupling to core's Config type.
package config

// NavConfig holds the settings for the `[nav]` section of grove.toml.
//
// UnmarshalExtension decodes the section with mapstructure using the `yaml`
// struct tag, so `yaml` is the authoritative tag; `toml`/`mapstructure`/`json`
// are mirrored for consistency with the on-disk grove.toml and any future
// JSON-schema tooling.
type NavConfig struct {
	// GitDiffCommand is the editor command/args template used to open a
	// changed file as a diff split from the git-changes overlay. The
	// {{base}} placeholder is substituted with the overlay's diff base
	// ("main" for since-main, "" for working tree). Empty falls back to
	// the default "+Gvdiffsplit {{base}}".
	GitDiffCommand string `yaml:"git_diff_command" toml:"git_diff_command" mapstructure:"git_diff_command" json:"git_diff_command"`
}
