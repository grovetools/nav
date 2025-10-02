package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <key>",
	Short: "Start a pre-configured tmux session",
	Long: `Start a tmux session using configuration from tmux-sessions.yaml.
	
The session will be created with the name 'grove-<key>' and will automatically
change to the configured directory for that session.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
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

		// Create tmux client
		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		// Prepare session name
		sessionName := fmt.Sprintf("grove-%s", key)

		// Check if session already exists
		exists, err := client.SessionExists(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("failed to check session existence: %w", err)
		}

		if exists {
			fmt.Printf("Session '%s' already exists. Attaching...\n", sessionName)
			fmt.Printf("\nTo attach manually, run:\n")
			fmt.Printf("  tmux attach-session -t %s\n", sessionName)
			return nil
		}

		// Determine working directory
		workDir := session.Path
		if workDir == "" && session.Repository != "" {
			// Try to use repository name as directory under home
			workDir = filepath.Join("~", session.Repository)
		}

		// Create launch options
		opts := tmuxclient.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: workDir,
			WindowName:       session.Repository,
		}

		// Launch the session
		err = client.Launch(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to launch session: %w", err)
		}

		fmt.Printf("Session '%s' started for %s\n", sessionName, session.Description)
		if workDir != "" {
			fmt.Printf("Working directory: %s\n", workDir)
		}

		fmt.Printf("\nTo attach to this session, run:\n")
		fmt.Printf("  tmux attach-session -t %s\n", sessionName)

		return nil
	},
}

func init() {
	// The start command uses the global config flags from rootCmd
}
