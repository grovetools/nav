package main

import (
	"context"

	"github.com/grovetools/core/pkg/mux"
	"github.com/spf13/cobra"

	"github.com/grovetools/nav/pkg/tmux"
)

var recordSessionCmd = &cobra.Command{
	Use:   "record-session",
	Short: "Record the current tmux session to access history",
	Long:  `Records the current tmux session's working directory to the access history. Designed to be called from a tmux hook (client-session-changed) to track session switches.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Must be in tmux
		if mux.ActiveMux() == mux.MuxNone {
			return nil
		}

		ctx := context.Background()

		engine, err := mux.DetectMuxEngine(ctx)
		if err != nil {
			return nil
		}

		currentSession, err := engine.GetCurrentSession(ctx)
		if err != nil || currentSession == "" {
			return nil
		}

		sessionPath, err := engine.GetSessionPath(ctx, currentSession)
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
