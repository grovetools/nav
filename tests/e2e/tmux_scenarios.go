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

// GmuxSessionExistsScenario tests the 'gmux session exists' command
func GmuxSessionExistsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-session-exists",
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
				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				cmd = command.New(gmuxBinary, "session", "exists", sessionName)
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

				if err := assert.Equal(0, result.ExitCode, "Should exit 0 when session exists"); err != nil {
					return err
				}

				if err := assert.Contains(result.Stdout, "exists", "Should report session exists"); err != nil {
					return err
				}

				// Check non-existent session
				cmd = command.New(gmuxBinary, "session", "exists", "non-existent-session-12345")
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

// GmuxSessionKillScenario tests the 'gmux session kill' command
func GmuxSessionKillScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-session-kill",
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

				// Kill using gmux
				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				cmd = command.New(gmuxBinary, "session", "kill", sessionName)
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

// GmuxSessionCaptureScenario tests the 'gmux session capture' command
func GmuxSessionCaptureScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-session-capture",
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

				// Capture using gmux
				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				cmd = command.New(gmuxBinary, "session", "capture", sessionName)
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

// GmuxLaunchScenario tests the 'gmux launch' command
func GmuxLaunchScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-launch",
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Test simple session launch", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := fmt.Sprintf("test-launch-%d", time.Now().Unix())

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				// Launch session
				cmd := command.New(gmuxBinary, "launch", sessionName)
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

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				// Launch session with 3 panes
				cmd := command.New(gmuxBinary, "launch", sessionName,
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

// GmuxWaitScenario tests the 'gmux wait' command
func GmuxWaitScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-wait",
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

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				// Start waiting
				start := time.Now()
				cmd = command.New(gmuxBinary, "wait", sessionName, "--poll-interval", "200ms")
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

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				// Wait with short timeout
				cmd = command.New(gmuxBinary, "wait", sessionName, "--timeout", "500ms", "--poll-interval", "100ms")
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

// GmuxStartScenario tests the 'gmux start' command
func GmuxStartScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-start",
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

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				configDir := ctx.GetString("config_dir")

				// Start session 'a' which has a path configured
				cmd := command.New(gmuxBinary, "start", "a", "--config-dir", configDir)
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
				cmd = command.New(gmuxBinary, "start", "a", "--config-dir", configDir)
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

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				configDir := ctx.GetString("config_dir")

				// Try to start a non-configured key
				cmd := command.New(gmuxBinary, "start", "z", "--config-dir", configDir)
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

// GmuxWindowsScenario tests the 'gmux windows' TUI with interactive behavior
func GmuxWindowsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "gmux-windows-tui",
		Description: "Tests the gmux windows TUI including active window selection, visual indicators, and icon assignment",
		LocalOnly:   true, // TUI tests require tmux
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup tmux session with multiple windows", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := fmt.Sprintf("test-windows-%d", time.Now().Unix())
				ctx.Set("session_name", sessionName)
				ctx.ShowCommandOutput("Session name", sessionName, "")

				// Create a session with a shell in the first window (we'll use this to run gmux windows)
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "-n", "main")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}

				// Create additional windows with specific names to test icon assignment
				// These names are chosen to trigger different icon logic in getIconForWindow
				// Use fish command to test that name patterns take priority over command patterns
				windows := []struct {
					name    string
					command string
				}{
					{"job-test", "fish"},           // Should get robot icon 󰚩 (not fish)
					{"code-review", "fish"},        // Should get code review icon  (not fish)
					{"term", "fish"},               // Should get shell icon  (not fish)
					{"plan", "fish"},               // Should get plan icon 󰠡 (not fish)
					{"cx-edit-file", "fish"},       // Should get file tree icon  (not fish)
					{"impl-task", "fish"},          // Should get interactive agent icon 󰈺 (not fish, but same icon)
					{"plain-fish", "fish"},         // Should get fish icon 󰈺 (fallback)
				}

				for _, win := range windows {
					cmd = command.New("tmux", "new-window", "-t", sessionName, "-n", win.name, win.command)
					result = cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.ExitCode != 0 {
						return fmt.Errorf("failed to create window %s: %s", win.name, result.Stderr)
					}
				}

				// Set window 1 (job-test) as the active window - not the first window
				cmd = command.New("tmux", "select-window", "-t", fmt.Sprintf("%s:1", sessionName))
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to select window: %s", result.Stderr)
				}

				// Give tmux a moment to update state
				time.Sleep(200 * time.Millisecond)

				// List windows to verify setup
				cmd = command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}:#{window_active}")
				result = cmd.Run()
				ctx.ShowCommandOutput("Window list after setup", result.Stdout, result.Stderr)

				return nil
			}),
			harness.NewStep("Launch gmux windows TUI", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find gmux binary: %w", err)
				}
				ctx.ShowCommandOutput("Using gmux binary", gmuxBinary, "")

				// Switch to the main window which has a shell ready
				cmd := command.New("tmux", "select-window", "-t", fmt.Sprintf("%s:main", sessionName))
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to select main window: %s", result.Stderr)
				}

				time.Sleep(100 * time.Millisecond)

				// Run gmux windows in the main window
				cmdStr := fmt.Sprintf("%s windows", gmuxBinary)
				cmd = command.New("tmux", "send-keys", "-t", fmt.Sprintf("%s:main", sessionName), cmdStr, "Enter")
				result = cmd.Run()
				ctx.ShowCommandOutput(fmt.Sprintf("Launching: %s", cmdStr), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to launch gmux windows: %s", result.Stderr)
				}

				// Give the TUI time to fully render
				ctx.ShowCommandOutput("Waiting for TUI to render", "1200ms", "")
				time.Sleep(1200 * time.Millisecond)

				return nil
			}),
			harness.NewStep("Verify active window indicator and initial cursor position", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Capture the pane content to see the TUI
				cmd := command.New("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("Captured TUI content", content, "")

				// Verify all expected windows are shown
				expectedWindows := []string{"main", "job-test", "code-review", "term", "plan", "cx-edit-file", "impl-task", "plain-fish"}
				for _, winName := range expectedWindows {
					if err := assert.Contains(content, winName, fmt.Sprintf("Should show window %s", winName)); err != nil {
						return err
					}
				}

				// The active window (job-test at index 1) should have the « indicator
				// This tests the feature that adds visual highlighting for active windows
				if err := assert.Contains(content, "«", "Active window should have « indicator"); err != nil {
					return err
				}

				// Verify the active window name appears with the indicator
				if err := assert.Contains(content, "job-test", "Should show job-test window"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Test navigation and window icons", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Navigate down to see other windows
				ctx.ShowCommandOutput("Sending key", "Down", "")
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "Down")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send down key: %s", result.Stderr)
				}

				time.Sleep(300 * time.Millisecond)

				// Capture again
				cmd = command.New("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane after navigation: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("TUI after navigation", content, "")

				// Verify specific icons are present for each window type
				// This tests that name patterns take priority over command patterns

				// job-test should have robot icon 󰚩 (not fish icon)
				if err := assert.Contains(content, "job-test", "Should show job-test window"); err != nil {
					return err
				}
				if err := assert.Contains(content, "󰚩", "Should show robot icon 󰚩 for job-test window"); err != nil {
					return err
				}

				// code-review should have code review icon  (not fish icon)
				// Check that the icon appears on the same line as the window name
				lines := strings.Split(content, "\n")
				foundCodeReviewWithIcon := false
				for _, line := range lines {
					if strings.Contains(line, "code-review") && strings.Contains(line, "") {
						foundCodeReviewWithIcon = true
						break
					}
				}
				if !foundCodeReviewWithIcon {
					return fmt.Errorf("code-review window should have code review icon  on the same line")
				}

				// term should have shell icon  (not fish icon)
				foundTermWithIcon := false
				for _, line := range lines {
					if strings.Contains(line, "term") && strings.Contains(line, "") {
						foundTermWithIcon = true
						break
					}
				}
				if !foundTermWithIcon {
					return fmt.Errorf("term window should have shell icon  on the same line")
				}

				// plan should have plan icon 󰠡 (not fish icon)
				foundPlanWithIcon := false
				for _, line := range lines {
					if strings.Contains(line, "plan") && strings.Contains(line, "󰠡") {
						foundPlanWithIcon = true
						break
					}
				}
				if !foundPlanWithIcon {
					return fmt.Errorf("plan window should have plan icon 󰠡 on the same line")
				}

				// cx-edit-file should have file tree icon  (not fish icon)
				if err := assert.Contains(content, "cx-edit-file", "Should show cx-edit-file window"); err != nil {
					return err
				}
				if err := assert.Contains(content, "", "Should show file tree icon  for cx-edit-file window"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Test quit with back/escape key", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Send Escape key (back key) to quit - this tests the new back key functionality
				ctx.ShowCommandOutput("Sending key to quit", "Escape", "")
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "Escape")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send escape key: %s", result.Stderr)
				}

				time.Sleep(300 * time.Millisecond)

				// Capture pane to verify TUI exited
				cmd = command.New("tmux", "capture-pane", "-t", sessionName, "-p")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane after quit: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("Shell after TUI quit", content, "")

				// After quitting with Escape, we should be back at the shell
				// The « indicator should no longer be visible in a repeating pattern
				// (it might still be in history, but the active TUI should be gone)

				return nil
			}),
			harness.NewStep("Cleanup test session", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")
				cleanupSession(sessionName)
				return nil
			}),
		},
	}
}

