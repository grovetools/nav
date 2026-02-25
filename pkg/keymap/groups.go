// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// GroupsKeyMap defines the key bindings for the group management TUI.
type GroupsKeyMap struct {
	keymap.Base
	New        key.Binding
	Delete     key.Binding // Override Base.Delete with single "d"
	Rename     key.Binding
	EditPrefix key.Binding
	MoveMode   key.Binding
	MoveUp     key.Binding
	MoveDown   key.Binding
	Toggle     key.Binding
	Select     key.Binding
	Undo       key.Binding
	Redo       key.Binding
}

func (k GroupsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k GroupsKeyMap) FullHelp() [][]key.Binding {
	sections := k.Sections()
	result := make([][]key.Binding, len(sections))
	for i, s := range sections {
		result[i] = s.Bindings
	}
	return result
}

// Sections returns grouped sections of key bindings for the full help view.
func (k GroupsKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(
			k.Up, k.Down,
			k.PageUp, k.PageDown,
			k.Top, k.Bottom,
			k.Select,
		),
		keymap.ActionsSection(
			k.New,
			k.Delete,
			k.Rename,
			k.EditPrefix,
		),
		keymap.NewSection("Reorder",
			k.MoveMode,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move row (in move mode)")),
		),
		keymap.NewSection("History",
			k.Undo,
			k.Redo,
		),
		keymap.SystemSection(k.Help, k.Quit),
	}
}

// NewGroupsKeyMap creates a new groups keymap with user configuration applied.
// Base bindings (navigation, actions, search, selection) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewGroupsKeyMap(cfg *config.Config) GroupsKeyMap {
	km := GroupsKeyMap{
		Base: keymap.Load(cfg, "nav.groups"),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new group"),
		),
		Delete: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "delete group"),
		),
		Rename: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "rename group"),
		),
		EditPrefix: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "edit prefix"),
		),
		MoveMode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "enter move mode"),
		),
		MoveUp: key.NewBinding(
			key.WithKeys("k"),
			key.WithHelp("k", "move up"),
		),
		MoveDown: key.NewBinding(
			key.WithKeys("j"),
			key.WithHelp("j", "move down"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "switch to group"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "switch to group"),
		),
		Undo: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "undo"),
		),
		Redo: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("C-r", "redo"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nav", "groups", &km)

	return km
}

// GroupsKeymapInfo returns the keymap metadata for the nav groups TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func GroupsKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-groups",
		"nav",
		"Workspace group manager",
		NewGroupsKeyMap(nil),
	)
}
