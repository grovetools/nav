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

// HistoryKeymapInfo returns the keymap metadata for the nav history TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func HistoryKeymapInfo() keymap.TUIInfo {
	return navkeymap.HistoryKeymapInfo()
}

// ManageKeymapInfo returns the keymap metadata for the nav manage TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func ManageKeymapInfo() keymap.TUIInfo {
	return navkeymap.ManageKeymapInfo()
}

// WindowsKeymapInfo returns the keymap metadata for the nav windows TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func WindowsKeymapInfo() keymap.TUIInfo {
	return navkeymap.WindowsKeymapInfo()
}