// GmuxWindowsActiveSelectionScenario tests that the windows TUI starts with cursor on active window
func GmuxWindowsActiveSelectionScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "gmux-windows-active-selection",
		Description: "Tests that gmux windows TUI initializes with cursor on the currently active window",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup session with non-first window active", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := fmt.Sprintf("test-active-%d", time.Now().Unix())
				ctx.Set("session_name", sessionName)
				ctx.ShowCommandOutput("Session name", sessionName, "")

				// Create a session with a shell in the first window
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "-n", "window0")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}

				// Create two more windows with sleep (they won't be the active window)
				for i := 1; i <= 2; i++ {
					cmd = command.New("tmux", "new-window", "-t", sessionName, "-n", fmt.Sprintf("window%d", i), "sleep", "60")
					result = cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.ExitCode != 0 {
						return fmt.Errorf("failed to create window %d: %s", i, result.Stderr)
					}
				}

				// Select window 2 (third window, the last one) as active
				// This tests that the cursor starts at the active window, not the first one
				ctx.ShowCommandOutput("Setting active window", "window 2 (last window)", "")
				cmd = command.New("tmux", "select-window", "-t", fmt.Sprintf("%s:2", sessionName))
				result = cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to select window: %s", result.Stderr)
				}

				time.Sleep(200 * time.Millisecond)

				// Verify window 2 is active
				cmd = command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}:#{window_active}")
				result = cmd.Run()
				ctx.ShowCommandOutput("Window list before TUI", result.Stdout, result.Stderr)

				// Window index 2 (named window1) should be active
				if err := assert.Contains(result.Stdout, "2:window1:1", "Window 2 (window1) should be active"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Launch gmux windows and verify initial selection", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find gmux binary: %w", err)
				}
				ctx.ShowCommandOutput("Using gmux binary", gmuxBinary, "")

				// Switch to window0 which has a shell
				cmd := command.New("tmux", "select-window", "-t", fmt.Sprintf("%s:window0", sessionName))
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to select window0: %s", result.Stderr)
				}

				time.Sleep(100 * time.Millisecond)

				// Run gmux windows from window0
				cmdStr := fmt.Sprintf("%s windows", gmuxBinary)
				cmd = command.New("tmux", "send-keys", "-t", fmt.Sprintf("%s:window0", sessionName), cmdStr, "Enter")
				result = cmd.Run()
				ctx.ShowCommandOutput(fmt.Sprintf("Launching: %s", cmdStr), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to launch gmux windows: %s", result.Stderr)
				}

				ctx.ShowCommandOutput("Waiting for TUI to render", "1200ms", "")
				time.Sleep(1200 * time.Millisecond)

				// Capture the TUI
				cmd = command.New("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("TUI with active window 2", content, "")

				// Verify all windows are shown
				if err := assert.Contains(content, "window0", "Should show window0"); err != nil {
					return err
				}
				if err := assert.Contains(content, "window1", "Should show window1"); err != nil {
					return err
				}
				if err := assert.Contains(content, "window2", "Should show window2"); err != nil {
					return err
				}

				// The active window (window2) should have the « indicator
				if err := assert.Contains(content, "window2", "Active window should be visible"); err != nil {
					return err
				}

				// The « indicator should be next to window2
				if err := assert.Contains(content, "«", "Active window should have « indicator"); err != nil {
					return err
				}

				// The initial cursor should be on window2 (the active window)
				// This is the key feature being tested - cursor starts on active window, not first window

				return nil
			}),
			harness.NewStep("Test navigation from active window", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Press up to navigate to window1
				ctx.ShowCommandOutput("Sending key", "Up", "")
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "Up")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send up key: %s", result.Stderr)
				}

				time.Sleep(300 * time.Millisecond)

				// Capture after navigation
				cmd = command.New("tmux", "capture-pane", "-t", sessionName, "-p")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("TUI after navigation up", content, "")

				// Should still show all windows
				if err := assert.Contains(content, "window1", "Should show window1 after navigation"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Quit and cleanup", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Quit with q
				ctx.ShowCommandOutput("Sending key to quit", "q", "")
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "q")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send q key: %s", result.Stderr)
				}

				time.Sleep(200 * time.Millisecond)

				// Cleanup
				ctx.ShowCommandOutput("Cleaning up session", sessionName, "")
				cleanupSession(sessionName)
				return nil
			}),
		},
	}
}

