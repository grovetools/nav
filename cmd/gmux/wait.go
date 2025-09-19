package main

import (
	"context"
	"fmt"
	"time"

	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/spf13/cobra"
)

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

		// Parse durations
		pollInterval, err := time.ParseDuration(waitPollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll interval: %w", err)
		}

		timeout, err := time.ParseDuration(waitTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}

		// Create context with timeout
		ctx := context.Background()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		fmt.Printf("Waiting for session '%s' to close...\n", sessionName)

		err = client.WaitForSessionClose(ctx, sessionName, pollInterval)
		if err != nil {
			if err == context.DeadlineExceeded {
				return fmt.Errorf("timeout waiting for session to close")
			}
			return fmt.Errorf("error waiting for session: %w", err)
		}

		fmt.Printf("Session '%s' has closed\n", sessionName)
		return nil
	},
}

func init() {
	waitCmd.Flags().StringVar(&waitPollInterval, "poll-interval", "1s", "How often to check if session exists")
	waitCmd.Flags().StringVar(&waitTimeout, "timeout", "0s", "Maximum time to wait (0 = no timeout)")
}
