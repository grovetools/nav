package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/spf13/cobra"
)

var groupPrefix string
var forceDelete bool

var groupCmd = &cobra.Command{
	Use:     "group",
	Aliases: []string{"g"},
	Short:   "Manage workspace groups",
}

var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all groups",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return err
		}
		for _, g := range mgr.GetAllGroups() {
			active := ""
			if g != "default" {
				if ref, ok := mgr.GetGroupConfig(g); ok {
					if ref.Active != nil && !*ref.Active {
						active = " (Inactive)"
					}
				}
			}
			fmt.Printf("- %s%s\n", g, active)
		}
		return nil
	},
}

var groupCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return err
		}
		if err := mgr.CreateGroup(args[0], groupPrefix); err != nil {
			return err
		}
		fmt.Printf("%s Created group '%s'\n", core_theme.IconSuccess, args[0])
		return nil
	},
}

var groupDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return err
		}
		if !forceDelete {
			fmt.Printf("Are you sure you want to delete group '%s'? All mappings will be lost. [y/N]: ", args[0])
			reader := bufio.NewReader(os.Stdin)
			ans, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(ans)) != "y" {
				fmt.Println("Cancelled")
				return nil
			}
		}
		if err := mgr.DeleteGroup(args[0]); err != nil {
			return err
		}
		fmt.Printf("%s Deleted group '%s'\n", core_theme.IconSuccess, args[0])
		return nil
	},
}

var groupActivateCmd = &cobra.Command{
	Use:   "activate [name]",
	Short: "Activate a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return err
		}
		if err := mgr.SetGroupActive(args[0], true); err != nil {
			return err
		}
		fmt.Printf("%s Activated group '%s'\n", core_theme.IconSuccess, args[0])
		return nil
	},
}

var groupDeactivateCmd = &cobra.Command{
	Use:   "deactivate [name]",
	Short: "Deactivate a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return err
		}
		if err := mgr.SetGroupActive(args[0], false); err != nil {
			return err
		}
		fmt.Printf("%s Deactivated group '%s'\n", core_theme.IconSuccess, args[0])
		return nil
	},
}

func init() {
	groupCreateCmd.Flags().StringVarP(&groupPrefix, "prefix", "p", "", "Prefix key (e.g. '<grove> g' → C-g g key)")
	groupDeleteCmd.Flags().BoolVar(&forceDelete, "force", false, "Force delete without confirmation")

	groupCmd.AddCommand(groupListCmd)
	groupCmd.AddCommand(groupCreateCmd)
	groupCmd.AddCommand(groupDeleteCmd)
	groupCmd.AddCommand(groupActivateCmd)
	groupCmd.AddCommand(groupDeactivateCmd)

	keyCmd.AddCommand(groupCmd)
}
