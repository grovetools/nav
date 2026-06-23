package main

import (
	"github.com/grovetools/core/config"

	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// loadConfig loads the user configuration for keybinding overrides.
// Returns nil on error (use defaults).
func loadConfig() *config.Config {
	cfg, _ := config.LoadDefault()
	return cfg
}

var (
	sessionizeKeys = navkeymap.NewSessionizeKeyMap(loadConfig())
	manageKeys     = navkeymap.NewManageKeyMap(loadConfig())
	historyKeys    = navkeymap.NewHistoryKeyMap(loadConfig())
	windowsKeys    = navkeymap.NewWindowsKeyMap(loadConfig())
	groupsKeys     = navkeymap.NewGroupsKeyMap(loadConfig())
)
