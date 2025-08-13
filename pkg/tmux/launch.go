package tmux

import (
	"context"
	"fmt"
)

func (c *Client) Launch(ctx context.Context, opts LaunchOptions) error {
	if opts.SessionName == "" {
		return fmt.Errorf("session name is required")
	}

	args := []string{"new-session", "-d", "-s", opts.SessionName}
	if opts.WorkingDirectory != "" {
		args = append(args, "-c", opts.WorkingDirectory)
	}

	_, err := c.run(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if opts.WindowName != "" {
		_, err = c.run(ctx, "rename-window", "-t", opts.SessionName, opts.WindowName)
		if err != nil {
			return fmt.Errorf("failed to rename window: %w", err)
		}
	}

	for i, pane := range opts.Panes {
		if i == 0 {
			if pane.Command != "" {
				_, err = c.run(ctx, "send-keys", "-t", opts.SessionName, pane.Command, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send command to first pane: %w", err)
				}
			}
			if pane.SendKeys != "" {
				_, err = c.run(ctx, "send-keys", "-t", opts.SessionName, pane.SendKeys, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send keys to first pane: %w", err)
				}
			}
		} else {
			splitArgs := []string{"split-window", "-t", opts.SessionName}
			if pane.WorkingDirectory != "" {
				splitArgs = append(splitArgs, "-c", pane.WorkingDirectory)
			}

			_, err = c.run(ctx, splitArgs...)
			if err != nil {
				return fmt.Errorf("failed to create pane %d: %w", i, err)
			}

			target := fmt.Sprintf("%s.%d", opts.SessionName, i)
			if pane.Command != "" {
				_, err = c.run(ctx, "send-keys", "-t", target, pane.Command, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send command to pane %d: %w", i, err)
				}
			}
			if pane.SendKeys != "" {
				_, err = c.run(ctx, "send-keys", "-t", target, pane.SendKeys, "Enter")
				if err != nil {
					return fmt.Errorf("failed to send keys to pane %d: %w", i, err)
				}
			}
		}
	}

	if len(opts.Panes) > 1 {
		_, err = c.run(ctx, "select-layout", "-t", opts.SessionName, "tiled")
		if err != nil {
			return fmt.Errorf("failed to apply layout: %w", err)
		}
	}

	return nil
}