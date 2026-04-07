package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/spf13/cobra"
)

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
