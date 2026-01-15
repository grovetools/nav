package main

import (
	"context"
	"os"

	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var recordSessionCmd = &cobra.Command{
	Use:   "record-session",
	Short: "Record the current tmux session to access history",
	Long:  `Records the current tmux session's working directory to the access history. Designed to be called from a tmux hook (client-session-changed) to track session switches.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Must be in tmux
		if os.Getenv("TMUX") == "" {
			return nil
		}

		client, err := tmuxclient.NewClient()
		if err != nil {
			return nil
		}

		ctx := context.Background()

		currentSession, err := client.GetCurrentSession(ctx)
		if err != nil || currentSession == "" {
			return nil
		}

		sessionPath, err := client.GetSessionPath(ctx, currentSession)
		if err != nil || sessionPath == "" {
			return nil
		}

		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return nil
		}

		_ = mgr.RecordProjectAccess(sessionPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(recordSessionCmd)
}
