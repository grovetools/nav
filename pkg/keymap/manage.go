// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// ManageKeyMap defines the key bindings for the session key manager TUI.
type ManageKeyMap struct {
	keymap.Base
	Toggle      key.Binding
	Edit        key.Binding // Overrides Base.Edit with "map CWD" behavior
	SetKey      key.Binding
	Open        key.Binding
	Delete      key.Binding // Overrides Base.Delete with "clear mapping" behavior
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
	sections := k.Sections()
	result := make([][]key.Binding, len(sections))
	for i, s := range sections {
		result[i] = s.Bindings
	}
	return result
}

// Sections returns grouped sections of key bindings for the full help view.
// Only includes bindings that the manage TUI actually implements.
func (k ManageKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(
			k.Up, k.Down,
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "jump to row")),
			k.Open,
		),
		keymap.ActionsSection(k.Edit, k.SetKey, k.Toggle, k.Delete, k.Save, k.CopyPath),
		keymap.NewSection("Reorder",
			k.MoveMode, k.Lock,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move row (in move mode)")),
			k.ConfirmMove,
		),
		keymap.ViewSection(k.TogglePaths),
		keymap.SystemSection(k.Help, k.Quit),
	}
}

// NewManageKeyMap creates a new manage keymap with user configuration applied.
// Base bindings (navigation, actions, search, selection) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewManageKeyMap(cfg *config.Config) ManageKeyMap {
	km := ManageKeyMap{
		Base: keymap.Load(cfg, "nav.manage"),
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

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nav", "manage", &km)

	return km
}

// ManageKeymapInfo returns the keymap metadata for the nav session key manager TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func ManageKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-manage",
		"nav",
		"Session hotkey manager",
		NewManageKeyMap(nil),
	)
}
