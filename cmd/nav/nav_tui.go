package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/pkg/daemon"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
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
	sessionizeModel *sessionizer.Model
	manageModel     *manageModel
	historyModel    *history.Model
	windowsModel    *windows.Model
	groupsModel     *groups.Model
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
			return m.sessionizeModel.IsTextInputFocused()
		}
	case viewManage:
		if m.manageModel != nil {
			return m.manageModel.setKeyMode || m.manageModel.saveToGroupNewMode || m.manageModel.newGroupMode
		}
	case viewHistory:
		if m.historyModel != nil {
			return m.historyModel.FilterMode()
		}
	case viewWindows:
		if m.windowsModel != nil {
			return m.windowsModel.Mode() == "filter" || m.windowsModel.Mode() == "rename"
		}
	case viewGroups:
		if m.groupsModel != nil {
			return m.groupsModel.InputMode() != ""
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
		// Refresh data when switching back to an already-initialized view
		switch view {
		case viewSessionize:
			if m.sessionizeModel != nil {
				// Refresh keyMap so newly mapped keys show immediately
				return fetchKeyMapCmd(m.manager)
			}
		case viewManage:
			if m.manageModel != nil {
				sessions, _ := m.manager.GetSessions()
				m.manageModel.sessions = sessions
				m.manageModel.rebuildSessionsOrder()
				m.manageModel.message = fmt.Sprintf("Switched to group: %s", m.manager.GetActiveGroup())
			}
		case viewGroups:
			if m.groupsModel != nil {
				m.groupsModel.Reset()
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

			// If no cache, run the full project loader so the cold-start path
			// gets the same sorting and virtual ecosystem grouping as cache
			// rebuilds. Falling back to a raw GetAvailableProjects fetch
			// here would skip SortProjectsByAccess and
			// groupClonedProjectsAsEcosystem.
			if len(projects) == 0 {
				loader := buildProjectLoader(m.manager, m.configDir)
				if ptrs, err := loader(); err == nil && len(ptrs) > 0 {
					projects = make([]manager.SessionizeProject, len(ptrs))
					for i, p := range ptrs {
						projects[i] = *p
					}
				}
			}

			if len(projects) > 0 {
				projectPtrs := make([]*api.Project, len(projects))
				for i := range projects {
					projectPtrs[i] = &projects[i]
				}
				searchPaths, _ := m.manager.GetEnabledSearchPaths()
				currentSession := ""
				if m.client != nil {
					if cur, err := m.client.GetCurrentSession(context.Background()); err == nil {
						currentSession = cur
					}
				}
				driver := NewTmuxDriver(m.client)
				cfg := sessionizer.Config{
					Store:                m.manager,
					SessionDriver:        driver,
					SessionStateProvider: driver,
					ConfigDir:            m.configDir,
					SearchPaths:          searchPaths,
					Features:             m.manager.GetResolvedFeatures(),
					CwdFocusPath:         m.opts.CwdFocusPath,
					UsedCache:            usedCache,
					CurrentSession:       currentSession,
					LoadProjects:         buildProjectLoader(m.manager, m.configDir),
					ReloadConfig:         reloadTmuxConfig,
					KeyMap:               sessionizeKeys,
				}
				m.sessionizeModel = sessionizer.New(cfg, projectPtrs)
			}
		}
		if m.sessionizeModel != nil {
			cmd = m.sessionizeModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.sessionizeModel.Update(childMsg)
				if sm, ok := newModel.(*sessionizer.Model); ok {
					m.sessionizeModel = sm
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
			accessHist, err := m.manager.GetAccessHistory()
			if err == nil && accessHist != nil && len(accessHist.Projects) > 0 {
				sessions, _ := m.manager.GetSessions()
				keyMap := make(map[string]string)
				for _, s := range sessions {
					if s.Path != "" {
						keyMap[s.Path] = s.Key
					}
				}

				// Build history items
				var historyAccesses []*workspace.ProjectAccess
				for _, access := range accessHist.Projects {
					historyAccesses = append(historyAccesses, access)
				}
				sort.Slice(historyAccesses, func(i, j int) bool {
					return historyAccesses[i].LastAccessed.After(historyAccesses[j].LastAccessed)
				})

				var items []history.Item
				for _, access := range historyAccesses {
					if len(items) >= 15 {
						break
					}
					node, err := workspace.GetProjectByPath(access.Path)
					if err != nil {
						node = &workspace.WorkspaceNode{Path: access.Path, Name: filepath.Base(access.Path)}
					}
					proj := &api.Project{WorkspaceNode: node}
					items = append(items, history.Item{Project: proj, Access: access})
				}
				m.historyModel = history.New(items, keyMap, historyKeys)
			}
		}
		if m.historyModel != nil {
			cmd = m.historyModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.historyModel.Update(childMsg)
				if hm, ok := newModel.(*history.Model); ok {
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
				wm := windows.New(m.client, currentSession, windowsKeys, showChildProcesses)
				m.windowsModel = &wm
			}
		}
		if m.windowsModel != nil {
			cmd = m.windowsModel.Init()
			// Forward size to the newly created model
			if m.width > 0 && m.height > 0 {
				childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
				newModel, _ := m.windowsModel.Update(childMsg)
				if wm, ok := newModel.(windows.Model); ok {
					m.windowsModel = &wm
				}
			}
		}

	case viewGroups:
		// Initialize groups model lazily
		if m.groupsModel == nil {
			m.groupsModel = groups.New(m.manager, groupsKeys, reloadTmuxConfig)
		}
		// groupsModel doesn't have an Init that returns commands
		if m.groupsModel != nil && m.width > 0 && m.height > 0 {
			childMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height - 2}
			newModel, _ := m.groupsModel.Update(childMsg)
			if gm, ok := newModel.(*groups.Model); ok {
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
			if sm, ok := newModel.(*sessionizer.Model); ok {
				m.sessionizeModel = sm
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
			if hm, ok := newModel.(*history.Model); ok {
				m.historyModel = hm
			}
		}
		if m.windowsModel != nil {
			newModel, _ := m.windowsModel.Update(childMsg)
			if wm, ok := newModel.(windows.Model); ok {
				m.windowsModel = &wm
			}
		}
		if m.groupsModel != nil {
			newModel, _ := m.groupsModel.Update(childMsg)
			if gm, ok := newModel.(*groups.Model); ok {
				m.groupsModel = gm
			}
		}
		return m, nil

	case switchViewMsg:
		// Clear pending mapping state if we are switching away from manage view
		if m.manageModel != nil && m.manageModel.pendingMapProject != nil {
			m.manageModel.pendingMapProject = nil
		}
		m.activeView = msg.to
		// Use lazy initialization - switchToView handles refresh for already-initialized views
		cmd := m.switchToView(msg.to)
		return m, cmd

	case sessionizer.RequestMapKeyMsg:
		m.activeView = viewManage
		cmd1 := m.switchToView(viewManage)
		var cmd2 tea.Cmd
		if m.manageModel != nil {
			newModel, cmd := m.manageModel.Update(msg)
			if mm, ok := newModel.(*manageModel); ok {
				m.manageModel = mm
			}
			cmd2 = cmd
		}
		return m, tea.Batch(cmd1, cmd2)

	case sessionizer.BulkMappingDoneMsg:
		m.activeView = viewManage
		cmd1 := m.switchToView(viewManage)
		if m.manageModel != nil {
			// Refresh sessions and set highlights
			m.manageModel.sessions, _ = m.manager.GetSessions()
			m.manageModel.rebuildSessionsOrder()
			m.manageModel.justMappedKeys = make(map[string]bool)
			for _, k := range msg.MappedKeys {
				m.manageModel.justMappedKeys[k] = true
			}
			m.manageModel.message = fmt.Sprintf("Mapped %d projects to keys", len(msg.MappedKeys))
		}
		return m, tea.Batch(cmd1, clearHighlightCmd())

	case sessionizer.RequestManageGroupsMsg:
		return m, func() tea.Msg { return switchViewMsg{to: viewGroups} }

	case jumpToMappingMsg:
		// Switch to manage view and jump to the specified path's mapping
		m.activeView = viewManage
		cmd1 := m.switchToView(viewManage)
		var cmd2 tea.Cmd
		if m.manageModel != nil {
			// Find which group and row contains this path
			found := m.manageModel.jumpToPath(msg.path)
			if !found {
				m.manageModel.message = "Workspace not mapped to any group"
			}
		}
		return m, tea.Batch(cmd1, cmd2)

	case jumpToSessionizeMsg:
		// Switch to sessionize view and jump to the specified path
		m.activeView = viewSessionize
		cmd1 := m.switchToView(viewSessionize)
		if m.sessionizeModel != nil {
			m.sessionizeModel.JumpToPath(msg.path, msg.applyGroupFilter)
		}
		return m, cmd1

	case history.JumpToSessionizeMsg:
		// Same as jumpToSessionizeMsg but emitted by the extracted history
		// package (which has its own exported msg type).
		m.activeView = viewSessionize
		cmd1 := m.switchToView(viewSessionize)
		if m.sessionizeModel != nil {
			m.sessionizeModel.JumpToPath(msg.Path, msg.ApplyGroupFilter)
		}
		return m, cmd1

	case focusCwdEcosystemMsg:
		// Switch to sessionize view and focus on the CWD's ecosystem
		m.activeView = viewSessionize
		cmd1 := m.switchToView(viewSessionize)
		if m.sessionizeModel != nil {
			cmd2 := m.sessionizeModel.FocusEcosystemForPath("")
			return m, tea.Batch(cmd1, cmd2)
		}
		return m, cmd1

	// Route background data messages to their owners regardless of active view
	case daemonStateUpdateMsg, daemonStreamStartedMsg, daemonStreamErrorMsg:
		// Always route daemon stream messages to the sessionize model to maintain the listening loop,
		// regardless of the currently active view tab.
		if m.sessionizeModel != nil {
			newModel, cmd := m.sessionizeModel.Update(msg)
			if sm, ok := newModel.(*sessionizer.Model); ok {
				m.sessionizeModel = sm
			}
			return m, cmd
		}
		// If sessionizeModel isn't available but stream started, ensure we keep listening
		if _, isError := msg.(daemonStreamErrorMsg); !isError {
			return m, listenToDaemonCmd()
		}
		return m, nil

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
			if sm, ok := newModel.(*sessionizer.Model); ok {
				m.sessionizeModel = sm
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
			if hm, ok := newModel.(*history.Model); ok {
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

	case windows.LoadedMsg, windows.PreviewLoadedMsg:
		if m.windowsModel != nil {
			newModel, cmd := m.windowsModel.Update(msg)
			if wm, ok := newModel.(windows.Model); ok {
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
				if sm, ok := newModel.(*sessionizer.Model); ok {
					m.sessionizeModel = sm
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
				if hm, ok := newModel.(*history.Model); ok {
					m.historyModel = hm
				}
				return m, cmd
			}
		}
		return m, nil

	case tea.KeyMsg:
		// Check for global tab navigation (1-4 or [ ]) if no text input is focused
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
			case ']', '[':
				// Define available views in order
				views := []navView{viewSessionize, viewManage, viewHistory}
				if m.client != nil {
					views = append(views, viewWindows)
				}

				// Find current index
				currIdx := 0
				for i, v := range views {
					if v == m.activeView {
						currIdx = i
						break
					}
				}

				// Calculate next view
				var nextView navView
				if r == ']' { // Next
					nextView = views[(currIdx+1)%len(views)]
				} else { // Prev
					nextView = views[(currIdx-1+len(views))%len(views)]
				}

				if m.activeView != nextView {
					return m, func() tea.Msg { return switchViewMsg{to: nextView} }
				}
			}
		}
	}

	// Route to active view
	switch m.activeView {
	case viewSessionize:
		if m.sessionizeModel != nil {
			newModel, cmd := m.sessionizeModel.Update(msg)
			if sm, ok := newModel.(*sessionizer.Model); ok {
				m.sessionizeModel = sm
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
			if hm, ok := newModel.(*history.Model); ok {
				m.historyModel = hm
			}
			return m, cmd
		}

	case viewWindows:
		if m.windowsModel != nil {
			newModel, cmd := m.windowsModel.Update(msg)
			if wm, ok := newModel.(windows.Model); ok {
				m.windowsModel = &wm
			}
			return m, cmd
		}

	case viewGroups:
		if m.groupsModel != nil {
			newModel, cmd := m.groupsModel.Update(msg)
			if gm, ok := newModel.(*groups.Model); ok {
				m.groupsModel = gm
				// Check if groups wants to switch back to manage
				if gm.NextCommand() == "km" {
					gm.ClearNextCommand()
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
		if matched := mgr.FindGroupForPath(cwd); matched != "" {
			mgr.SetActiveGroup(matched)
		} else if _, err := workspace.GetProjectByPath(cwd); err == nil {
			// Workspace but not mapped to any group - use default
			mgr.SetActiveGroup("default")
		} else if last := mgr.GetLastAccessedGroup(); last != "" {
			mgr.SetActiveGroup(last)
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

	// Clear daemon focus so it stops high-frequency scanning after TUI exit
	clientDaemon := daemon.New()
	if clientDaemon.IsRunning() {
		ctxDaemon, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = clientDaemon.SetFocus(ctxDaemon, []string{})
		cancel()
	}
	clientDaemon.Close()

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
		if nm.sessionizeModel != nil {
			if selected := nm.sessionizeModel.Selected(); selected != nil {
				if selected.WorkspaceNode != nil && selected.Path != "" {
					_ = mgr.RecordProjectAccess(selected.Path)
					if selected.IsWorktree() && selected.ParentProjectPath != "" {
						_ = mgr.RecordProjectAccess(selected.ParentProjectPath)
					}
					return sessionizeProject(selected)
				}
			}
		}

		// Handle history model exit
		if nm.historyModel != nil && nm.historyModel.Selected() != nil {
			selected := nm.historyModel.Selected()
			_ = mgr.RecordProjectAccess(selected.Path)
			return mgr.Sessionize(selected.Path)
		}

		// Handle windows model exit
		if nm.windowsModel != nil && nm.windowsModel.SelectedWindow() != nil {
			wm := nm.windowsModel
			if client != nil {
				target := fmt.Sprintf("%s:%d", wm.SessionName(), wm.SelectedWindow().Index)
				if err := client.SwitchClient(context.Background(), target); err != nil {
					// This might fail if not in a popup, which is fine
				}
				_ = client.ClosePopupCmd().Run()
			}
		}
	}

	return nil
}


