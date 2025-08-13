package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/command"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// Helper to check if tmux is available
func skipIfNoTmux(ctx *harness.Context) error {
	cmd := command.New("which", "tmux")
	result := cmd.Run()
	if result.ExitCode != 0 {
		// Return nil to skip the test gracefully without failing
		ctx.Set("skip_tmux_test", true)
		ctx.ShowCommandOutput("which tmux", "", "tmux not found - skipping tmux tests")
		return nil
	}
	return nil
}

// Helper to check if we should skip the test
func shouldSkipTmuxTest(ctx *harness.Context) bool {
	return ctx.GetBool("skip_tmux_test")
}

// Helper to cleanup tmux session
func cleanupSession(sessionName string) {
	command.New("tmux", "kill-session", "-t", sessionName).Run()
}

// GtmuxSessionExistsScenario tests the 'gtmux session exists' command
func GtmuxSessionExistsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-session-exists",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test session exists functionality", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-session-%d", time.Now().Unix())
				
				// Create a test session
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "sleep", "30")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}
				
				// Ensure cleanup happens
				defer cleanupSession(sessionName)
				
				// Check session exists
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				cmd = command.New(gtmuxBinary, "session", "exists", sessionName)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should exit 0 when session exists"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "exists", "Should report session exists"); err != nil {
					return err
				}
				
				// Check non-existent session
				cmd = command.New(gtmuxBinary, "session", "exists", "non-existent-session-12345")
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(1, result.ExitCode, "Should exit 1 when session doesn't exist"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "does not exist", "Should report session doesn't exist"); err != nil {
					return err
				}
				
				return nil
			}),
		},
	}
}

// GtmuxSessionKillScenario tests the 'gtmux session kill' command
func GtmuxSessionKillScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-session-kill",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test session kill functionality", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-kill-%d", time.Now().Unix())
				
				// Create a test session
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "sleep", "30")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}
				
				// Kill using gtmux
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				cmd = command.New(gtmuxBinary, "session", "kill", sessionName)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should successfully kill session"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "killed", "Should report session killed"); err != nil {
					return err
				}
				
				// Verify session is gone
				cmd = command.New("tmux", "has-session", "-t", sessionName)
				result = cmd.Run()
				if result.ExitCode == 0 {
					return fmt.Errorf("session still exists after kill")
				}
				
				return nil
			}),
		},
	}
}

// GtmuxSessionCaptureScenario tests the 'gtmux session capture' command
func GtmuxSessionCaptureScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-session-capture",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test session capture functionality", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-capture-%d", time.Now().Unix())
				testMessage := "Hello from tmux test!"
				
				// Create a test session with specific content
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, 
					"bash", "-c", fmt.Sprintf("echo '%s'; sleep 5", testMessage))
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}
				
				// Ensure cleanup
				defer cleanupSession(sessionName)
				
				// Wait a bit for content to appear
				time.Sleep(200 * time.Millisecond)
				
				// Capture using gtmux
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				cmd = command.New(gtmuxBinary, "session", "capture", sessionName)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should successfully capture pane"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, testMessage, "Should capture the test message"); err != nil {
					return err
				}
				
				return nil
			}),
		},
	}
}

// GtmuxLaunchScenario tests the 'gtmux launch' command
func GtmuxLaunchScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-launch",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test simple session launch", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-launch-%d", time.Now().Unix())
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				// Launch session
				cmd := command.New(gtmuxBinary, "launch", sessionName)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should successfully launch session"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "launched successfully", "Should report successful launch"); err != nil {
					return err
				}
				
				// Cleanup
				defer cleanupSession(sessionName)
				
				// Verify session exists
				cmd = command.New("tmux", "has-session", "-t", sessionName)
				result = cmd.Run()
				if err := assert.Equal(0, result.ExitCode, "Session should exist after launch"); err != nil {
					return err
				}
				
				return nil
			}),
			harness.NewStep("Test multi-pane session launch", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-multipane-%d", time.Now().Unix())
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				// Launch session with 3 panes
				cmd := command.New(gtmuxBinary, "launch", sessionName,
					"--window-name", "dev",
					"--pane", "echo 'Pane 1'",
					"--pane", "echo 'Pane 2'",
					"--pane", "echo 'Pane 3'")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should successfully launch multi-pane session"); err != nil {
					return err
				}
				
				// Cleanup
				defer cleanupSession(sessionName)
				
				// Wait a bit for panes to be created
				time.Sleep(200 * time.Millisecond)
				
				// Verify pane count
				cmd2 := command.New("tmux", "list-panes", "-t", sessionName)
				result2 := cmd2.Run()
				if err := assert.Equal(0, result2.ExitCode, "Should list panes successfully"); err != nil {
					return err
				}
				
				paneCount := len(strings.Split(strings.TrimSpace(result2.Stdout), "\n"))
				if err := assert.Equal(3, paneCount, "Should have 3 panes"); err != nil {
					return err
				}
				
				return nil
			}),
		},
	}
}

