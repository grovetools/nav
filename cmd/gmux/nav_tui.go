package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tmux"
)

// navView represents which view is currently active
type navView int

const (
	viewManage navView = iota
	viewGroups
)

// switchViewMsg signals a view switch
type switchViewMsg struct {
	to navView
}

// navModel is the root model that manages view switching
type navModel struct {
	activeView    navView
	manageModel   *manageModel
	groupsModel   *groupsModel
	manager       *tmux.Manager
	width, height int
}

func newNavModel(mgr *tmux.Manager, sessions []models.TmuxSession, cwd string, enrichedProjects map[string]*manager.SessionizeProject, usedCache bool) navModel {
	mm := newManageModel(sessions, mgr, cwd, enrichedProjects, usedCache)
	gm := newGroupsModel(mgr)

	return navModel{
		activeView:  viewManage,
		manageModel: &mm,
		groupsModel: &gm,
		manager:     mgr,
	}
}

func (m *navModel) Init() tea.Cmd {
	// Initialize the manage model
	return m.manageModel.Init()
}

func (m *navModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to both models
		m.manageModel.Update(msg)
		m.groupsModel.Update(msg)
		return m, nil

	case switchViewMsg:
		m.activeView = msg.to
		if msg.to == viewGroups {
			// Refresh groups list
			m.groupsModel.groups = m.manager.GetAllGroups()
			m.groupsModel.cursor = 0
			m.groupsModel.message = ""
		} else if msg.to == viewManage {
			// Refresh sessions for current group
			sessions, _ := m.manager.GetSessions()
			m.manageModel.sessions = sessions
			m.manageModel.rebuildSessionsOrder()
			m.manageModel.message = fmt.Sprintf("Switched to group: %s", m.manager.GetActiveGroup())
		}
		return m, nil
	}

	// Route to active view
	switch m.activeView {
	case viewManage:
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

	case viewGroups:
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

	return m, nil
}

func (m *navModel) View() string {
	switch m.activeView {
	case viewManage:
		return m.manageModel.View()
	case viewGroups:
		return m.groupsModel.View()
	}
	return ""
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

	if len(sessions) == 0 {
		fmt.Println("No sessions configured")
		return nil
	}

	// Load cache
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

	m := newNavModel(mgr, sessions, cwd, enrichedProjects, usedCache)
	m.activeView = startView
	p := tea.NewProgram(&m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	// Handle post-TUI logic
	if nm, ok := finalModel.(*navModel); ok {
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

	return nil
}
