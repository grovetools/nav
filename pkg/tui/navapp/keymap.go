package navapp

// KeyMap is the meta-panel's tab-navigation binding set. It is
// intentionally small: the only keys the meta-panel itself intercepts
// are the tab jumps and the prev/next cyclers. Everything else falls
// through to the active sub-TUI.
type KeyMap struct {
	// JumpTabs maps a rune (e.g. '1'..'5') to a destination tab. When
	// the user presses the rune and no text input is focused, the
	// meta-panel switches to the mapped tab.
	JumpTabs map[rune]Tab
	// Next cycles to the next available tab.
	Next rune
	// Prev cycles to the previous available tab.
	Prev rune
}

// DefaultKeyMap returns the standalone nav CLI's historical bindings:
// '1'..'5' jump to sessionize / keymanage / history / windows / groups,
// and '[' / ']' cycle through the tabs. Tabs whose factory is nil are
// hidden in the available-tab list, so a jump to an unavailable tab is
// silently ignored.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		JumpTabs: map[rune]Tab{
			'1': TabSessionize,
			'2': TabKeymanage,
			'3': TabHistory,
			'4': TabWindows,
			'5': TabGroups,
		},
		Next: ']',
		Prev: '[',
	}
}

// isZero reports whether the KeyMap was left as its zero value — used by
// New() to decide whether to apply the default bindings.
func (k KeyMap) isZero() bool {
	return k.JumpTabs == nil && k.Next == 0 && k.Prev == 0
}
