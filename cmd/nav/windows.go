package main

import (
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/paths"
	"github.com/spf13/cobra"

	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tui/navapp"
)

// windowsCmd is a thin shim that launches the unified nav TUI focused on
// the windows view. The TUI itself lives in nav/pkg/tui/windows.
var windowsCmd = &cobra.Command{
	Use:   "windows",
	Short: "Interactively manage windows in the current tmux session",
	Long:  `Launches a TUI to list, filter, and manage windows in the current tmux session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNavTUIWithTab(navapp.TabWindows, NavTUIOptions{})
	},
}

// loadTmuxConfig loads the user's nav/tmux configuration extension. Used by
// the windows view to read display flags like ShowChildProcesses.
func loadTmuxConfig() (*manager.TmuxConfig, error) {
	groveConfigPath, err := config.FindConfigFile(paths.ConfigDir())
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(groveConfigPath)
	if err != nil {
		return nil, err
	}

	// Try 'nav' first, fall back to 'tmux' for backwards compatibility.
	var navConfig manager.TmuxConfig
	if err := cfg.UnmarshalExtension("nav", &navConfig); err != nil {
		return nil, err
	}
	if navConfig.AvailableKeys == nil {
		if err := cfg.UnmarshalExtension("tmux", &navConfig); err != nil {
			return nil, err
		}
	}
	return &navConfig, nil
}

func init() {
	rootCmd.AddCommand(windowsCmd)
}
