package main

import (
	"context"
	"fmt"
	"os"

	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage tmux sessions",
	Long:  "Commands for managing tmux sessions including checking existence, killing sessions, and capturing pane content.",
}

var sessionExistsCmd = &cobra.Command{
	Use:   "exists <session-name>",
	Short: "Check if a tmux session exists",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		sessionName := args[0]

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		exists, err := client.SessionExists(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("failed to check session: %w", err)
		}

		if exists {
			fmt.Printf("Session '%s' exists\n", sessionName)
			return nil
		} else {
			fmt.Printf("Session '%s' does not exist\n", sessionName)
			os.Exit(1)
		}
		return nil
	},
}

var sessionKillCmd = &cobra.Command{
	Use:   "kill <session-name>",
	Short: "Kill a tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		sessionName := args[0]

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		err = client.KillSession(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("failed to kill session: %w", err)
		}

		fmt.Printf("Session '%s' killed\n", sessionName)
		return nil
	},
}

var sessionCaptureCmd = &cobra.Command{
	Use:   "capture <target>",
	Short: "Capture content from a tmux pane",
	Long:  "Capture content from a tmux pane. Target can be session-name, session-name:window.pane, etc.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		target := args[0]

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		content, err := client.CapturePane(ctx, target)
		if err != nil {
			return fmt.Errorf("failed to capture pane: %w", err)
		}

		fmt.Print(content)
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionExistsCmd)
	sessionCmd.AddCommand(sessionKillCmd)
	sessionCmd.AddCommand(sessionCaptureCmd)
}
