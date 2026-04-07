package history

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is the set of keybindings used by the history TUI. It mirrors
// navkeymap.HistoryKeyMap so callers can construct it from their own
// *config.Config or accept the default returned by DefaultKeyMap.
type KeyMap = navkeymap.HistoryKeyMap

// DefaultKeyMap returns the history keymap built from the default nav
// config.
func DefaultKeyMap() KeyMap {
	return navkeymap.NewHistoryKeyMap(nil)
}