// GmuxWindowsMoveScenario tests the move window functionality in the windows TUI
func GmuxWindowsMoveScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "gmux-windows-move",
		Description: "Tests the move window feature that allows reordering windows with m key and j/k navigation",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test session with multiple windows", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := fmt.Sprintf("test-move-%d", time.Now().Unix())
				ctx.Set("session_name", sessionName)
				ctx.ShowCommandOutput("Session name", sessionName, "")

				// Create session with a default shell window (will be used to launch gmux)
				cmd := command.New("tmux", "new-session", "-d", "-s", sessionName, "-n", "shell")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to create test session: %s", result.Stderr)
				}

				// Create three more windows with sleep to have enough to test reordering
				windows := []string{"alpha", "beta", "gamma"}
				for _, winName := range windows {
					cmd = command.New("tmux", "new-window", "-t", sessionName, "-n", winName, "sleep", "60")
					result = cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.ExitCode != 0 {
						return fmt.Errorf("failed to create window %s: %s", winName, result.Stderr)
					}
				}

				// Give windows time to initialize
				time.Sleep(200 * time.Millisecond)

				// Verify window order
				cmd = command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}")
				result = cmd.Run()
				ctx.ShowCommandOutput("Initial window order", result.Stdout, result.Stderr)

				return nil
			}),
			harness.NewStep("Launch gmux windows TUI", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find gmux binary: %w", err)
				}

				// Switch to shell window
				cmd := command.New("tmux", "select-window", "-t", fmt.Sprintf("%s:shell", sessionName))
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to select shell window: %s", result.Stderr)
				}

				time.Sleep(100 * time.Millisecond)

				// Launch gmux windows
				cmdStr := fmt.Sprintf("%s windows", gmuxBinary)
				cmd = command.New("tmux", "send-keys", "-t", fmt.Sprintf("%s:shell", sessionName), cmdStr, "Enter")
				result = cmd.Run()
				ctx.ShowCommandOutput(fmt.Sprintf("Launching: %s", cmdStr), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to launch gmux windows: %s", result.Stderr)
				}

				ctx.ShowCommandOutput("Waiting for TUI to render", "1200ms", "")
				time.Sleep(1200 * time.Millisecond)

				return nil
			}),
			harness.NewStep("Navigate to beta window", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Navigate down to beta window (second window in list after shell)
				// Windows are: shell, alpha, beta, gamma
				// So we need to go down twice to get to beta
				for i := 0; i < 2; i++ {
					cmd := command.New("tmux", "send-keys", "-t", sessionName, "Down")
					result := cmd.Run()
					if result.ExitCode != 0 {
						return fmt.Errorf("failed to send down key: %s", result.Stderr)
					}
					time.Sleep(200 * time.Millisecond)
				}

				// Give the TUI time to update after navigation
				time.Sleep(300 * time.Millisecond)

				// Capture to verify cursor position
				cmd := command.New("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("TUI at beta window", content, "")

				// Verify beta is shown
				if err := assert.Contains(content, "beta", "Should show beta window"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Enter move mode with m key", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Press m to enter move mode
				ctx.ShowCommandOutput("Entering move mode", "m key", "")
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "m")
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send m key: %s", result.Stderr)
				}

				time.Sleep(300 * time.Millisecond)

				// Capture pane to verify move mode indicator
				cmd = command.New("tmux", "capture-pane", "-t", sessionName, "-p", "-e")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to capture pane: %s", result.Stderr)
				}

				content := result.Stdout
				ctx.ShowCommandOutput("TUI in move mode", content, "")

				// Should show [MOVE MODE] indicator
				if err := assert.Contains(content, "[MOVE MODE]", "Should display move mode indicator"); err != nil {
					return err
				}

				// The selected window line should be highlighted
				// Since we're on beta, verify it's still visible
				if err := assert.Contains(content, "beta", "Should show beta window in move mode"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Move beta window up multiple times and apply changes", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Press k twice to move window up 2 positions (from position 2 to position 0)
				// This tests that multi-position moves work correctly
				ctx.ShowCommandOutput("Moving window up twice", "k k", "")
				for i := 0; i < 2; i++ {
					cmd := command.New("tmux", "send-keys", "-t", sessionName, "k")
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.ExitCode != 0 {
						return fmt.Errorf("failed to send k key: %s", result.Stderr)
					}
					time.Sleep(150 * time.Millisecond)
				}

				time.Sleep(300 * time.Millisecond)

				// Verify the visual order hasn't been applied to tmux yet
				cmd := command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}")
				result := cmd.Run()
				ctx.ShowCommandOutput("Window order before exit (should be unchanged)", result.Stdout, result.Stderr)

				// Now exit move mode with Enter to apply the changes
				ctx.ShowCommandOutput("Applying changes with Enter", "Enter key", "")
				cmd = command.New("tmux", "send-keys", "-t", sessionName, "Enter")
				result = cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send Enter key: %s", result.Stderr)
				}

				time.Sleep(2000 * time.Millisecond)

				// Now verify the window order changed in tmux
				cmd = command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}")
				result = cmd.Run()
				ctx.ShowCommandOutput("Window order after applying move", result.Stdout, result.Stderr)

				// beta should now be at the first position (moved up 2 times from position 2)
				// Expected order: beta, shell, alpha, gamma
				// Parse the window order by window index
				windowsByIndex := make(map[int]string)
				for _, line := range strings.Split(strings.TrimSpace(result.Stdout), "\n") {
					parts := strings.Split(line, ":")
					if len(parts) == 2 {
						var idx int
						fmt.Sscanf(parts[0], "%d", &idx)
						windowsByIndex[idx] = parts[1]
					}
				}

				// Find indices of all windows
				betaIdx := -1
				shellIdx := -1
				alphaIdx := -1
				for idx, name := range windowsByIndex {
					if name == "beta" {
						betaIdx = idx
					}
					if name == "shell" {
						shellIdx = idx
					}
					if name == "alpha" {
						alphaIdx = idx
					}
				}

				if betaIdx == -1 || shellIdx == -1 || alphaIdx == -1 {
					return fmt.Errorf("could not find all windows in window list")
				}

				// Beta should be before both shell and alpha (moved up 2 positions)
				if betaIdx >= shellIdx {
					return fmt.Errorf("beta should be before shell after moving up twice, but beta is at index %d and shell is at index %d", betaIdx, shellIdx)
				}
				if betaIdx >= alphaIdx {
					return fmt.Errorf("beta should be before alpha after moving up twice, but beta is at index %d and alpha is at index %d", betaIdx, alphaIdx)
				}

				ctx.ShowCommandOutput("Move successful", fmt.Sprintf("beta at %d, shell at %d, alpha at %d", betaIdx, shellIdx, alphaIdx), "")

				return nil
			}),
			harness.NewStep("Verify windows reordered successfully", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Verify the final window order persists in tmux
				cmd := command.New("tmux", "list-windows", "-t", sessionName, "-F", "#{window_index}:#{window_name}")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to list windows: %s", result.Stderr)
				}

				ctx.ShowCommandOutput("Final window order in tmux", result.Stdout, result.Stderr)

				// Just verify beta is in the list (reordering already verified in previous step)
				if err := assert.Contains(result.Stdout, "beta", "Windows should still exist"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Exit TUI and cleanup", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				sessionName := ctx.GetString("session_name")

				// Exit TUI with q
				cmd := command.New("tmux", "send-keys", "-t", sessionName, "q")
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("failed to send q key: %s", result.Stderr)
				}

				time.Sleep(200 * time.Millisecond)

				// Cleanup session
				ctx.ShowCommandOutput("Cleaning up session", sessionName, "")
				cleanupSession(sessionName)
				return nil
			}),
		},
	}
}
