// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// HistoryKeyMap defines the key bindings for the session history TUI.
type HistoryKeyMap struct {
	keymap.Base
	Open   key.Binding
	Filter key.Binding
}

func (k HistoryKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Open, k.Quit}
}

func (k HistoryKeyMap) FullHelp() [][]key.Binding {
	sections := k.Sections()
	result := make([][]key.Binding, len(sections))
	for i, s := range sections {
		result[i] = s.Bindings
	}
	return result
}

// Sections returns grouped sections of key bindings for the full help view.
// Only includes bindings that the history TUI actually implements.
func (k HistoryKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(
			k.Up, k.Down,
			key.NewBinding(key.WithKeys("g"), key.WithHelp("g + 1-9", "jump to row")),
		),
		keymap.ActionsSection(k.Filter, k.Open, k.CopyPath),
		keymap.SystemSection(k.Help, k.Quit),
	}
}

// NewHistoryKeyMap creates a new history keymap with user configuration applied.
// Base bindings (navigation, actions, search, selection) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewHistoryKeyMap(cfg *config.Config) HistoryKeyMap {
	km := HistoryKeyMap{
		Base: keymap.Load(cfg, "nav.history"),
		Open: key.NewBinding(
			key.WithKeys("o", "enter"),
			key.WithHelp("enter/o", "switch to session"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nav", "history", &km)

	return km
}

// HistoryKeymapInfo returns the keymap metadata for the nav history TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func HistoryKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-history",
		"nav",
		"Session history browser",
		NewHistoryKeyMap(nil),
	)
}
