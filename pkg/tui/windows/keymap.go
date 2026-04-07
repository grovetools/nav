package windows

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is the set of keybindings used by the windows TUI. It mirrors
// navkeymap.WindowsKeyMap so callers can construct it from their own
// *config.Config or accept the default returned by DefaultKeyMap.
type KeyMap = navkeymap.WindowsKeyMap

// DefaultKeyMap returns the windows keymap built from the default nav
// config.
func DefaultKeyMap() KeyMap {
	return navkeymap.NewWindowsKeyMap(nil)
}
