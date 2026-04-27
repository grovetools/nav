// Package keymap contains exported keymap definitions for nav TUIs.
package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// SessionizeKeyMap defines the key bindings for the sessionize TUI
type SessionizeKeyMap struct {
	keymap.Base
	EditKey              key.Binding
	ClearKey             key.Binding
	CloseSession         key.Binding
	FocusEcosystem       key.Binding
	OpenEcosystem        key.Binding
	FocusEcosystemCursor key.Binding
	FocusEcosystemCwd    key.Binding
	ClearFocus           key.Binding
	ToggleWorktrees      key.Binding
	NextGroup            key.Binding
	PrevGroup            key.Binding
	FilterGroup          key.Binding
	ManageGroups         key.Binding
	NewGroup             key.Binding
	MapToGroup           key.Binding
	GoToMappingCursor    key.Binding
	GoToMappingCwd       key.Binding
	ToggleGitStatus      key.Binding
	ToggleBranch         key.Binding
	ToggleNoteCounts     key.Binding
	TogglePlanStats      key.Binding
	TogglePaths          key.Binding
	FilterDirty          key.Binding
	RefreshProjects      key.Binding
	ToggleHotContext     key.Binding
	ToggleHold           key.Binding
	ToggleRelease        key.Binding
	ToggleBinary         key.Binding
	ToggleLink           key.Binding
	ToggleCx             key.Binding
	ToggleTaskResults    key.Binding
	JumpBack             key.Binding
	JumpForward          key.Binding
	Undo                 key.Binding
	Redo                 key.Binding
}

func (k SessionizeKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.FocusEcosystem, k.Help, k.Quit}
}

func (k SessionizeKeyMap) FullHelp() [][]key.Binding {
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
			k.JumpBack,
			k.JumpForward,
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Session Management")),
			k.RefreshProjects,
			k.Select,
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
			k.FilterDirty,
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Groups")),
			k.NextGroup,
			k.PrevGroup,
			k.FilterGroup,
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "History")),
			k.Undo,
			k.Redo,
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Context Management")),
			k.ToggleHotContext,
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Column Toggles")),
			k.ToggleCx,
			k.ToggleGitStatus,
			k.ToggleBranch,
			k.ToggleNoteCounts,
			k.TogglePlanStats,
			k.ToggleHold,
			k.TogglePaths,
			k.ToggleRelease,
			k.ToggleBinary,
			k.ToggleLink,
			k.ToggleTaskResults,
		},
	}
}

// Sections returns grouped sections of key bindings for the full help view.
// Only includes sections that the sessionize TUI actually implements.
func (k SessionizeKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		keymap.NavigationSection(
			k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom,
			k.Search,
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select session")),
			k.JumpBack,
			k.JumpForward,
		),
		keymap.SelectionSection(k.Select, k.SelectAll, k.SelectNone),
		keymap.NewSection("Session",
			k.RefreshProjects,
			k.EditKey,
			k.ClearKey,
			k.CopyPath,
			k.CloseSession,
		),
		keymap.NewSection("Focus",
			k.FocusEcosystem,
			k.OpenEcosystem,
			k.FocusEcosystemCursor,
			k.FocusEcosystemCwd,
			k.GoToMappingCursor,
			k.GoToMappingCwd,
			k.ClearFocus,
			k.ToggleWorktrees,
			k.FilterDirty,
			k.ToggleHotContext,
		),
		keymap.NewSection("Groups",
			k.NextGroup,
			k.PrevGroup,
			k.FilterGroup,
			k.ManageGroups,
			k.NewGroup,
			k.MapToGroup,
		),
		keymap.NewSection("History",
			k.Undo,
			k.Redo,
		),
		keymap.NewSection("Columns",
			k.ToggleCx,
			k.ToggleGitStatus,
			k.ToggleBranch,
			k.ToggleNoteCounts,
			k.TogglePlanStats,
			k.ToggleHold,
			k.TogglePaths,
			k.ToggleRelease,
			k.ToggleBinary,
			k.ToggleLink,
			k.ToggleTaskResults,
		),
		k.FoldSection(),
		keymap.SystemSection(k.Help, k.Quit),
	}
}

