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

// tabFromID maps a human-readable tab ID string (matching the PageWithID
// values in pages.go) to the corresponding Tab constant.
func tabFromID(id string) Tab {
	switch id {
	case "sessionize":
		return TabSessionize
	case "keymanage":
		return TabKeymanage
	case "history":
		return TabHistory
	case "windows":
		return TabWindows
	case "groups":
		return TabGroups
	default:
		return TabSessionize
	}
}

// requestSwitchTab returns a tea.Cmd that emits an embed.SwitchTabMsg
// targeting the given tab. Using a cmd lets the current Update call
// finish processing before the new tab is focused.
func requestSwitchTab(to Tab) tea.Cmd {
	return func() tea.Msg { return embed.SwitchTabMsg{TabIndex: int(to)} }
}

// Update is the meta-panel's main event loop. It handles cross-TUI
// message routing above the pager and delegates everything else
// (window sizing, tab navigation keys, active-page forwarding) to
// the pager.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		m.pager, cmd = m.pager.Update(msg)
		return m, cmd

	case embed.SwitchTabMsg:
		tab := Tab(msg.TabIndex)
		if msg.TabID != "" {
			tab = tabFromID(msg.TabID)
		}
		return m, m.doSwitchTab(tab)

	case embed.SetWorkspaceMsg:
		// Workspace changes must reach every initialized sub-model, not
		// just the active one — otherwise tabs the user hasn't opened
		// yet operate on a stale workspace pointer when they're later
		// focused.
		var cmds []tea.Cmd
		if m.state.sessionize != nil {
			updated, cmd := m.state.sessionize.Update(msg)
			if sm, ok := updated.(*sessionizer.Model); ok {
				m.state.sessionize = sm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.state.keymanage != nil {
			updated, cmd := m.state.keymanage.Update(msg)
			if km, ok := updated.(*keymanage.Model); ok {
				m.state.keymanage = km
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.state.history != nil {
			updated, cmd := m.state.history.Update(msg)
			if hm, ok := updated.(*history.Model); ok {
				m.state.history = hm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.state.windows != nil {
			updated, cmd := m.state.windows.Update(msg)
			if wm, ok := updated.(*windows.Model); ok {
				m.state.windows = wm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.state.groups != nil {
			updated, cmd := m.state.groups.Update(msg)
			if gm, ok := updated.(*groups.Model); ok {
				m.state.groups = gm
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
		if m.state.keymanage != nil {
			updated, cmd := m.state.keymanage.Update(keymanage.RequestMapKeyMsg{Project: msg.Project})
			if km, ok := updated.(*keymanage.Model); ok {
				m.state.keymanage = km
			}
			cmd2 = cmd
		}
		return m, tea.Batch(cmd1, cmd2)

	case sessionizer.BulkMappingDoneMsg:
		cmd1 := m.doSwitchTab(TabKeymanage)
		var cmd2 tea.Cmd
		if m.state.keymanage != nil {
			updated, cmd := m.state.keymanage.Update(keymanage.BulkMappingDoneMsg{MappedKeys: msg.MappedKeys})
			if km, ok := updated.(*keymanage.Model); ok {
				m.state.keymanage = km
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
		if m.state.sessionize != nil {
			m.state.sessionize.JumpToPath(msg.Path, msg.ApplyGroupFilter)
		}
		return m, cmd

	case history.JumpToSessionizeMsg:
		cmd := m.doSwitchTab(TabSessionize)
		if m.state.sessionize != nil {
			m.state.sessionize.JumpToPath(msg.Path, msg.ApplyGroupFilter)
		}
		return m, cmd

	case windows.LoadedMsg, windows.PreviewLoadedMsg:
		// These land on the windows sub-model regardless of which tab
		// is currently focused (they're async results from the driver).
		if m.state.windows != nil {
			updated, cmd := m.state.windows.Update(msg)
			if wm, ok := updated.(*windows.Model); ok {
				m.state.windows = wm
			}
			return m, cmd
		}
		return m, nil
	}

	// Default: delegate to pager (handles key navigation + active page routing).
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)

	// Post-pager NextCommand checks — sub-models signal cross-tab
	// transitions via a string field inspected after every Update.
	switch Tab(m.pager.ActiveIndex()) {
	case TabKeymanage:
		if m.state.keymanage != nil && m.state.keymanage.NextCommand() == "groups" {
			m.state.keymanage.ClearNextCommand()
			return m, tea.Batch(cmd, requestSwitchTab(TabGroups))
		}
	case TabGroups:
		if m.state.groups != nil && m.state.groups.NextCommand() == "km" {
			m.state.groups.ClearNextCommand()
			return m, tea.Batch(cmd, requestSwitchTab(TabKeymanage))
		}
	}

	return m, cmd
}

// doSwitchTab handles the pre-switch bookkeeping (clearing keymanage's
// pending map state if we're switching away from it) and then delegates
// to the pager via embed.SwitchTabMsg.
func (m *Model) doSwitchTab(to Tab) tea.Cmd {
	if Tab(m.pager.ActiveIndex()) == TabKeymanage && to != TabKeymanage &&
		m.state.keymanage != nil && m.state.keymanage.PendingMapProject() != nil {
		m.state.keymanage.ClearPendingMapProject()
	}
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(embed.SwitchTabMsg{TabIndex: int(to)})
	return cmd
}
