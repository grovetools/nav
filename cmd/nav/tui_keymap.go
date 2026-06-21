package main

import (
	"github.com/grovetools/core/config"

	navconfig "github.com/grovetools/nav/pkg/config"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// loadConfig loads the user configuration for keybinding overrides.
// Returns nil on error (use defaults).
func loadConfig() *config.Config {
	cfg, _ := config.LoadDefault()
	return cfg
}

// loadNavConfig parses nav's own settings from the [nav] section of
// grove.toml via the extension mechanism. Missing/invalid sections yield a
// zero-valued NavConfig (defaults applied downstream).
func loadNavConfig() navconfig.NavConfig {
	var navCfg navconfig.NavConfig
	if cfg := loadConfig(); cfg != nil {
		_ = cfg.UnmarshalExtension("nav", &navCfg)
	}
	return navCfg
}

var (
	sessionizeKeys = navkeymap.NewSessionizeKeyMap(loadConfig())
	manageKeys     = navkeymap.NewManageKeyMap(loadConfig())
	historyKeys    = navkeymap.NewHistoryKeyMap(loadConfig())
	windowsKeys    = navkeymap.NewWindowsKeyMap(loadConfig())
	groupsKeys     = navkeymap.NewGroupsKeyMap(loadConfig())

	// navUserConfig holds nav's [nav] grove.toml settings, resolved once at
	// startup and threaded into the sessionizer Config.
	navUserConfig = loadNavConfig()
)
