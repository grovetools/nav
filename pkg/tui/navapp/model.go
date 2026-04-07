// Package navapp hosts the nav TUI meta-panel: a tabbed multiplexer that
// routes between the five nav sub-TUIs (sessionizer, keymanage, history,
// windows, groups) and handles the cross-TUI message passing that used to
// live in nav/cmd/nav/nav_tui.go.
//
// It is a reusable bubbletea model: the standalone nav CLI wraps it in a
// tea.Program for its interactive subcommands, and the grove terminal
// multiplexer can embed it directly as a unified Nav panel.
//
// All dependencies (the five sub-model factories, on-reentry hooks, and
// initial tab) are supplied via the Config struct. The package itself
// holds no global state.
package navapp

import (
	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"

	tea "github.com/charmbracelet/bubbletea"
)

// Tab identifies one of the five sub-TUIs the meta-panel can display.
type Tab int

const (
	// TabSessionize is the project picker / sessionizer.
	TabSessionize Tab = iota
	// TabKeymanage is the key-binding manager.
	TabKeymanage
	// TabHistory is the recently-accessed project list.
	TabHistory
	// TabWindows is the tmux windows browser.
	TabWindows
	// TabGroups is the workspace group editor.
	TabGroups
)

// SessionizeFactory lazily builds the sessionizer sub-model on first
// access. Returning nil signals the tab is unavailable.
type SessionizeFactory func() *sessionizer.Model

// KeymanageFactory lazily builds the keymanage sub-model.
type KeymanageFactory func() *keymanage.Model

// HistoryFactory lazily builds the history sub-model.
type HistoryFactory func() *history.Model

// WindowsFactory lazily builds the windows sub-model. A nil factory (or
// one that returns nil) hides the Windows tab entirely — this is how the
// standalone CLI suppresses it when running outside tmux.
type WindowsFactory func() *windows.Model

// GroupsFactory lazily builds the groups sub-model.
type GroupsFactory func() *groups.Model

// Config collects every dependency the meta-panel needs from its host.
// Factories are called exactly once, the first time their tab is shown.
type Config struct {
	// InitialTab controls which tab is visible on startup.
	InitialTab Tab

	// Sub-TUI factories. A nil factory disables the corresponding tab.
	NewSessionize SessionizeFactory
	NewKeymanage  KeymanageFactory
	NewHistory    HistoryFactory
	NewWindows    WindowsFactory
	NewGroups     GroupsFactory

	// OnReenterSessionize is invoked every time the user switches back to
	// an already-initialized sessionize tab. Hosts use this to refresh
	// data that might have changed while the tab was hidden. May be nil.
	OnReenterSessionize func() tea.Cmd

	// OnReenterKeymanage is invoked every time the user switches back to
	// an already-initialized keymanage tab. May be nil.
	OnReenterKeymanage func()

	// OnReenterGroups is invoked every time the user switches back to an
	// already-initialized groups tab. May be nil.
	OnReenterGroups func()

	// KeyMap overrides the default tab-navigation keymap. Zero value
	// uses DefaultKeyMap().
	KeyMap KeyMap
}

// Model is the meta-panel tea.Model. It owns the five sub-model pointers
// and the active-tab state.
type Model struct {
	cfg Config

	activeTab Tab

	sessionize *sessionizer.Model
	keymanage  *keymanage.Model
	history    *history.Model
	windows    *windows.Model
	groups     *groups.Model

	// initialized tracks which tabs have had their factory called. A tab
	// may be "initialized" with a nil sub-model if its factory returned
	// nil (e.g. windows with no tmux client); re-entry then skips it.
	initialized map[Tab]bool

	width, height int
	keys          KeyMap
}

// New constructs a Model from the given Config. Sub-models are not built
// eagerly — they come up the first time their tab is selected.
func New(cfg Config) *Model {
	keys := cfg.KeyMap
	if keys.isZero() {
		keys = DefaultKeyMap()
	}
	return &Model{
		cfg:         cfg,
		activeTab:   cfg.InitialTab,
		initialized: make(map[Tab]bool),
		keys:        keys,
	}
}

// ActiveTab reports which tab the meta-panel is currently displaying.
// Callers use this after the program exits to decide which sub-model's
// result to inspect.
func (m *Model) ActiveTab() Tab { return m.activeTab }

// Sessionize returns the sessionizer sub-model, or nil if the tab was
// never opened. Used by hosts to extract the user's selection on exit.
func (m *Model) Sessionize() *sessionizer.Model { return m.sessionize }

// Keymanage returns the keymanage sub-model, or nil if never opened.
func (m *Model) Keymanage() *keymanage.Model { return m.keymanage }

// History returns the history sub-model, or nil if never opened.
func (m *Model) History() *history.Model { return m.history }

// Windows returns the windows sub-model, or nil if never opened (or if
// the host supplied a nil WindowsFactory).
func (m *Model) Windows() *windows.Model { return m.windows }

// Groups returns the groups sub-model, or nil if never opened.
func (m *Model) Groups() *groups.Model { return m.groups }

// Close releases resources owned by every initialized sub-model. Hosts
// must call it on shutdown so background goroutines (e.g. the
// sessionizer's daemon SSE listener) don't leak between Model lifetimes.
func (m *Model) Close() error {
	var firstErr error
	if m.sessionize != nil {
		if err := m.sessionize.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.keymanage != nil {
		if err := m.keymanage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.history != nil {
		if err := m.history.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.windows != nil {
		if err := m.windows.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.groups != nil {
		if err := m.groups.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// tabAvailable reports whether a given tab can be shown — i.e. the host
// supplied a factory (or, for an already-initialized tab, a non-nil
// sub-model came out of it).
func (m *Model) tabAvailable(t Tab) bool {
	if m.initialized[t] {
		switch t {
		case TabSessionize:
			return m.sessionize != nil
		case TabKeymanage:
			return m.keymanage != nil
		case TabHistory:
			return m.history != nil
		case TabWindows:
			return m.windows != nil
		case TabGroups:
			return m.groups != nil
		}
		return false
	}
	switch t {
	case TabSessionize:
		return m.cfg.NewSessionize != nil
	case TabKeymanage:
		return m.cfg.NewKeymanage != nil
	case TabHistory:
		return m.cfg.NewHistory != nil
	case TabWindows:
		return m.cfg.NewWindows != nil
	case TabGroups:
		return m.cfg.NewGroups != nil
	}
	return false
}

// isTextInputFocused reports whether any text input in the active sub-tui
// is currently focused. Hosts use it to decide whether a global key
// stroke should be intercepted for tab navigation or forwarded as text.
func (m *Model) isTextInputFocused() bool {
	switch m.activeTab {
	case TabSessionize:
		if m.sessionize != nil {
			return m.sessionize.IsTextInputFocused()
		}
	case TabKeymanage:
		if m.keymanage != nil {
			return m.keymanage.IsTextInputFocused()
		}
	case TabHistory:
		if m.history != nil {
			return m.history.FilterMode()
		}
	case TabWindows:
		if m.windows != nil {
			mode := m.windows.Mode()
			return mode == "filter" || mode == "rename"
		}
	case TabGroups:
		if m.groups != nil {
			return m.groups.InputMode() != ""
		}
	}
	return false
}

// IsTextInputFocused is the host-facing wrapper around isTextInputFocused.
func (m *Model) IsTextInputFocused() bool { return m.isTextInputFocused() }

// Init initializes the starting tab and returns its init cmd.
func (m *Model) Init() tea.Cmd {
	return m.switchToTab(m.activeTab)
}
