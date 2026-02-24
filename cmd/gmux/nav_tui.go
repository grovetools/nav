package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tmux"
)

// navView represents which view is currently active
type navView int

const (
	viewSessionize navView = iota
	viewManage
	viewHistory
	viewWindows
	viewGroups
)

// switchViewMsg signals a view switch
type switchViewMsg struct {
	to navView
}

// NavTUIOptions contains options for initializing the nav TUI
type NavTUIOptions struct {
	CwdFocusPath string // Path to focus on in sessionize view
}

// navModel is the root model that manages view switching between all nav TUIs
type navModel struct {
	activeView      navView
	sessionizeModel *sessionizeModel
	manageModel     *manageModel
	historyModel    *historyModel
	windowsModel    *windowsModel
	groupsModel     *groupsModel
	manager         *tmux.Manager
	client          *tmuxclient.Client // May be nil if not in tmux
	width, height   int
	configDir       string
	cwd             string
	opts            NavTUIOptions
	initialized     map[navView]bool // Tracks which views have been initialized
}

// isTextInputFocused returns true if any text input is focused in the active view
func (m *navModel) isTextInputFocused() bool {
	switch m.activeView {
	case viewSessionize:
		if m.sessionizeModel != nil {
			return m.sessionizeModel.filterInput.Focused() || m.sessionizeModel.editingKeys
		}
	case viewManage:
		if m.manageModel != nil {
			return m.manageModel.setKeyMode || m.manageModel.saveToGroupNewMode || m.manageModel.newGroupMode
		}
	case viewHistory:
		if m.historyModel != nil {
			return m.historyModel.filterMode
		}
	case viewWindows:
		if m.windowsModel != nil {
			return m.windowsModel.mode == "filter" || m.windowsModel.mode == "rename"
		}
	case viewGroups:
		if m.groupsModel != nil {
			return m.groupsModel.inputMode != ""
		}
	}
	return false
}

