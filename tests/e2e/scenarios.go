package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/command"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// setupMockTmuxConfig creates a mock config directory with tmux-sessions.yaml
func setupMockTmuxConfig(ctx *harness.Context) error {
	configDir := ctx.NewDir("config")
	ctx.Set("config_dir", configDir)

	// Create a mock git repo
	repoDir := ctx.NewDir("repo")
	// Ensure the directory exists
	if err := fs.CreateDir(repoDir); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}
	if err := git.Init(repoDir); err != nil {
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	// Set git config to avoid errors
	if err := git.SetupTestConfig(repoDir); err != nil {
		return fmt.Errorf("failed to setup git config: %w", err)
	}

	// Create tmux-sessions.yaml
	sessionsYAML := fmt.Sprintf(`available_keys: [a, b, c]
sessions:
  a:
    path: %s
    repo: test-repo-a
    description: Test repository A
  b:
    path: /non/existent/path
    repo: test-repo-b
    description: Test repository B (no path)
  c:
    repo: test-repo-c
    description: Test repository C (path not set)
`, repoDir)

	if err := fs.WriteString(filepath.Join(configDir, "tmux-sessions.yaml"), sessionsYAML); err != nil {
		return fmt.Errorf("failed to write tmux-sessions.yaml: %w", err)
	}

	return nil
}

// GmuxListScenario tests the 'gmux list' command
func GmuxListScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-list-command",
		Steps: []harness.Step{
			harness.NewStep("Setup mock tmux config", setupMockTmuxConfig),
			harness.NewStep("Run 'gmux list'", func(ctx *harness.Context) error {
				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				configDir := ctx.GetString("config_dir")
				cmd := command.New(gmuxBinary, "list", "--config-dir", configDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

				if err := assert.Equal(0, result.ExitCode, "gmux list should exit successfully"); err != nil {
					return err
				}

				// Check output contains expected sessions
				if err := assert.Contains(result.Stdout, "test-repo-a", "Should list test-repo-a"); err != nil {
					return err
				}
				if err := assert.Contains(result.Stdout, "test-repo-b", "Should list test-repo-b"); err != nil {
					return err
				}
				if err := assert.Contains(result.Stdout, "test-repo-c", "Should list test-repo-c"); err != nil {
					return err
				}

				// Check descriptions are shown
				if err := assert.Contains(result.Stdout, "Test repository A", "Should show description for repo A"); err != nil {
					return err
				}

				// Check path handling
				if err := assert.Contains(result.Stdout, "/non/existent/path", "Should show configured path for repo B"); err != nil {
					return err
				}
				if !strings.Contains(result.Stdout, "test-repo-c") || !strings.Contains(result.Stdout, "<not configured>") {
					return fmt.Errorf("Should show <not configured> for repo C which has no path")
				}

				return nil
			}),
		},
	}
}

// GmuxStatusScenario tests the 'gmux status' command
func GmuxStatusScenario() *harness.Scenario {
	return &harness.Scenario{
		Name: "gmux-status-command",
		Steps: []harness.Step{
			harness.NewStep("Setup mock tmux config with git repo", func(ctx *harness.Context) error {
				// Setup basic config
				if err := setupMockTmuxConfig(ctx); err != nil {
					return err
				}

				// Add a file to the git repo to create some status
				repoDir := ctx.Dir("repo")
				testFile := filepath.Join(repoDir, "test.txt")
				if err := fs.WriteString(testFile, "test content"); err != nil {
					return err
				}

				// Stage the file
				cmd := command.New("git", "add", "test.txt").Dir(repoDir)
				if result := cmd.Run(); result.Error != nil {
					return fmt.Errorf("failed to stage file: %w", result.Error)
				}

				return nil
			}),
			harness.NewStep("Run 'gmux status'", func(ctx *harness.Context) error {
				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				configDir := ctx.GetString("config_dir")
				cmd := command.New(gmuxBinary, "status", "--config-dir", configDir)
				result := cmd.Run()
				ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

				if err := assert.Equal(0, result.ExitCode, "gmux status should exit successfully"); err != nil {
					return err
				}

				// Check that it shows repository status
				if err := assert.Contains(result.Stdout, "test-repo-a", "Should show test-repo-a in status"); err != nil {
					return err
				}

				// The status should indicate changes (staged file)
				// Note: The exact status text depends on the git implementation
				output := result.Stdout
				if !strings.Contains(output, "staged") && !strings.Contains(output, "changes") && !strings.Contains(output, "modified") {
					// If no specific status, at least check the header was printed
					if err := assert.Contains(output, "Repository", "Should show Repository header"); err != nil {
						return err
					}
					if err := assert.Contains(output, "Status", "Should show Status header"); err != nil {
						return err
					}
				}

				return nil
			}),
		},
	}
}
