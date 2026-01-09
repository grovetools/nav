package main

import (
	"context"
	"fmt"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/spf13/cobra"
)

var ulogSessionizeAdd = grovelogging.NewUnifiedLogger("gmux.sessionize.add")

var sessionizeAddCmd = &cobra.Command{
	Use:   "add [path]",
	Short: "[DEPRECATED] Add an explicit project to sessionizer",
	Long:  `This command is deprecated. Project discovery is now managed via the global grove.yml 'groves' configuration.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ulogSessionizeAdd.Warn("Deprecated command used").
			Field("command", "add").
			Pretty(theme.IconError + " This command is deprecated.\n\nProject discovery is now managed centrally via grove-core's DiscoveryService.\nTo add search paths, edit your global ~/.config/grove/grove.yml file:\n\n  groves:\n    work:\n      path: ~/work\n      enabled: true\n\nSee https://docs.grove.dev for more information.").
			PrettyOnly().
			Log(ctx)
		return fmt.Errorf("command deprecated")
	},
}

var sessionizeRemoveCmd = &cobra.Command{
	Use:   "remove [path]",
	Short: "[DEPRECATED] Remove an explicit project from sessionizer",
	Long:  `This command is deprecated. Project discovery is now managed via the global grove.yml 'groves' configuration.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		ulogSessionizeAdd.Warn("Deprecated command used").
			Field("command", "remove").
			Pretty(theme.IconError + " This command is deprecated.\n\nProject discovery is now managed centrally via grove-core's DiscoveryService.\nTo manage search paths, edit your global ~/.config/grove/grove.yml file:\n\n  groves:\n    work:\n      path: ~/work\n      enabled: true\n\nSee https://docs.grove.dev for more information.").
			PrettyOnly().
			Log(ctx)
		return fmt.Errorf("command deprecated")
	},
}

func init() {
	sessionizeCmd.AddCommand(sessionizeAddCmd)
	sessionizeCmd.AddCommand(sessionizeRemoveCmd)
}
