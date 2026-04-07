package main

// This file hosts helpers shared by the cmd/nav TUI wiring. Most of
// the original message types and enrichment commands that used to live
// here were consumed only by the old in-file manageModel; they now live
// inside nav/pkg/tui/keymanage. Only the helpers still invoked by
// cmd/nav/nav_tui.go remain here.

import (
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
)

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
