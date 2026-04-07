package navapp

import tea "github.com/charmbracelet/bubbletea"

// switchTabMsg is the meta-panel's internal tab-change signal. It is
// produced by tea.Cmds returned from tab-navigation keybindings (see
// handleGlobalKey) and by cross-TUI routing (e.g. the keymanage CancelMapping
// handler) so that switches are applied during the next Update cycle.
type switchTabMsg struct {
	to Tab
}

// requestSwitchTab returns a tea.Cmd that, when executed, emits a
// switchTabMsg targeting the given tab. Using a cmd (rather than
// synchronously mutating activeTab) lets the current Update call
// finish processing whatever message triggered the switch before the
// new tab is focused.
func requestSwitchTab(to Tab) tea.Cmd {
	return func() tea.Msg { return switchTabMsg{to: to} }
}
