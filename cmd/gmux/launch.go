package main

import (
	"context"
	"fmt"
	"strings"

	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/spf13/cobra"
)

var (
	launchWindowName string
	launchWorkingDir string
	launchPanes      []string
)

var launchCmd = &cobra.Command{
	Use:   "launch <session-name>",
	Short: "Launch a new tmux session with optional panes",
	Long: `Launch a new tmux session with support for multiple panes.
	
Examples:
  # Simple session
  gmux launch dev-session
  
  # Session with window name and working directory
  gmux launch dev-session --window-name coding --working-dir /path/to/project
  
  # Session with multiple panes
  gmux launch dev-session --pane "vim main.go" --pane "go test -v" --pane "htop"
  
  # Complex panes with working directories (format: command[@workdir])
  gmux launch dev-session --pane "npm run dev@/app/frontend" --pane "go run .@/app/backend"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		sessionName := args[0]

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		// Parse pane configurations
		var paneOpts []tmuxclient.PaneOptions
		for _, paneStr := range launchPanes {
			pane := tmuxclient.PaneOptions{}

			// Check for @workdir syntax
			if idx := strings.LastIndex(paneStr, "@"); idx != -1 {
				pane.Command = paneStr[:idx]
				pane.WorkingDirectory = paneStr[idx+1:]
			} else {
				pane.Command = paneStr
			}

			paneOpts = append(paneOpts, pane)
		}

		opts := tmuxclient.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: launchWorkingDir,
			WindowName:       launchWindowName,
			Panes:            paneOpts,
		}

		err = client.Launch(ctx, opts)
		if err != nil {
			return fmt.Errorf("failed to launch session: %w", err)
		}

		fmt.Printf("Session '%s' launched successfully\n", sessionName)

		// Show how to attach
		fmt.Printf("\nTo attach to this session, run:\n")
		fmt.Printf("  tmux attach-session -t %s\n", sessionName)

		return nil
	},
}

func init() {
	launchCmd.Flags().StringVar(&launchWindowName, "window-name", "", "Name for the initial window")
	launchCmd.Flags().StringVar(&launchWorkingDir, "working-dir", "", "Working directory for the session")
	launchCmd.Flags().StringArrayVar(&launchPanes, "pane", []string{}, "Add a pane with command (can be used multiple times). Format: 'command[@workdir]'")
}