// GtmuxWaitScenario tests the 'gtmux wait' command
func GtmuxWaitScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-wait",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test wait for session close", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-wait-%d", time.Now().Unix())
				
				// Create a session that will exit after 1 second
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "sleep", "1")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				// Start waiting
				start := time.Now()
				cmd = command.New(gtmuxBinary, "wait", sessionName, "--poll-interval", "200ms")
				result = cmd.Run()
				duration := time.Since(start)
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should exit successfully after session closes"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "has closed", "Should report session closed"); err != nil {
					return err
				}
				
				// Should have waited approximately 1 second
				if duration < 900*time.Millisecond || duration > 2*time.Second {
					return fmt.Errorf("wait duration out of expected range: %v", duration)
				}
				
				return nil
			}),
			harness.NewStep("Test wait with timeout", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				sessionName := fmt.Sprintf("test-wait-timeout-%d", time.Now().Unix())
				
				// Create a long-running session
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "sleep", "30")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}
				
				// Cleanup
				defer cleanupSession(sessionName)
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				// Wait with short timeout
				cmd = command.New(gtmuxBinary, "wait", sessionName, "--timeout", "500ms", "--poll-interval", "100ms")
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				// Should fail due to timeout
				if result.ExitCode == 0 {
					return fmt.Errorf("expected timeout error, but command succeeded")
				}
				
				if err := assert.Contains(result.Stderr, "timeout", "Should report timeout error"); err != nil {
					return err
				}
				
				return nil
			}),
		},
	}
}

// GtmuxStartScenario tests the 'gtmux start' command
func GtmuxStartScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gtmux-start",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup mock tmux config", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupMockTmuxConfig(ctx)
			}),
			harness.NewStep("Test start configured session", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				configDir := ctx.GetString("config_dir")
				
				// Start session 'a' which has a path configured
				cmd := command.New(gtmuxBinary, "start", "a", "--config-dir", configDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should successfully start session"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "Session 'grove-a' started", "Should report session started"); err != nil {
					return err
				}
				
				// Cleanup
				defer cleanupSession("grove-a")
				
				// Verify session exists
				cmd = command.New("tmux", "has-session", "-t", "grove-a")
				result = cmd.Run()
				if err := assert.Equal(0, result.ExitCode, "Session grove-a should exist"); err != nil {
					return err
				}
				
				// Try to start 'a' again (should report it already exists)
				cmd = command.New(gtmuxBinary, "start", "a", "--config-dir", configDir)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				if err := assert.Equal(0, result.ExitCode, "Should succeed even if session exists"); err != nil {
					return err
				}
				
				if err := assert.Contains(result.Stdout, "already exists", "Should report session already exists"); err != nil {
					return err
				}
				
				return nil
			}),
			harness.NewStep("Test start non-configured session", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				
				gtmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}
				
				configDir := ctx.GetString("config_dir")
				
				// Try to start a non-configured key
				cmd := command.New(gtmuxBinary, "start", "z", "--config-dir", configDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				
				// Should fail
				if result.ExitCode == 0 {
					return fmt.Errorf("expected error for non-configured key, but command succeeded")
				}
				
				if err := assert.Contains(result.Stderr, "no session configured", "Should report no session configured"); err != nil {
					return err
				}
				
				return nil
			}),
		},
	}
}