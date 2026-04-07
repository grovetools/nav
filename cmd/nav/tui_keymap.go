package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/config"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// Re-export keymap types from the library package for use in this main package.
// These aliases are used by the other nav TUIs (key manage, history, groups,
// windows) that have not yet been extracted out of package main.
type sessionizeKeyMap = navkeymap.SessionizeKeyMap
type manageKeyMap = navkeymap.ManageKeyMap
type historyKeyMap = navkeymap.HistoryKeyMap
type windowsKeyMap = navkeymap.WindowsKeyMap
type groupsKeyMap = navkeymap.GroupsKeyMap

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

// pageStyle is the default lipgloss style used by the remaining cmd/nav
// TUIs (history, groups). The sessionizer view maintains its own copy.
var pageStyle = lipgloss.NewStyle()
