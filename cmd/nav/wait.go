package main

import (
	"context"
	"fmt"
	"time"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/mux"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/theme"
	"github.com/spf13/cobra"
)

var ulogWait = grovelogging.NewUnifiedLogger("nav.wait")

var (
	waitPollInterval string
	waitTimeout      string
)

var waitCmd = &cobra.Command{
	Use:   "wait <session-name>",
	Short: "Wait for a tmux session to close",
	Long: `Block until the specified tmux session closes. Useful for scripting and automation.
	
The command will poll at regular intervals to check if the session still exists.
When the session closes, the command exits with status 0.
If the timeout is reached or an error occurs, it exits with non-zero status.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionName := args[0]

		pollInterval, err := time.ParseDuration(waitPollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll interval: %w", err)
		}

		timeout, err := time.ParseDuration(waitTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}

		ctx := context.Background()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		ulogWait.Progress("Waiting for session").
			Field("session", sessionName).
			Pretty(fmt.Sprintf("%s Waiting for session '%s' to close...", theme.IconRunning, sessionName)).
			PrettyOnly().
			Emit()

		// Try mux engine first for polling, fall back to tmux client.
		if engine, engineErr := mux.DetectMuxEngine(ctx); engineErr == nil {
			err = waitForSessionClose(ctx, engine, sessionName, pollInterval)
		} else {
			client, clientErr := tmuxclient.NewClient()
			if clientErr != nil {
				return fmt.Errorf("failed to create tmux client: %w", clientErr)
			}
			err = client.WaitForSessionClose(ctx, sessionName, pollInterval)
		}

		if err != nil {
			if err == context.DeadlineExceeded {
				return fmt.Errorf("timeout waiting for session to close")
			}
			return fmt.Errorf("error waiting for session: %w", err)
		}

		ulogWait.Success("Session closed").
			Field("session", sessionName).
			Pretty(fmt.Sprintf("%s Session '%s' has closed", theme.IconSuccess, sessionName)).
			PrettyOnly().
			Emit()
		return nil
	},
}

func init() {
	waitCmd.Flags().StringVar(&waitPollInterval, "poll-interval", "1s", "How often to check if session exists")
	waitCmd.Flags().StringVar(&waitTimeout, "timeout", "0s", "Maximum time to wait (0 = no timeout)")
}

func waitForSessionClose(ctx context.Context, engine mux.MuxEngine, sessionName string, pollInterval time.Duration) error {
	for {
		exists, err := engine.SessionExists(ctx, sessionName)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
