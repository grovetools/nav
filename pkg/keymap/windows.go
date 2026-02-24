// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// WindowsKeyMap defines the key bindings for the window manager TUI.
type WindowsKeyMap struct {
	keymap.Base
	Switch   key.Binding
	Filter   key.Binding
	Rename   key.Binding
	Close    key.Binding
	MoveMode key.Binding
	MoveUp   key.Binding
	MoveDown key.Binding
}

func (k WindowsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit}
}

func (k WindowsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up, k.Down,
			key.NewBinding(key.WithKeys("g"), key.WithHelp("g + 0-9", "jump to window")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Switch, k.Filter, k.Rename, k.Close, k.Help, k.Quit,
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Reorder")),
			k.MoveMode,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move (in move mode)")),
		},
	}
}

// Sections returns grouped sections of key bindings for the full help view.
func (k WindowsKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(
			k.Up, k.Down,
			key.NewBinding(key.WithKeys("g"), key.WithHelp("g + 0-9", "jump to window")),
		),
		keymap.ActionsSection(k.Switch, k.Filter, k.Rename, k.Close),
		keymap.NewSection("Reorder",
			k.MoveMode,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move (in move mode)")),
		),
		keymap.SystemSection(k.Help, k.Quit),
	}
}

// NewWindowsKeyMap creates a new windows keymap with user configuration applied.
// Base bindings (navigation, actions, search, selection) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewWindowsKeyMap(cfg *config.Config) WindowsKeyMap {
	km := WindowsKeyMap{
		Base: keymap.Load(cfg, "nav.windows"),
		Switch: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "switch"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Rename: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "rename"),
		),
		Close: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "close"),
		),
		MoveMode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "move window"),
		),
		MoveUp: key.NewBinding(
			key.WithKeys("k"),
			key.WithHelp("k", "move up"),
		),
		MoveDown: key.NewBinding(
			key.WithKeys("j"),
			key.WithHelp("j", "move down"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nav", "windows", &km)

	return km
}

// WindowsKeymapInfo returns the keymap metadata for the nav windows TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func WindowsKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-windows",
		"nav",
		"Tmux window manager",
		NewWindowsKeyMap(nil),
	)
}
