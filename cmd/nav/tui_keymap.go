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

var sessionizeKeys = navkeymap.NewSessionizeKeyMap(loadConfig())
var manageKeys = navkeymap.NewManageKeyMap(loadConfig())
var historyKeys = navkeymap.NewHistoryKeyMap(loadConfig())
var windowsKeys = navkeymap.NewWindowsKeyMap(loadConfig())
var groupsKeys = navkeymap.NewGroupsKeyMap(loadConfig())
