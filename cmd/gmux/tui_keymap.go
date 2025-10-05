package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/mattsolo1/grove-core/tui/keymap"
)

// sessionizeKeyMap defines the key bindings for the sessionize TUI
type sessionizeKeyMap struct {
	keymap.Base
	EditKey         key.Binding
	ClearKey        key.Binding
	CopyPath        key.Binding
	CloseSession    key.Binding
	FocusEcosystem  key.Binding
	ClearFocus      key.Binding
	ToggleWorktrees key.Binding
	ToggleGitStatus key.Binding
	ToggleClaude    key.Binding
	TogglePaths     key.Binding
}

func (k sessionizeKeyMap) ShortHelp() []key.Binding {
	// Return empty to show no help in footer - all help goes in popup
	return []key.Binding{}
}

func (k sessionizeKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			key.NewBinding(key.WithKeys("j/k, ↑/↓"), key.WithHelp("j/k, ↑/↓", "Move up/down")),
			key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "Page up")),
			key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "Page down")),
			key.NewBinding(key.WithKeys("gg"), key.WithHelp("gg", "Go to top")),
			key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "Go to bottom")),
			key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "Start filtering")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Create/switch session")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Session Management")),
			k.EditKey,
			k.ClearKey,
			k.CopyPath,
			k.CloseSession,
			k.Help,
			k.Quit,
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Focus & View")),
			k.FocusEcosystem,
			k.ClearFocus,
			k.ToggleWorktrees,
			k.ToggleGitStatus,
			k.ToggleClaude,
			k.TogglePaths,
		},
	}
}

var sessionizeKeys = sessionizeKeyMap{
	Base: keymap.NewBase(),
	EditKey: key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "edit key mapping"),
	),
	ClearKey: key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "clear key mapping"),
	),
	CopyPath: key.NewBinding(
		key.WithKeys("ctrl+y"),
		key.WithHelp("ctrl+y", "copy path to clipboard"),
	),
	CloseSession: key.NewBinding(
		key.WithKeys("X"),
		key.WithHelp("X", "close session"),
	),
	FocusEcosystem: key.NewBinding(
		key.WithKeys("@"),
		key.WithHelp("@", "focus on project"),
	),
	ClearFocus: key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "clear focus"),
	),
	ToggleWorktrees: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "toggle worktrees"),
	),
	ToggleGitStatus: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "toggle git status"),
	),
	ToggleClaude: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "toggle claude sessions"),
	),
	TogglePaths: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "toggle full paths"),
	),
}
