package main

import (
	"context"
	"fmt"
	"os"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/spf13/cobra"
)

var ulogSession = grovelogging.NewUnifiedLogger("gmux.session")

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
		sessionName := args[0]
		ctx := context.Background()

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		exists, err := client.SessionExists(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("failed to check session: %w", err)
		}

		if exists {
			ulogSession.Info("Session exists").
				Field("session", sessionName).
				Pretty(fmt.Sprintf("%s Session '%s' exists", theme.IconSuccess, sessionName)).
				PrettyOnly().
				Emit()
			return nil
		} else {
			ulogSession.Info("Session does not exist").
				Field("session", sessionName).
				Pretty(fmt.Sprintf("%s Session '%s' does not exist", theme.IconError, sessionName)).
				PrettyOnly().
				Emit()
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
		sessionName := args[0]
		ctx := context.Background()

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		err = client.KillSession(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("failed to kill session: %w", err)
		}

		ulogSession.Success("Session killed").
			Field("session", sessionName).
			Pretty(fmt.Sprintf("%s Session '%s' killed", theme.IconSuccess, sessionName)).
			PrettyOnly().
			Emit()
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
