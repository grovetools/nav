// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/tui/keymap"
)

// ManageKeyMap defines the key bindings for the session key manager TUI.
type ManageKeyMap struct {
	keymap.Base
	Up          key.Binding
	Down        key.Binding
	Toggle      key.Binding
	Edit        key.Binding
	SetKey      key.Binding
	Open        key.Binding
	Delete      key.Binding
	Save        key.Binding
	MoveMode    key.Binding
	Lock        key.Binding
	MoveUp      key.Binding
	MoveDown    key.Binding
	ConfirmMove key.Binding
	TogglePaths key.Binding
}

func (k ManageKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MoveMode, k.Lock, k.TogglePaths, k.Quit}
}

func (k ManageKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up,
			k.Down,
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "Jump to row")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Switch to session")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Edit,
			k.SetKey,
			k.Toggle,
			k.Delete,
			k.Save,
			k.Help,
			k.Quit,
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Reorder")),
			k.MoveMode,
			k.Lock,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move row (in move mode)")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm move")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "View")),
			k.TogglePaths,
		},
	}
}

// Sections returns grouped sections of key bindings for the full help view.
func (k ManageKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		{
			Name: "Navigation",
			Bindings: []key.Binding{
				k.Up,
				k.Down,
				key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "jump to row")),
				key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch to session")),
			},
		},
		{
			Name:     "Actions",
			Bindings: []key.Binding{k.Edit, k.SetKey, k.Toggle, k.Delete, k.Save},
		},
		{
			Name: "Reorder",
			Bindings: []key.Binding{
				k.MoveMode,
				k.Lock,
				key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move row (in move mode)")),
				key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm move")),
			},
		},
		{
			Name:     "View",
			Bindings: []key.Binding{k.TogglePaths},
		},
		k.Base.SystemSection(),
	}
}

// NewManageKeyMap creates a new manage keymap with default bindings.
func NewManageKeyMap() ManageKeyMap {
	return ManageKeyMap{
		Base: keymap.NewBase(),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "quick toggle"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "map CWD"),
		),
		SetKey: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "set key mode"),
		),
		Open: key.NewBinding(
			key.WithKeys("o", "enter"),
			key.WithHelp("enter/o", "switch to session"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "delete"),
			key.WithHelp("d/del", "clear mapping"),
		),
		Save: key.NewBinding(
			key.WithKeys("s", "ctrl+s"),
			key.WithHelp("s/ctrl+s", "save & exit"),
		),
		MoveMode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "enter move mode"),
		),
		Lock: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "toggle lock"),
		),
		MoveUp: key.NewBinding(
			key.WithKeys("k"),
			key.WithHelp("k", "move up"),
		),
		MoveDown: key.NewBinding(
			key.WithKeys("j"),
			key.WithHelp("j", "move down"),
		),
		ConfirmMove: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm move"),
		),
		TogglePaths: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "toggle paths"),
		),
	}
}

// ManageKeymapInfo returns the keymap metadata for the nav session key manager TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func ManageKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-manage",
		"nav",
		"Session hotkey manager",
		NewManageKeyMap(),
	)
}
