package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/spf13/cobra"
)

// buildHistoryLoader returns a history.HistoryLoader closure that reads
// the most recent 15 access-history entries from *tmux.Manager. Used by
// the standalone nav TUI to supply the history.Config with a live loader.
func buildHistoryLoader(mgr *tmux.Manager) history.HistoryLoader {
	return func() ([]history.Item, error) {
		accessHist, err := mgr.GetAccessHistory()
		if err != nil || accessHist == nil {
			return nil, err
		}
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
		return items, nil
	}
}

// buildHistoryKeyMap returns a path -> session-key map derived from the
// manager's current session list, used to render the Key column.
func buildHistoryKeyMap(mgr *tmux.Manager) map[string]string {
	keyMap := make(map[string]string)
	sessions, err := mgr.GetSessions()
	if err != nil {
		return keyMap
	}
	for _, s := range sessions {
		if s.Path != "" {
			keyMap[s.Path] = s.Key
		}
	}
	return keyMap
}

// historyCmd is a thin shim that launches the unified nav TUI focused on
// the history view. The TUI itself lives in nav/pkg/tui/history.
var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h"},
	Short:   "View and switch to recently accessed project sessions",
	Long:    `Shows an interactive TUI listing recently accessed project sessions, sorted from most to least recent.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNavTUIWithView(viewHistory, NavTUIOptions{})
	},
}

// historyLastCmd jumps directly to the most recently accessed project
// without showing the TUI. Doesn't depend on the extracted package.
var historyLastCmd = &cobra.Command{
	Use:     "last",
	Aliases: []string{"l"},
	Short:   "Switch to the most recently accessed project session",
	Long:    `Switches to the most recently used project session without showing the interactive UI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}

		allProjects, err := mgr.GetAvailableProjects()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}
		projectSet := make(map[string]struct{})
		for _, p := range allProjects {
			projectSet[p.Path] = struct{}{}
		}

		history, err := mgr.GetAccessHistory()
		if err != nil {
			return fmt.Errorf("failed to load access history: %w", err)
		}

		var historyAccesses []*workspace.ProjectAccess
		for _, access := range history.Projects {
			historyAccesses = append(historyAccesses, access)
		}
		sort.Slice(historyAccesses, func(i, j int) bool {
			return historyAccesses[i].LastAccessed.After(historyAccesses[j].LastAccessed)
		})

		if len(historyAccesses) == 0 {
			return fmt.Errorf("no session history found")
		}

		cwd, _ := os.Getwd()
		if cwd != "" {
			cwd = filepath.Clean(cwd)
		}

		var latestProjectPath string
		for _, access := range historyAccesses {
			cleanPath := filepath.Clean(access.Path)
			if cwd != "" && strings.EqualFold(cleanPath, cwd) {
				continue
			}
			if _, ok := projectSet[access.Path]; ok {
				latestProjectPath = access.Path
				break
			}
		}

		if latestProjectPath == "" {
			return fmt.Errorf("no valid recent sessions found")
		}

		_ = mgr.RecordProjectAccess(latestProjectPath)
		return mgr.Sessionize(latestProjectPath)
	},
}

func init() {
	historyCmd.AddCommand(historyLastCmd)
	rootCmd.AddCommand(historyCmd)
}