// switchToView initializes a view lazily if needed and returns its Init() command
func (m *navModel) switchToView(view navView) tea.Cmd {
	// Initialize the map if needed
	if m.initialized == nil {
		m.initialized = make(map[navView]bool)
	}

	// Check if already initialized
	if m.initialized[view] {
		// For manage/groups, refresh data when switching back
		switch view {
		case viewManage:
			if m.manageModel != nil {
				sessions, _ := m.manager.GetSessions()
				m.manageModel.sessions = sessions
				m.manageModel.rebuildSessionsOrder()
				m.manageModel.message = fmt.Sprintf("Switched to group: %s", m.manager.GetActiveGroup())
			}
		case viewGroups:
			if m.groupsModel != nil {
				m.groupsModel.groups = m.manager.GetAllGroups()
				m.groupsModel.cursor = 0
				m.groupsModel.message = ""
			}
		}
		return nil
	}

	// Mark as initialized
	m.initialized[view] = true

	var cmd tea.Cmd

	switch view {
	case viewSessionize:
		// Initialize sessionize model lazily
		if m.sessionizeModel == nil {
			usedCache := false
			var projects []manager.SessionizeProject

			// Try to load from cache first
			if cache, err := manager.LoadProjectCache(m.configDir); err == nil && cache != nil && len(cache.Projects) > 0 {
				projects = make([]manager.SessionizeProject, len(cache.Projects))
				for i, cached := range cache.Projects {
					projects[i] = manager.SessionizeProject{
						WorkspaceNode: cached.WorkspaceNode,
						GitStatus:     cached.GitStatus,
						NoteCounts:    cached.NoteCounts,
						PlanStats:     cached.PlanStats,
						ReleaseInfo:   cached.ReleaseInfo,
						ActiveBinary:  cached.ActiveBinary,
						CxStats:       cached.CxStats,
						GitRemoteURL:  cached.GitRemoteURL,
					}
				}
				usedCache = true
			}

			// If no cache, fetch projects
			if len(projects) == 0 {
				fetchedProjects, err := m.manager.GetAvailableProjects()
				if err == nil && len(fetchedProjects) > 0 {
					projects = fetchedProjects
				}
			}

			if len(projects) > 0 {
				projectPtrs := make([]*manager.SessionizeProject, len(projects))
				for i := range projects {
					projectPtrs[i] = &projects[i]
				}
				searchPaths, _ := m.manager.GetEnabledSearchPaths()
				sm := newSessionizeModel(projectPtrs, searchPaths, m.manager, m.configDir, usedCache, m.opts.CwdFocusPath)
				m.sessionizeModel = &sm
			}
		}
		if m.sessionizeModel != nil {
			cmd = m.sessionizeModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.sessionizeModel.Update(childMsg)
				if sm, ok := newModel.(sessionizeModel); ok {
					m.sessionizeModel = &sm
				}
			}
		}

	case viewManage:
		// Initialize manage model lazily
		if m.manageModel == nil {
			sessions, _ := m.manager.GetSessions()
			enrichedProjects := make(map[string]*manager.SessionizeProject)
			usedCache := false

			// Try to load enriched projects cache
			if cache, err := manager.LoadKeyManageCache(m.configDir); err == nil && cache != nil && len(cache.EnrichedProjects) > 0 {
				for path, cached := range cache.EnrichedProjects {
					enrichedProjects[path] = &manager.SessionizeProject{
						WorkspaceNode: cached.WorkspaceNode,
						GitStatus:     cached.GitStatus,
						NoteCounts:    cached.NoteCounts,
						PlanStats:     cached.PlanStats,
					}
				}
				usedCache = len(enrichedProjects) > 0
			}

			mm := newManageModel(sessions, m.manager, m.cwd, enrichedProjects, usedCache)
			m.manageModel = &mm
		}
		if m.manageModel != nil {
			cmd = m.manageModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.manageModel.Update(childMsg)
				if mm, ok := newModel.(*manageModel); ok {
					m.manageModel = mm
				}
			}
		}

	case viewHistory:
		// Initialize history model lazily
		if m.historyModel == nil {
			history, err := m.manager.GetAccessHistory()
			if err == nil && history != nil && len(history.Projects) > 0 {
				sessions, _ := m.manager.GetSessions()
				keyMap := make(map[string]string)
				for _, s := range sessions {
					if s.Path != "" {
						keyMap[s.Path] = s.Key
					}
				}

				// Build history items
				var historyAccesses []*workspace.ProjectAccess
				for _, access := range history.Projects {
					historyAccesses = append(historyAccesses, access)
				}
				sort.Slice(historyAccesses, func(i, j int) bool {
					return historyAccesses[i].LastAccessed.After(historyAccesses[j].LastAccessed)
				})

				var items []historyItem
				for _, access := range historyAccesses {
					if len(items) >= 15 {
						break
					}
					node, err := workspace.GetProjectByPath(access.Path)
					if err != nil {
						node = &workspace.WorkspaceNode{Path: access.Path, Name: filepath.Base(access.Path)}
					}
					proj := &manager.SessionizeProject{WorkspaceNode: node}
					items = append(items, historyItem{project: proj, access: access})
				}
				hm := newHistoryModel(items, m.manager, keyMap)
				m.historyModel = hm
			}
		}
		if m.historyModel != nil {
			cmd = m.historyModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.historyModel.Update(childMsg)
				if hm, ok := newModel.(*historyModel); ok {
					m.historyModel = hm
				}
			}
		}

	case viewWindows:
		// Initialize windows model lazily
		if m.windowsModel == nil && m.client != nil {
			ctx := context.Background()
			if currentSession, err := m.client.GetCurrentSession(ctx); err == nil && currentSession != "" {
				showChildProcesses := false
				if tmuxCfg, err := loadTmuxConfig(); err == nil && tmuxCfg != nil {
					showChildProcesses = tmuxCfg.ShowChildProcesses
				}
				wm := newWindowsModel(m.client, currentSession, showChildProcesses)
				m.windowsModel = &wm
			}
		}
		if m.windowsModel != nil {
			cmd = m.windowsModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.windowsModel.Update(childMsg)
				if wm, ok := newModel.(windowsModel); ok {
					m.windowsModel = &wm
				}
			}
		}

	case viewGroups:
		// Initialize groups model lazily
		if m.groupsModel == nil {
			gm := newGroupsModel(m.manager)
			m.groupsModel = &gm
		}
		// groupsModel doesn't have an Init that returns commands
		if m.groupsModel != nil && m.width > 0 && m.height > 0 {
			childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
			newModel, _ := m.groupsModel.Update(childMsg)
			if gm, ok := newModel.(*groupsModel); ok {
				m.groupsModel = gm
			}
		}
	}

	return cmd
}

