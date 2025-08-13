package tmux

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/mattsolo1/grove-core/command"
)

type Client struct {
	builder *command.SafeBuilder
}

func NewClient() (*Client, error) {
	if _, err := exec.LookPath("tmux"); err != nil {
		return nil, fmt.Errorf("tmux command not found in PATH: %w", err)
	}

	builder := command.NewSafeBuilder()
	return &Client{
		builder: builder,
	}, nil
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd, err := c.builder.Build(ctx, "tmux", args...)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("tmux command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}