// NewSessionizeKeyMap creates a new sessionize keymap with user configuration applied.
// Base bindings (navigation, actions, search, selection) come from keymap.Load().
// Only TUI-specific bindings are defined here.
func NewSessionizeKeyMap(cfg *config.Config) SessionizeKeyMap {
	km := SessionizeKeyMap{
		Base: keymap.Load(cfg, "nav.sessionize"),
		EditKey: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit key mapping"),
		),
		ClearKey: key.NewBinding(
			key.WithKeys("x", "ctrl+x"),
			key.WithHelp("x", "clear key mapping"),
		),
		CloseSession: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "close session"),
		),
		FocusEcosystem: key.NewBinding(
			key.WithKeys("@"),
			key.WithHelp("@", "focus ecosystem"),
		),
		OpenEcosystem: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open ecosystem"),
		),
		FocusEcosystemCursor: key.NewBinding(
			key.WithKeys("."),
			key.WithHelp(".", "focus cursor ecosystem"),
		),
		FocusEcosystemCwd: key.NewBinding(
			key.WithKeys(">"),
			key.WithHelp(">", "focus cwd ecosystem"),
		),
		ClearFocus: key.NewBinding(
			key.WithKeys("0"),
			key.WithHelp("0", "clear ecosystem focus"),
		),
		ToggleWorktrees: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "toggle worktrees"),
		),
		NextGroup: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next group"),
		),
		PrevGroup: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("S-tab", "prev group"),
		),
		FilterGroup: key.NewBinding(
			key.WithKeys("F"),
			key.WithHelp("F", "filter to group"),
		),
		ManageGroups: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "manage groups"),
		),
		NewGroup: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "new group"),
		),
		MapToGroup: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "map to group"),
		),
		GoToMappingCursor: key.NewBinding(
			key.WithKeys(","),
			key.WithHelp(",", "go to cursor mapping"),
		),
		GoToMappingCwd: key.NewBinding(
			key.WithKeys("<"),
			key.WithHelp("<", "go to cwd mapping"),
		),
		ToggleGitStatus: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "toggle git status"),
		),
		ToggleBranch: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "toggle branch names"),
		),
		ToggleNoteCounts: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "toggle note counts"),
		),
		TogglePlanStats: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "toggle flow plans"),
		),
		TogglePaths: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "toggle full paths"),
		),
		FilterDirty: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "filter dirty"),
		),
		RefreshProjects: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh project list"),
		),
		ToggleHotContext: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "toggle context"),
		),
		ToggleHold: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "toggle on-hold"),
		),
		ToggleRelease: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "toggle release"),
		),
		ToggleBinary: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "toggle tool/version"),
		),
		ToggleLink: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "toggle remote"),
		),
		ToggleCx: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "toggle cx column"),
		),
		ToggleTaskResults: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "toggle validation matrix"),
		),
		JumpBack: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("C-o", "jump back"),
		),
		JumpForward: key.NewBinding(
			key.WithKeys("ctrl+i", "ctrl+]"),
			key.WithHelp("C-i/C-]", "jump forward"),
		),
		Undo: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "undo data change"),
		),
		Redo: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("C-r", "redo data change"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "nav", "sessionize", &km)

	return km
}

// SessionizeKeymapInfo returns the keymap metadata for the nav sessionize TUI.
// Used by the grove keys registry generator to aggregate all TUI keybindings.
func SessionizeKeymapInfo() keymap.TUIInfo {
	return keymap.MakeTUIInfo(
		"nav-sessionize",
		"nav",
		"Tmux session switcher and workspace browser",
		NewSessionizeKeyMap(nil),
	)
}
