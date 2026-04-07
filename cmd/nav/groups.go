package main

import (
	"github.com/grovetools/nav/pkg/tui/navapp"
	"github.com/spf13/cobra"
)

// groupsCmd is a thin shim that launches the unified nav TUI focused on
// the groups view. The TUI itself lives in nav/pkg/tui/groups.
var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Interactively manage workspace groups",
	Long:  `Open an interactive table to manage workspace groups. Create, rename, reorder, and delete groups.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNavTUIWithTab(navapp.TabGroups, NavTUIOptions{})
	},
}

func init() {
	rootCmd.AddCommand(groupsCmd)
}
