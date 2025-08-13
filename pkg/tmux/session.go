package tmux

import (
	"context"
	"strings"
)

func (c *Client) SessionExists(ctx context.Context, sessionName string) (bool, error) {
	_, err := c.run(ctx, "has-session", "-t", sessionName)
	if err == nil {
		return true, nil
	}

	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}

	return false, err
}

func (c *Client) KillSession(ctx context.Context, sessionName string) error {
	_, err := c.run(ctx, "kill-session", "-t", sessionName)
	return err
}

func (c *Client) CapturePane(ctx context.Context, target string) (string, error) {
	output, err := c.run(ctx, "capture-pane", "-p", "-t", target)
	if err != nil {
		return "", err
	}
	return output, nil
}