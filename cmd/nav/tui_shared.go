package main

// This file hosts helpers shared by the cmd/nav TUI wiring. Most of
// the original message types and enrichment commands that used to live
// here were consumed only by the old in-file manageModel; they now live
// inside nav/pkg/tui/keymanage. Only the helpers still invoked by
// cmd/nav/nav_tui.go remain here.

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
)

// keyMapUpdateMsg carries the refreshed session->key mapping back into
// the sessionizer view after mutations in the key-manage view.
type keyMapUpdateMsg struct {
	keyMap   map[string]string
	sessions []models.TmuxSession
}

// fetchKeyMapCmd returns a tea.Cmd that re-reads the session list and
// builds a path -> key map, used by nav_tui.go to refresh sessionize
// after returning from the key-manage view.
func fetchKeyMapCmd(mgr *tmux.Manager) tea.Cmd {
	return func() tea.Msg {
		keyMap := make(map[string]string)
		sessions, err := mgr.GetSessions()
		if err != nil {
			sessions = []models.TmuxSession{}
		}
		for _, s := range sessions {
			if s.Path != "" {
				expandedPath := expandPath(s.Path)
				absPath, err := filepath.Abs(expandedPath)
				if err == nil {
					cleanPath := filepath.Clean(absPath)
					keyMap[cleanPath] = s.Key
				}
			}
		}
		return keyMapUpdateMsg{keyMap: keyMap, sessions: sessions}
	}
}

// buildProjectLoader returns a sessionizer.ProjectLoader that talks to
// the nav *tmux.Manager: it fetches projects, sorts by access history,
// applies the cloned-repo virtual ecosystem grouping, saves the project
// cache, and returns the pointer slice the sessionizer TUI expects.
func buildProjectLoader(mgr *tmux.Manager, configDir string) sessionizer.ProjectLoader {
	return func() ([]*api.Project, error) {
		projects, err := mgr.GetAvailableProjects()
		if err != nil {
			return nil, err
		}

		if history, histErr := mgr.GetAccessHistory(); histErr == nil {
			projects = manager.SortProjectsByAccess(history, projects)
		}

		projects = groupClonedProjectsAsEcosystem(projects)

		_ = api.SaveProjectCache(configDir, projects)

		ptrs := make([]*api.Project, len(projects))
		for i := range projects {
			ptrs[i] = &projects[i]
		}
		return ptrs, nil
	}
}

// jumpToMappingMsg is a cross-TUI message still handled in nav_tui.go
// for the sessionize -> manage jump. Kept here rather than in the
// sessionizer package because it is a cmd/nav-internal routing type.
type jumpToMappingMsg struct {
	path string
}

// jumpToSessionizeMsg is a cross-TUI message emitted by cmd/nav helpers
// (e.g. history jump) and routed by nav_tui.go.
type jumpToSessionizeMsg struct {
	path             string
	applyGroupFilter bool
}

// focusCwdEcosystemMsg is a cross-TUI message that asks the sessionize
// view to focus on the CWD's ecosystem.
type focusCwdEcosystemMsg struct{}
