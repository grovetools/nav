package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

func (m *navModel) Init() tea.Cmd {
	// Batch init commands from all models that are initialized
	var cmds []tea.Cmd

	if m.sessionizeModel != nil {
		cmds = append(cmds, m.sessionizeModel.Init())
	}
	if m.manageModel != nil {
		cmds = append(cmds, m.manageModel.Init())
	}
	if m.historyModel != nil {
		cmds = append(cmds, m.historyModel.Init())
	}
	if m.windowsModel != nil {
		cmds = append(cmds, m.windowsModel.Init())
	}
	if m.groupsModel != nil {
		// groupsModel doesn't have an Init that returns commands
	}

	return tea.Batch(cmds...)
}

func (m *navModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward adjusted size to all models (subtract 2 lines for tab bar)
		childMsg := tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height - 2}
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
		// Refresh data when switching views
		switch msg.to {
		case viewGroups:
			if m.groupsModel != nil {
				m.groupsModel.groups = m.manager.GetAllGroups()
				m.groupsModel.cursor = 0
				m.groupsModel.message = ""
			}
		case viewManage:
			if m.manageModel != nil {
				sessions, _ := m.manager.GetSessions()
				m.manageModel.sessions = sessions
				m.manageModel.rebuildSessionsOrder()
				m.manageModel.message = fmt.Sprintf("Switched to group: %s", m.manager.GetActiveGroup())
			}
		}
		return m, nil

	case tea.KeyMsg:
		// Check for global tab navigation (1-4) if no text input is focused
		if !m.isTextInputFocused() && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			switch r {
			case '1':
				if m.activeView != viewSessionize && m.sessionizeModel != nil {
					return m, func() tea.Msg { return switchViewMsg{to: viewSessionize} }
				}
			case '2':
				if m.activeView != viewManage && m.manageModel != nil {
					return m, func() tea.Msg { return switchViewMsg{to: viewManage} }
				}
			case '3':
				if m.activeView != viewHistory && m.historyModel != nil {
					return m, func() tea.Msg { return switchViewMsg{to: viewHistory} }
				}
			case '4':
				if m.activeView != viewWindows && m.windowsModel != nil {
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

	// Render global tab bar
	tabs := []struct {
		num  string
		name string
		view navView
		ok   bool
	}{
		{"1", "Sessionize", viewSessionize, m.sessionizeModel != nil},
		{"2", "Key Manage", viewManage, m.manageModel != nil},
		{"3", "History", viewHistory, m.historyModel != nil},
		{"4", "Windows", viewWindows, m.windowsModel != nil},
	}

	var tabParts []string
	for _, tab := range tabs {
		if !tab.ok {
			continue
		}
		label := fmt.Sprintf("[%s] %s", tab.num, tab.name)
		if m.activeView == tab.view {
			tabParts = append(tabParts, core_theme.DefaultTheme.Highlight.Render(label))
		} else {
			tabParts = append(tabParts, core_theme.DefaultTheme.Muted.Render(label))
		}
	}
	b.WriteString(strings.Join(tabParts, "  "))
	b.WriteString("\n")

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
	return b.String()
}

// runNavTUI runs the unified nav TUI starting in manage view
func runNavTUI() error {
	return runNavTUIWithView(viewManage)
}

// runNavTUIWithView runs the unified nav TUI starting in the specified view
func runNavTUIWithView(startView navView) error {
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

	sessions, err := mgr.GetSessions()
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	// Load enriched projects cache for manage view
	enrichedProjects := make(map[string]*manager.SessionizeProject)
	usedCache := false
	if cache, err := manager.LoadKeyManageCache(configDir); err == nil && cache != nil && len(cache.EnrichedProjects) > 0 {
		for path, cached := range cache.EnrichedProjects {
			if _, err := os.Stat(path); err == nil {
				enrichedProjects[path] = &manager.SessionizeProject{
					WorkspaceNode: cached.WorkspaceNode,
					GitStatus:     cached.GitStatus,
					NoteCounts:    cached.NoteCounts,
					PlanStats:     cached.PlanStats,
				}
			}
		}
		usedCache = len(enrichedProjects) > 0
	}

	// Create the unified nav model
	m := &navModel{
		activeView: startView,
		manager:    mgr,
		client:     client,
		configDir:  configDir,
		cwd:        cwd,
	}

	// Initialize manage model (always available)
	if len(sessions) > 0 {
		mm := newManageModel(sessions, mgr, cwd, enrichedProjects, usedCache)
		m.manageModel = &mm
	}

	// Initialize groups model (always available)
	gm := newGroupsModel(mgr)
	m.groupsModel = &gm

	// Initialize windows model (only if in tmux)
	if client != nil {
		ctx := context.Background()
		if currentSession, err := client.GetCurrentSession(ctx); err == nil && currentSession != "" {
			wm := newWindowsModel(client, currentSession, false)
			m.windowsModel = &wm
		}
	}

	// Initialize sessionize model
	if projects, err := mgr.GetAvailableProjects(); err == nil && len(projects) > 0 {
		projectPtrs := make([]*manager.SessionizeProject, len(projects))
		for i := range projects {
			projectPtrs[i] = &projects[i]
		}
		searchPaths, _ := mgr.GetEnabledSearchPaths()
		sm := newSessionizeModel(projectPtrs, searchPaths, mgr, configDir, usedCache, "")
		m.sessionizeModel = &sm
	}

	// Initialize history model
	if history, err := mgr.GetAccessHistory(); err == nil && history != nil && len(history.Projects) > 0 {
		// Build key map from sessions
		keyMap := make(map[string]string)
		for _, s := range sessions {
			if s.Path != "" {
				keyMap[s.Path] = s.Key
			}
		}
		// Get projects for enrichment
		projectMap := make(map[string]*manager.SessionizeProject)
		if projects, err := mgr.GetAvailableProjects(); err == nil {
			for i := range projects {
				projectMap[projects[i].Path] = &projects[i]
			}
		}
		// Build history items
		var items []historyItem
		for _, access := range history.Projects {
			proj := projectMap[access.Path]
			if proj == nil {
				// Create minimal project with workspace node for unknown paths
				proj = &manager.SessionizeProject{
					WorkspaceNode: &workspace.WorkspaceNode{Path: access.Path},
				}
			}
			items = append(items, historyItem{project: proj, access: access})
		}
		if len(items) > 15 {
			items = items[:15]
		}
		hm := newHistoryModel(items, mgr, keyMap)
		m.historyModel = hm
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

// runSessionizeTUI runs the unified nav TUI starting with sessionize view
func runSessionizeTUI(projects []*manager.SessionizeProject, searchPaths []string, mgr *tmux.Manager, cwdFocusPath string, usedCache bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	// Try to create tmux client (may fail if not in tmux)
	var client *tmuxclient.Client
	if os.Getenv("TMUX") != "" {
		client, _ = tmuxclient.NewClient()
	}

	// Create the unified nav model with sessionize
	m := &navModel{
		activeView: viewSessionize,
		manager:    mgr,
		client:     client,
		configDir:  configDir,
		cwd:        cwd,
	}

	// Initialize sessionize model
	sm := newSessionizeModel(projects, searchPaths, mgr, configDir, usedCache, cwdFocusPath)
	m.sessionizeModel = &sm

	// Initialize manage model with sessions from the current group
	sessions, _ := mgr.GetSessions()
	if len(sessions) > 0 {
		enrichedProjects := make(map[string]*manager.SessionizeProject)
		mm := newManageModel(sessions, mgr, cwd, enrichedProjects, false)
		m.manageModel = &mm
	}

	// Initialize groups model
	gm := newGroupsModel(mgr)
	m.groupsModel = &gm

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	// Handle post-TUI logic (same as runNavTUIWithView)
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

// runHistoryTUI runs the unified nav TUI starting with history view
func runHistoryTUI(items []historyItem, mgr *tmux.Manager, keyMap map[string]string) error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	// Try to create tmux client (may fail if not in tmux)
	var client *tmuxclient.Client
	if os.Getenv("TMUX") != "" {
		client, _ = tmuxclient.NewClient()
	}

	// Create the unified nav model with history
	m := &navModel{
		activeView: viewHistory,
		manager:    mgr,
		client:     client,
		configDir:  configDir,
		cwd:        cwd,
	}

	// Initialize history model
	hm := newHistoryModel(items, mgr, keyMap)
	m.historyModel = hm

	// Initialize manage model
	sessions, _ := mgr.GetSessions()
	if len(sessions) > 0 {
		enrichedProjects := make(map[string]*manager.SessionizeProject)
		mm := newManageModel(sessions, mgr, cwd, enrichedProjects, false)
		m.manageModel = &mm
	}

	// Initialize groups model
	gm := newGroupsModel(mgr)
	m.groupsModel = &gm

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

// runWindowsTUI runs the unified nav TUI starting with windows view
func runWindowsTUI(client *tmuxclient.Client, sessionName string, showChildProcesses bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	mgr, err := tmux.NewManager(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	// Create the unified nav model with windows
	m := &navModel{
		activeView: viewWindows,
		manager:    mgr,
		client:     client,
		configDir:  configDir,
		cwd:        cwd,
	}

	// Initialize windows model
	wm := newWindowsModel(client, sessionName, showChildProcesses)
	m.windowsModel = &wm

	// Initialize manage model
	sessions, _ := mgr.GetSessions()
	if len(sessions) > 0 {
		enrichedProjects := make(map[string]*manager.SessionizeProject)
		mm := newManageModel(sessions, mgr, cwd, enrichedProjects, false)
		m.manageModel = &mm
	}

	// Initialize groups model
	gm := newGroupsModel(mgr)
	m.groupsModel = &gm

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

