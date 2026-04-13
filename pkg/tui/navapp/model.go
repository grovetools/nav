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
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
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
}

// Model is the meta-panel tea.Model. It delegates tab navigation and
// rendering to the pager component and holds a shared navState for
// cross-tab message routing.
type Model struct {
	pager pager.Model
	state *navState
}

// New constructs a Model from the given Config. Sub-models are not built
// eagerly — they come up the first time their tab is selected.
func New(cfg Config) *Model {
	st := &navState{
		cfg:         cfg,
		initialized: make(map[Tab]bool),
	}
	pages := []pager.Page{
		&sessionizePage{s: st},
		&keymanagePage{s: st},
		&historyPage{s: st},
		&windowsPage{s: st},
		&groupsPage{s: st},
	}
	pg := pager.NewAt(pages, pager.DefaultKeyMap(), int(cfg.InitialTab))
	pg.SetConfig(pager.Config{
		OuterPadding: [4]int{1, 2, 1, 2},
		ShowTitleRow: true,
		FooterHeight: 1,
	})
	return &Model{
		pager: pg,
		state: st,
	}
}

// ActiveTab reports which tab the meta-panel is currently displaying.
// Callers use this after the program exits to decide which sub-model's
// result to inspect.
func (m *Model) ActiveTab() Tab { return Tab(m.pager.ActiveIndex()) }

// Sessionize returns the sessionizer sub-model, or nil if the tab was
// never opened. Used by hosts to extract the user's selection on exit.
func (m *Model) Sessionize() *sessionizer.Model { return m.state.sessionize }

// Keymanage returns the keymanage sub-model, or nil if never opened.
func (m *Model) Keymanage() *keymanage.Model { return m.state.keymanage }

// History returns the history sub-model, or nil if never opened.
func (m *Model) History() *history.Model { return m.state.history }

// Windows returns the windows sub-model, or nil if never opened (or if
// the host supplied a nil WindowsFactory).
func (m *Model) Windows() *windows.Model { return m.state.windows }

// Groups returns the groups sub-model, or nil if never opened.
func (m *Model) Groups() *groups.Model { return m.state.groups }

// Close releases resources owned by every initialized sub-model. Hosts
// must call it on shutdown so background goroutines (e.g. the
// sessionizer's daemon SSE listener) don't leak between Model lifetimes.
func (m *Model) Close() error {
	var firstErr error
	if m.state.sessionize != nil {
		if err := m.state.sessionize.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.state.keymanage != nil {
		if err := m.state.keymanage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.state.history != nil {
		if err := m.state.history.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.state.windows != nil {
		if err := m.state.windows.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if m.state.groups != nil {
		if err := m.state.groups.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// IsTextInputFocused reports whether the active sub-model has a focused
// text input. Hosts use this to decide whether a global key stroke
// should be intercepted or forwarded as text.
func (m *Model) IsTextInputFocused() bool {
	if p, ok := m.pager.Active().(pager.PageWithTextInput); ok {
		return p.IsTextEntryActive()
	}
	return false
}

// Init initializes the starting tab and returns its init cmd.
func (m *Model) Init() tea.Cmd {
	return m.pager.Init()
}
