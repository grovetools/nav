package main

import (
	"context"
	"fmt"
	"path/filepath"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/mux"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/theme"
	"github.com/spf13/cobra"

	"github.com/grovetools/nav/pkg/tmux"
)

var ulogStart = grovelogging.NewUnifiedLogger("nav.start")

var startCmd = &cobra.Command{
	Use:   "start <key>",
	Short: "Start a pre-configured tmux session",
	Long: `Start a tmux session using configuration from tmux-sessions.yaml.

The session will be created with the name 'grove-<key>' and will automatically
change to the configured directory for that session.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to load sessions: %w", err)
		}

		// Find the session config
		var session *models.TmuxSession
		for _, s := range sessions {
			if s.Key == key {
				session = &s
				break
			}
		}

		if session == nil {
			return fmt.Errorf("no session configured for key '%s'", key)
		}

		ctx := context.Background()
		sessionName := fmt.Sprintf("grove-%s", key)

		// Determine working directory
		workDir := session.Path
		if workDir == "" && session.Repository != "" {
			workDir = filepath.Join("~", session.Repository)
		}

		// Route through mux engine when available, fall back to tmux client.
		engine, engineErr := mux.DetectMuxEngine(ctx)
		if engineErr == nil {
			exists, err := engine.SessionExists(ctx, sessionName)
			if err != nil {
				return fmt.Errorf("failed to check session existence: %w", err)
			}
			if exists {
				ulogStart.Info("Session already exists").
					Field("session", sessionName).
					Field("key", key).
					Pretty(fmt.Sprintf("%s Session '%s' already exists.",
						theme.IconInfo, sessionName)).
					PrettyOnly().
					Emit()
				return nil
			}
			if err := engine.CreateSession(ctx, sessionName, mux.WithWorkDir(workDir)); err != nil {
				return fmt.Errorf("failed to launch session: %w", err)
			}
		} else {
			client, err := tmuxclient.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create tmux client: %w", err)
			}
			exists, err := client.SessionExists(ctx, sessionName)
			if err != nil {
				return fmt.Errorf("failed to check session existence: %w", err)
			}
			if exists {
				ulogStart.Info("Session already exists").
					Field("session", sessionName).
					Field("key", key).
					Pretty(fmt.Sprintf("%s Session '%s' already exists.",
						theme.IconInfo, sessionName)).
					PrettyOnly().
					Emit()
				return nil
			}
			opts := tmuxclient.LaunchOptions{
				SessionName:      sessionName,
				WorkingDirectory: workDir,
				WindowName:       session.Repository,
			}
			if err := client.Launch(ctx, opts); err != nil {
				return fmt.Errorf("failed to launch session: %w", err)
			}
		}

		prettyMsg := fmt.Sprintf("%s Session '%s' started for %s", theme.IconSuccess, sessionName, session.Description)
		if workDir != "" {
			prettyMsg += fmt.Sprintf("\nWorking directory: %s", workDir)
		}

		ulogStart.Success("Session started").
			Field("session", sessionName).
			Field("key", key).
			Field("working_dir", workDir).
			Field("description", session.Description).
			Pretty(prettyMsg).
			PrettyOnly().
			Emit()

		return nil
	},
}

func init() {
	// The start command uses the global config flags from rootCmd
}
