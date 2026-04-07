package navapp

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
)

// switchToTab lazily initializes the target tab (invoking its factory
// the first time) and returns the sub-model's Init cmd. When the target
// tab was already initialized, switchToTab fires the configured on-reentry
// refresh hook instead.
func (m *Model) switchToTab(t Tab) tea.Cmd {
	if m.initialized[t] {
		switch t {
		case TabSessionize:
			if m.sessionize != nil && m.cfg.OnReenterSessionize != nil {
				return m.cfg.OnReenterSessionize()
			}
		case TabKeymanage:
			if m.keymanage != nil && m.cfg.OnReenterKeymanage != nil {
				m.cfg.OnReenterKeymanage()
			}
		case TabGroups:
			if m.groups != nil && m.cfg.OnReenterGroups != nil {
				m.cfg.OnReenterGroups()
			}
		}
		return nil
	}

	m.initialized[t] = true

	var cmd tea.Cmd
	switch t {
	case TabSessionize:
		if m.cfg.NewSessionize != nil {
			m.sessionize = m.cfg.NewSessionize()
		}
		if m.sessionize != nil {
			cmd = m.sessionize.Init()
			m.forwardSizeTo(TabSessionize)
		}
	case TabKeymanage:
		if m.cfg.NewKeymanage != nil {
			m.keymanage = m.cfg.NewKeymanage()
		}
		if m.keymanage != nil {
			cmd = m.keymanage.Init()
			m.forwardSizeTo(TabKeymanage)
		}
	case TabHistory:
		if m.cfg.NewHistory != nil {
			m.history = m.cfg.NewHistory()
		}
		if m.history != nil {
			cmd = m.history.Init()
			m.forwardSizeTo(TabHistory)
		}
	case TabWindows:
		if m.cfg.NewWindows != nil {
			m.windows = m.cfg.NewWindows()
		}
		if m.windows != nil {
			cmd = m.windows.Init()
			m.forwardSizeTo(TabWindows)
		}
	case TabGroups:
		if m.cfg.NewGroups != nil {
			m.groups = m.cfg.NewGroups()
		}
		// groups.Model has no Init cmd — just forward size.
		if m.groups != nil {
			m.forwardSizeTo(TabGroups)
		}
	}
	return cmd
}

// forwardSizeTo pushes the current tea.WindowSizeMsg down to a single
// sub-model after it was freshly initialized. The outer Update handler
// already fans size messages to every live sub-model.
func (m *Model) forwardSizeTo(t Tab) {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	child := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
	switch t {
	case TabSessionize:
		if m.sessionize != nil {
			updated, _ := m.sessionize.Update(child)
			if sm, ok := updated.(*sessionizer.Model); ok {
				m.sessionize = sm
			}
		}
	case TabKeymanage:
		if m.keymanage != nil {
			updated, _ := m.keymanage.Update(child)
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
			}
		}
	case TabHistory:
		if m.history != nil {
			updated, _ := m.history.Update(child)
			if hm, ok := updated.(*history.Model); ok {
				m.history = hm
			}
		}
	case TabWindows:
		if m.windows != nil {
			updated, _ := m.windows.Update(child)
			if wm, ok := updated.(*windows.Model); ok {
				m.windows = wm
			}
		}
	case TabGroups:
		if m.groups != nil {
			updated, _ := m.groups.Update(child)
			if gm, ok := updated.(*groups.Model); ok {
				m.groups = gm
			}
		}
	}
}

// availableTabs returns the tab order used by Prev/Next rotation. A tab
// is included only if the host supplied a factory for it (or an already-
// initialized sub-model is non-nil).
func (m *Model) availableTabs() []Tab {
	order := []Tab{TabSessionize, TabKeymanage, TabHistory, TabWindows, TabGroups}
	out := make([]Tab, 0, len(order))
	for _, t := range order {
		if m.tabAvailable(t) {
			out = append(out, t)
		}
	}
	return out
}

