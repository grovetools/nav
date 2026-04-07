package groups

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is the set of keybindings used by the groups TUI. It mirrors
// navkeymap.GroupsKeyMap so callers can construct it from their own
// *config.Config or accept the default returned by DefaultKeyMap.
type KeyMap = navkeymap.GroupsKeyMap

// DefaultKeyMap returns the groups keymap built from the default nav
// config.
func DefaultKeyMap() KeyMap {
	return navkeymap.NewGroupsKeyMap(nil)
}