func (m *navModel) Init() tea.Cmd {
	// Initialize only the active view lazily
	return m.switchToView(m.activeView)
}

func (m *navModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward adjusted size to all initialized models
		// Subtract 4 for Width (2 left padding + 2 right padding)
		// Subtract 4 for Height (1 top padding + 1 bottom padding + 2 lines for tab bar)
		childMsg := tea.WindowSizeMsg{Width: msg.Width - 4, Height: msg.Height - 4}
		if m.sessionizeModel != nil {
			newModel, _ := m.sessionizeModel.Update(childMsg)
			if sm, ok := newModel.(sessionizeModel); ok {
				m.sessionizeModel = &sm
			}
		}
		if m.manageModel != nil {
			newModel, _ := m.manageModel.Update(childMsg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
		}
		if m.historyModel != nil {
			newModel, _ := m.historyModel.Update(childMsg)
			if hm, ok := newModel.(*historyModel); ok {
				m.historyModel = hm
			}
		}
		if m.windowsModel != nil {
			newModel, _ := m.windowsModel.Update(childMsg)
			if wm, ok := newModel.(windowsModel); ok {
				m.windowsModel = &wm
			}
		}
		if m.groupsModel != nil {
			newModel, _ := m.groupsModel.Update(childMsg)
			if gm, ok := newModel.(*groupsModel); ok {
				m.groupsModel = gm
			}
		}
		return m, nil

	case switchViewMsg:
		m.activeView = msg.to
		// Use lazy initialization - switchToView handles refresh for already-initialized views
		cmd := m.switchToView(msg.to)
		return m, cmd

	// Route background data messages to their owners regardless of active view
	case initialProjectsEnrichedMsg:
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			return m, cmd
		}
		return m, nil

	case rulesStateMsg:
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			return m, cmd
		}
		return m, nil

	case gitStatusMapMsg:
		// Route to both sessionize and manage models
		var cmds []tea.Cmd
		if m.sessionizeModel != nil {
			newModel, cmd := m.sessionizeModel.Update(msg)
			if sm, ok := newModel.(sessionizeModel); ok {
				m.sessionizeModel = &sm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.historyModel != nil {
			newModel, cmd := m.historyModel.Update(msg)
			if hm, ok := newModel.(*historyModel); ok {
				m.historyModel = hm
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case noteCountsMapMsg, planStatsMapMsg:
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			return m, cmd
		}
		return m, nil

	case cwdProjectEnrichedMsg:
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			return m, cmd
		}
		return m, nil

	case windowsLoadedMsg, previewLoadedMsg:
		if m.windowsModel != nil {
			newModel, cmd := m.windowsModel.Update(msg)
			if wm, ok := newModel.(windowsModel); ok {
				m.windowsModel = &wm
			}
			return m, cmd
		}
		return m, nil

	case spinnerTickMsg:
		// Route spinner ticks to the active view
		switch m.activeView {
		case viewSessionize:
			if m.sessionizeModel != nil {
				newModel, cmd := m.sessionizeModel.Update(msg)
				if sm, ok := newModel.(sessionizeModel); ok {
					m.sessionizeModel = &sm
				}
				return m, cmd
			}
		case viewManage:
			if m.manageModel != nil {
				newModel, cmd := m.manageModel.Update(msg)
				if mm, ok := newModel.(*manageModel); ok {
					m.manageModel = mm
				}
				return m, cmd
			}
		case viewHistory:
			if m.historyModel != nil {
				newModel, cmd := m.historyModel.Update(msg)
				if hm, ok := newModel.(*historyModel); ok {
					m.historyModel = hm
				}
				return m, cmd
			}
		}
		return m, nil

	case tea.KeyMsg:
		// Check for global tab navigation (1-4) if no text input is focused
		if !m.isTextInputFocused() && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			switch r {
			case '1':
				if m.activeView != viewSessionize {
					return m, func() tea.Msg { return switchViewMsg{to: viewSessionize} }
				}
			case '2':
				if m.activeView != viewManage {
					return m, func() tea.Msg { return switchViewMsg{to: viewManage} }
				}
			case '3':
				if m.activeView != viewHistory {
					return m, func() tea.Msg { return switchViewMsg{to: viewHistory} }
				}
			case '4':
				if m.activeView != viewWindows && m.client != nil {
					return m, func() tea.Msg { return switchViewMsg{to: viewWindows} }
				}
			}
		}
	}

	// Route to active view
	switch m.activeView {
	case viewSessionize:
		if m.sessionizeModel != nil {
			newModel, cmd := m.sessionizeModel.Update(msg)
			if sm, ok := newModel.(sessionizeModel); ok {
				m.sessionizeModel = &sm
			}
			return m, cmd
		}

	case viewManage:
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
				// Check if manage wants to switch to groups
				if mm.nextCommand == "groups" {
					mm.nextCommand = ""
					return m, func() tea.Msg { return switchViewMsg{to: viewGroups} }
				}
			}
			return m, cmd
		}

	case viewHistory:
		if m.historyModel != nil {
			newModel, cmd := m.historyModel.Update(msg)
			if hm, ok := newModel.(*historyModel); ok {
				m.historyModel = hm
			}
			return m, cmd
		}

	case viewWindows:
		if m.windowsModel != nil {
			newModel, cmd := m.windowsModel.Update(msg)
			if wm, ok := newModel.(windowsModel); ok {
				m.windowsModel = &wm
			}
			return m, cmd
		}

	case viewGroups:
		if m.groupsModel != nil {
			newModel, cmd := m.groupsModel.Update(msg)
			if gm, ok := newModel.(*groupsModel); ok {
				m.groupsModel = gm
				// Check if groups wants to switch back to manage
				if gm.nextCommand == "km" {
					gm.nextCommand = ""
					return m, func() tea.Msg { return switchViewMsg{to: viewManage} }
				}
			}
			return m, cmd
		}
	}

	return m, nil
}