// Update is the meta-panel's main event loop. It handles window-size
// fan-out, cross-TUI message routing, tab navigation, and forwarding
// everything else to the active sub-model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Subtract 4/4 for the outer padding + tab bar (matches the
		// pre-extraction behavior from cmd/nav/nav_tui.go).
		child := tea.WindowSizeMsg{Width: msg.Width - 4, Height: msg.Height - 4}
		if m.sessionize != nil {
			updated, _ := m.sessionize.Update(child)
			if sm, ok := updated.(*sessionizer.Model); ok {
				m.sessionize = sm
			}
		}
		if m.keymanage != nil {
			updated, _ := m.keymanage.Update(child)
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
			}
		}
		if m.history != nil {
			updated, _ := m.history.Update(child)
			if hm, ok := updated.(*history.Model); ok {
				m.history = hm
			}
		}
		if m.windows != nil {
			updated, _ := m.windows.Update(child)
			if wm, ok := updated.(*windows.Model); ok {
				m.windows = wm
			}
		}
		if m.groups != nil {
			updated, _ := m.groups.Update(child)
			if gm, ok := updated.(*groups.Model); ok {
				m.groups = gm
			}
		}
		return m, nil

	case switchTabMsg:
		return m, m.doSwitchTab(msg.to)

	case embed.SetWorkspaceMsg:
		// Workspace changes must reach every initialized sub-model, not
		// just the active one — otherwise tabs the user hasn't opened
		// yet operate on a stale workspace pointer when they're later
		// focused. Fan out to each live sub-model the same way
		// tea.WindowSizeMsg does.
		var cmds []tea.Cmd
		if m.sessionize != nil {
			updated, cmd := m.sessionize.Update(msg)
			if sm, ok := updated.(*sessionizer.Model); ok {
				m.sessionize = sm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.keymanage != nil {
			updated, cmd := m.keymanage.Update(msg)
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.history != nil {
			updated, cmd := m.history.Update(msg)
			if hm, ok := updated.(*history.Model); ok {
				m.history = hm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.windows != nil {
			updated, cmd := m.windows.Update(msg)
			if wm, ok := updated.(*windows.Model); ok {
				m.windows = wm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.groups != nil {
			updated, cmd := m.groups.Update(msg)
			if gm, ok := updated.(*groups.Model); ok {
				m.groups = gm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	// ---- Cross-TUI routing ------------------------------------------------

	case sessionizer.RequestMapKeyMsg:
		cmd1 := m.doSwitchTab(TabKeymanage)
		var cmd2 tea.Cmd
		if m.keymanage != nil {
			updated, cmd := m.keymanage.Update(keymanage.RequestMapKeyMsg{Project: msg.Project})
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
			}
			cmd2 = cmd
		}
		return m, tea.Batch(cmd1, cmd2)

	case sessionizer.BulkMappingDoneMsg:
		cmd1 := m.doSwitchTab(TabKeymanage)
		var cmd2 tea.Cmd
		if m.keymanage != nil {
			updated, cmd := m.keymanage.Update(keymanage.BulkMappingDoneMsg{MappedKeys: msg.MappedKeys})
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
			}
			cmd2 = cmd
		}
		return m, tea.Batch(cmd1, cmd2)

	case sessionizer.RequestManageGroupsMsg, keymanage.RequestManageGroupsMsg:
		return m, requestSwitchTab(TabGroups)

	case keymanage.CancelMappingMsg:
		return m, requestSwitchTab(TabSessionize)

	case keymanage.JumpToSessionizeMsg:
		cmd := m.doSwitchTab(TabSessionize)
		if m.sessionize != nil {
			m.sessionize.JumpToPath(msg.Path, msg.ApplyGroupFilter)
		}
		return m, cmd

	case history.JumpToSessionizeMsg:
		cmd := m.doSwitchTab(TabSessionize)
		if m.sessionize != nil {
			m.sessionize.JumpToPath(msg.Path, msg.ApplyGroupFilter)
		}
		return m, cmd

	case windows.LoadedMsg, windows.PreviewLoadedMsg:
		// These land on the windows sub-model regardless of which tab
		// is currently focused (they're async results from the driver).
		if m.windows != nil {
			updated, cmd := m.windows.Update(msg)
			if wm, ok := updated.(*windows.Model); ok {
				m.windows = wm
			}
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if cmd, handled := m.handleGlobalKey(msg); handled {
			return m, cmd
		}
	}

	// Route everything else to the active sub-model.
	switch m.activeTab {
	case TabSessionize:
		if m.sessionize != nil {
			updated, cmd := m.sessionize.Update(msg)
			if sm, ok := updated.(*sessionizer.Model); ok {
				m.sessionize = sm
			}
			return m, cmd
		}
	case TabKeymanage:
		if m.keymanage != nil {
			updated, cmd := m.keymanage.Update(msg)
			if km, ok := updated.(*keymanage.Model); ok {
				m.keymanage = km
				// keymanage signals "open groups next" via NextCommand.
				if km.NextCommand() == "groups" {
					km.ClearNextCommand()
					return m, requestSwitchTab(TabGroups)
				}
			}
			return m, cmd
		}
	case TabHistory:
		if m.history != nil {
			updated, cmd := m.history.Update(msg)
			if hm, ok := updated.(*history.Model); ok {
				m.history = hm
			}
			return m, cmd
		}
	case TabWindows:
		if m.windows != nil {
			updated, cmd := m.windows.Update(msg)
			if wm, ok := updated.(*windows.Model); ok {
				m.windows = wm
			}
			return m, cmd
		}
	case TabGroups:
		if m.groups != nil {
			updated, cmd := m.groups.Update(msg)
			if gm, ok := updated.(*groups.Model); ok {
				m.groups = gm
				// groups signals "return to keymanage" via NextCommand.
				if gm.NextCommand() == "km" {
					gm.ClearNextCommand()
					return m, requestSwitchTab(TabKeymanage)
				}
			}
			return m, cmd
		}
	}

	return m, nil
}

// doSwitchTab handles the pre-switch bookkeeping (clearing keymanage's
// pending map state if we're switching away from it) and then delegates
// to switchToTab. Mirrors the behavior of the pre-extraction
// switchViewMsg handler.
func (m *Model) doSwitchTab(to Tab) tea.Cmd {
	if m.activeTab == TabKeymanage && to != TabKeymanage &&
		m.keymanage != nil && m.keymanage.PendingMapProject() != nil {
		m.keymanage.ClearPendingMapProject()
	}
	m.activeTab = to
	return m.switchToTab(to)
}

// handleGlobalKey inspects a tea.KeyMsg for the meta-panel's tab
// navigation bindings ('1'..'5', '['/']'). If the key is handled,
// returns (cmd, true); otherwise (nil, false) and the caller forwards
// the key to the active sub-model.
func (m *Model) handleGlobalKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.isTextInputFocused() {
		return nil, false
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return nil, false
	}
	r := msg.Runes[0]

	if target, ok := m.keys.JumpTabs[r]; ok {
		if !m.tabAvailable(target) || target == m.activeTab {
			return nil, true
		}
		return requestSwitchTab(target), true
	}

	if m.keys.Next != 0 && r == m.keys.Next {
		return m.cycleTab(+1), true
	}
	if m.keys.Prev != 0 && r == m.keys.Prev {
		return m.cycleTab(-1), true
	}
	return nil, false
}

// cycleTab rotates to the neighbour of the active tab within the set of
// currently-available tabs.
func (m *Model) cycleTab(dir int) tea.Cmd {
	order := m.availableTabs()
	if len(order) == 0 {
		return nil
	}
	currIdx := 0
	for i, t := range order {
		if t == m.activeTab {
			currIdx = i
			break
		}
	}
	n := len(order)
	nextIdx := (currIdx + dir + n) % n
	next := order[nextIdx]
	if next == m.activeTab {
		return nil
	}
	return requestSwitchTab(next)
}
