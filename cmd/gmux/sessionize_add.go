package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var sessionizeAddCmd = &cobra.Command{
	Use:   "add [path]",
	Short: "[DEPRECATED] Add an explicit project to sessionizer",
	Long:  `This command is deprecated. Project discovery is now managed via the global grove.yml 'groves' configuration.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("❌ This command is deprecated.")
		fmt.Println("")
		fmt.Println("Project discovery is now managed centrally via grove-core's DiscoveryService.")
		fmt.Println("To add search paths, edit your global ~/.config/grove/grove.yml file:")
		fmt.Println("")
		fmt.Println("  groves:")
		fmt.Println("    work:")
		fmt.Println("      path: ~/work")
		fmt.Println("      enabled: true")
		fmt.Println("")
		fmt.Println("See https://docs.grove.dev for more information.")
		return fmt.Errorf("command deprecated")
	},
}

var sessionizeRemoveCmd = &cobra.Command{
	Use:   "remove [path]",
	Short: "[DEPRECATED] Remove an explicit project from sessionizer",
	Long:  `This command is deprecated. Project discovery is now managed via the global grove.yml 'groves' configuration.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("❌ This command is deprecated.")
		fmt.Println("")
		fmt.Println("Project discovery is now managed centrally via grove-core's DiscoveryService.")
		fmt.Println("To manage search paths, edit your global ~/.config/grove/grove.yml file:")
		fmt.Println("")
		fmt.Println("  groves:")
		fmt.Println("    work:")
		fmt.Println("      path: ~/work")
		fmt.Println("      enabled: true")
		fmt.Println("")
		fmt.Println("See https://docs.grove.dev for more information.")
		return fmt.Errorf("command deprecated")
	},
}

func init() {
	sessionizeCmd.AddCommand(sessionizeAddCmd)
	sessionizeCmd.AddCommand(sessionizeRemoveCmd)
}
