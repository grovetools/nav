package keymanage

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is a type alias for the shared nav manage key bindings. The
// standalone nav binary passes navkeymap.NewManageKeyMap(config) via
// Config so keybinding overrides from the user's config apply; terminal
// embedders can supply their own KeyMap.
type KeyMap = navkeymap.ManageKeyMap

// DefaultKeyMap returns the zero-config default keybindings. Callers
// that want user config overrides applied should use
// navkeymap.NewManageKeyMap(config) instead.
func DefaultKeyMap() KeyMap {
	return navkeymap.NewManageKeyMap(nil)
}
