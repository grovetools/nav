// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/tui/keymap"
)

// HistoryKeyMap defines the key bindings for the session history TUI.
type HistoryKeyMap struct {
	keymap.Base
	Up     key.Binding
	Down   key.Binding
	Open   key.Binding
	Filter key.Binding
}

func (k HistoryKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Open, k.Quit}
}

func (k HistoryKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up,
			k.Down,
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "jump to row")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Filter,
			k.Open,
			k.Help,
			k.Quit,
		},
	}
}

// Sections returns grouped sections of key bindings for the full help view.
func (k HistoryKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		{
			Name: "Navigation",
			Bindings: []key.Binding{
				k.Up,
				k.Down,
				key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "jump to row")),
			},
		},
		{
			Name:     "Actions",
			Bindings: []key.Binding{k.Filter, k.Open},
		},
		k.Base.SystemSection(),
	}
}

// NewHistoryKeyMap creates a new history keymap with default bindings.
func NewHistoryKeyMap() HistoryKeyMap {
	return HistoryKeyMap{
		Base: keymap.NewBase(),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Open: key.NewBinding(
			key.WithKeys("o", "enter"),
			key.WithHelp("enter/o", "switch to session"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
	}
}

// HistoryKeymapInfo returns the keymap metadata for the nav history TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func HistoryKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-history",
		"nav",
		"Session history browser",
		NewHistoryKeyMap(),
	)
}
