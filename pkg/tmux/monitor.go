package tmux

import (
	"context"
	"time"
)

func (c *Client) WaitForSessionClose(ctx context.Context, sessionName string, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			exists, err := c.SessionExists(ctx, sessionName)
			if err != nil {
				return err
			}
			if !exists {
				return nil
			}
		}
	}
}