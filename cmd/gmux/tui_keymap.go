package main

import (
	"github.com/grovetools/core/config"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// Re-export keymap types from the library package for use in this main package
type sessionizeKeyMap = navkeymap.SessionizeKeyMap
type manageKeyMap = navkeymap.ManageKeyMap
type historyKeyMap = navkeymap.HistoryKeyMap
type windowsKeyMap = navkeymap.WindowsKeyMap

// loadConfig loads the user configuration for keybinding overrides.
// Returns nil on error (use defaults).
func loadConfig() *config.Config {
	cfg, _ := config.LoadDefault()
	return cfg
}

var sessionizeKeys = navkeymap.NewSessionizeKeyMap(loadConfig())
var manageKeys = navkeymap.NewManageKeyMap()
var historyKeys = navkeymap.NewHistoryKeyMap()
var windowsKeys = navkeymap.NewWindowsKeyMap()