func (m *navModel) View() string {
	var b strings.Builder

	// Render global tab bar - tabs are always available, models are lazily initialized
	tabs := []struct {
		numIcon string
		name    string
		view    navView
		ok      bool
	}{
		{core_theme.IconNumeric1CircleOutline, "Sessionize", viewSessionize, true},
		{core_theme.IconNumeric2CircleOutline, "Key Manage", viewManage, true},
		{core_theme.IconNumeric3CircleOutline, "History", viewHistory, true},
		{core_theme.IconNumeric4CircleOutline, "Windows", viewWindows, m.client != nil}, // Windows requires tmux client
	}

	var tabParts []string
	for _, tab := range tabs {
		if !tab.ok {
			continue
		}

		if m.activeView == tab.view {
			// Active tab: Violet number icon, bold white text
			numStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultTheme.Colors.Violet).Bold(true)
			nameStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultTheme.Colors.LightText).Bold(true)
			tabParts = append(tabParts, fmt.Sprintf("%s %s", numStyle.Render(tab.numIcon), nameStyle.Render(tab.name)))
		} else {
			// Inactive tab: Keep number visible (not faint), text muted
			numStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultTheme.Colors.MutedText)
			nameStyle := core_theme.DefaultTheme.Muted
			tabParts = append(tabParts, fmt.Sprintf("%s %s", numStyle.Render(tab.numIcon), nameStyle.Render(tab.name)))
		}
	}

	// Join with generous spacing and a faint dot separator
	separator := core_theme.DefaultTheme.Muted.Faint(true).Render("  •  ")
	b.WriteString(strings.Join(tabParts, separator))
	b.WriteString("\n\n")

	// Render active view (prepend tab bar to child view)
	var childView string
	switch m.activeView {
	case viewSessionize:
		if m.sessionizeModel != nil {
			childView = m.sessionizeModel.View()
		}
	case viewManage:
		if m.manageModel != nil {
			childView = m.manageModel.View()
		}
	case viewHistory:
		if m.historyModel != nil {
			childView = m.historyModel.View()
		}
	case viewWindows:
		if m.windowsModel != nil {
			childView = m.windowsModel.View()
		}
	case viewGroups:
		if m.groupsModel != nil {
			childView = m.groupsModel.View()
		}
	}

	b.WriteString(childView)
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

