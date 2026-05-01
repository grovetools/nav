package main

import (
	"context"
	"fmt"
	"strings"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/mux"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/theme"
	"github.com/spf13/cobra"
)

var ulogLaunch = grovelogging.NewUnifiedLogger("nav.launch")

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
  nav launch dev-session

  # Session with window name and working directory
  nav launch dev-session --window-name coding --working-dir /path/to/project

  # Session with multiple panes
  nav launch dev-session --pane "vim main.go" --pane "go test -v" --pane "htop"

  # Complex panes with working directories (format: command[@workdir])
  nav launch dev-session --pane "npm run dev@/app/frontend" --pane "go run .@/app/backend"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]
		ctx := context.Background()

		// Try mux engine first, fall back to direct tmux client.
		if engine, err := mux.DetectMuxEngine(ctx); err == nil {
			var opts []mux.SessionOption
			if launchWorkingDir != "" {
				opts = append(opts, mux.WithWorkDir(launchWorkingDir))
			}
			if err := engine.CreateSession(ctx, sessionName, opts...); err != nil {
				return fmt.Errorf("failed to launch session: %w", err)
			}
		} else {
			client, err := tmuxclient.NewClient()
			if err != nil {
				return fmt.Errorf("failed to create tmux client: %w", err)
			}

			var paneOpts []tmuxclient.PaneOptions
			for _, paneStr := range launchPanes {
				pane := tmuxclient.PaneOptions{}
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
			if err := client.Launch(ctx, opts); err != nil {
				return fmt.Errorf("failed to launch session: %w", err)
			}
		}

		ulogLaunch.Success("Session launched").
			Field("session", sessionName).
			Field("window_name", launchWindowName).
			Field("working_dir", launchWorkingDir).
			Pretty(fmt.Sprintf("%s Session '%s' launched successfully",
				theme.IconSuccess, sessionName)).
			PrettyOnly().
			Emit()

		return nil
	},
}

func init() {
	launchCmd.Flags().StringVar(&launchWindowName, "window-name", "", "Name for the initial window")
	launchCmd.Flags().StringVar(&launchWorkingDir, "working-dir", "", "Working directory for the session")
	launchCmd.Flags().StringArrayVar(&launchPanes, "pane", []string{}, "Add a pane with command (can be used multiple times). Format: 'command[@workdir]'")
}
