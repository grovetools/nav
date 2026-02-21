package cmd

import (
	"github.com/grovetools/core/tui/keymap"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// SessionizeKeymapInfo returns the keymap metadata for the nav sessionize TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func SessionizeKeymapInfo() keymap.TUIInfo {
	return navkeymap.SessionizeKeymapInfo()
}