// runNavTUI runs the unified nav TUI starting in manage view
func runNavTUI() error {
	return runNavTUIWithView(viewManage, NavTUIOptions{})
}

// runNavTUIWithView runs the unified nav TUI starting in the specified view
// Models are lazily initialized when their tab is first accessed
func runNavTUIWithView(startView navView, opts NavTUIOptions) error {
	mgr, err := tmux.NewManager(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Try to create tmux client (may fail if not in tmux)
	var client *tmuxclient.Client
	if os.Getenv("TMUX") != "" {
		client, _ = tmuxclient.NewClient()
	}

	// Auto-select group
	if targetGroup == "" {
		if last := mgr.GetLastAccessedGroup(); last != "" {
			mgr.SetActiveGroup(last)
		} else if matched := mgr.FindGroupForPath(cwd); matched != "" {
			mgr.SetActiveGroup(matched)
		} else {
			mgr.SetActiveGroup("default")
		}
	} else {
		mgr.SetActiveGroup(targetGroup)
	}

	// Create the unified nav model - models are lazily initialized
	m := &navModel{
		activeView:  startView,
		manager:     mgr,
		client:      client,
		configDir:   configDir,
		cwd:         cwd,
		opts:        opts,
		initialized: make(map[navView]bool),
	}

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	// Handle post-TUI logic
	if nm, ok := finalModel.(*navModel); ok {
		// Handle manage model exit
		if nm.manageModel != nil {
			mm := nm.manageModel
			if mm.changesMade {
				if err := mgr.UpdateSessionsAndLocks(mm.sessions, mm.getLockedKeysSlice()); err != nil {
					return fmt.Errorf("failed to save sessions: %w", err)
				}
				if err := mgr.RegenerateBindings(); err != nil {
					return fmt.Errorf("failed to regenerate bindings: %w", err)
				}
				_ = reloadTmuxConfig()
			}

			if mm.commandOnExit != nil {
				mm.commandOnExit.Stdin = os.Stdin
				mm.commandOnExit.Stdout = os.Stdout
				mm.commandOnExit.Stderr = os.Stderr
				_ = mm.commandOnExit.Run()
			}
		}

		// Handle sessionize model exit
		if nm.sessionizeModel != nil && nm.sessionizeModel.selected != nil {
			sm := nm.sessionizeModel
			if sm.selected.WorkspaceNode != nil && sm.selected.Path != "" {
				_ = mgr.RecordProjectAccess(sm.selected.Path)
				if sm.selected.IsWorktree() && sm.selected.ParentProjectPath != "" {
					_ = mgr.RecordProjectAccess(sm.selected.ParentProjectPath)
				}
				return sessionizeProject(sm.selected)
			}
		}

		// Handle history model exit
		if nm.historyModel != nil && nm.historyModel.selected != nil {
			hm := nm.historyModel
			_ = mgr.RecordProjectAccess(hm.selected.Path)
			return mgr.Sessionize(hm.selected.Path)
		}

		// Handle windows model exit
		if nm.windowsModel != nil && nm.windowsModel.selectedWindow != nil {
			wm := nm.windowsModel
			if client != nil {
				target := fmt.Sprintf("%s:%d", wm.sessionName, wm.selectedWindow.Index)
				if err := client.SwitchClient(context.Background(), target); err != nil {
					// This might fail if not in a popup, which is fine
				}
				_ = client.ClosePopupCmd().Run()
			}
		}
	}

	return nil
}


