package sessionizer

import (
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is the set of keybindings used by the sessionizer TUI.
// It mirrors the SessionizeKeyMap defined in nav/pkg/keymap so callers
// can construct it from their own *config.Config (or accept the default
// returned by DefaultKeyMap).
type KeyMap = navkeymap.SessionizeKeyMap

// DefaultKeyMap returns the sessionizer keymap built from the default
// nav config. Callers that want user overrides should construct it via
// navkeymap.NewSessionizeKeyMap(cfg) directly.
func DefaultKeyMap() KeyMap {
	return navkeymap.NewSessionizeKeyMap(nil)
}
