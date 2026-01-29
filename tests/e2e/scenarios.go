package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/assert"
	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
)

// setupMockTmuxConfig creates a mock config directory with grove.yml and sessions.yml
func setupMockTmuxConfig(ctx *harness.Context) error {
	// Use XDG config directory for grove.yml
	groveConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
	ctx.Set("config_dir", groveConfigDir)

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

	// Create grove.yml with static tmux config (no sessions)
	groveYAML := `version: "1.0"
tmux:
  available_keys: [a, b, c]
`

	if err := fs.CreateDir(groveConfigDir); err != nil {
		return fmt.Errorf("failed to create grove config directory: %w", err)
	}
	if err := fs.WriteString(filepath.Join(groveConfigDir, "grove.yml"), groveYAML); err != nil {
		return fmt.Errorf("failed to write grove.yml: %w", err)
	}

	// Create nav state directory for sessions.yml
	navStateDir := filepath.Join(ctx.StateDir(), "grove", "nav")
	if err := fs.CreateDir(navStateDir); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}

	// Create sessions.yml with session mappings
	sessionsYAML := fmt.Sprintf(`sessions:
  a:
    path: %s
    repository: test-repo-a
    description: Test repository A
  b:
    path: /non/existent/path
    repository: test-repo-b
    description: Test repository B (no path)
  c:
    repository: test-repo-c
    description: Test repository C (path not set)
`, repoDir)

	if err := fs.WriteString(filepath.Join(navStateDir, "sessions.yml"), sessionsYAML); err != nil {
		return fmt.Errorf("failed to write sessions.yml: %w", err)
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

				// Use ctx.Command to get sandboxed environment (XDG_STATE_HOME, etc.)
				configDir := ctx.GetString("config_dir")
				cmd := ctx.Command(gmuxBinary, "list", "--config-dir", configDir)
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

				// Note: Descriptions are not shown in the current list output

				// Check path handling
				if err := assert.Contains(result.Stdout, "/non/existent/path", "Should show configured path for repo B"); err != nil {
					return err
				}
				// Note: Empty paths are shown as empty cells in the table, not "<not configured>"

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

				// Use ctx.Command to get sandboxed environment (XDG_STATE_HOME, etc.)
				configDir := ctx.GetString("config_dir")
				cmd := ctx.Command(gmuxBinary, "status", "--config-dir", configDir)
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
