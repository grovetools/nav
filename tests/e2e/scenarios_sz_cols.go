package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// setupSzColsTestEnv creates a sandboxed test environment with:
// - A grove.yml config with groves pointing to test projects
// - Multiple test git repositories as projects
// - Optionally configured session key mappings
// - Optionally configured context rules
func setupSzColsTestEnv(ctx *harness.Context, setupOpts *szColsSetupOptions) error {
	if setupOpts == nil {
		setupOpts = &szColsSetupOptions{}
	}

	// Create projects directory structure
	projectsDir := filepath.Join(ctx.RootDir, "projects")
	if err := fs.CreateDir(projectsDir); err != nil {
		return fmt.Errorf("failed to create projects directory: %w", err)
	}

	// Create test git repos
	projectNames := []string{"proj-a", "proj-b", "proj-c"}
	for _, name := range projectNames {
		repoDir := filepath.Join(projectsDir, name)
		if err := fs.CreateDir(repoDir); err != nil {
			return fmt.Errorf("failed to create repo directory %s: %w", name, err)
		}
		if err := git.Init(repoDir); err != nil {
			return fmt.Errorf("failed to init git repo %s: %w", name, err)
		}
		if err := git.SetupTestConfig(repoDir); err != nil {
			return fmt.Errorf("failed to setup git config for %s: %w", name, err)
		}
		// Create a file and commit to make it a valid git repo
		testFile := filepath.Join(repoDir, "README.md")
		if err := fs.WriteString(testFile, fmt.Sprintf("# %s\n\nTest project\n", name)); err != nil {
			return fmt.Errorf("failed to write README for %s: %w", name, err)
		}
		// Stage and commit the file
		if err := git.Add(repoDir, "."); err != nil {
			return fmt.Errorf("failed to stage files for %s: %w", name, err)
		}
		if err := git.Commit(repoDir, "Initial commit"); err != nil {
			return fmt.Errorf("failed to commit for %s: %w", name, err)
		}
	}

	// Store project paths for later use
	ctx.Set("projects_dir", projectsDir)
	ctx.Set("proj_a_path", filepath.Join(projectsDir, "proj-a"))
	ctx.Set("proj_b_path", filepath.Join(projectsDir, "proj-b"))
	ctx.Set("proj_c_path", filepath.Join(projectsDir, "proj-c"))

	// Create grove.yml with groves configuration
	// grove-core loads global config from $XDG_CONFIG_HOME/grove/grove.yml
	// while nav stores session mappings in $XDG_STATE_HOME/grove/nav/sessions.yml
	groveYAML := fmt.Sprintf(`version: "1.0"
groves:
  test_projects:
    path: %s
    enabled: true
tmux:
  available_keys: [a, b, c, d, e, f]
`, projectsDir)

	// Global config goes to $XDG_CONFIG_HOME/grove/grove.yml
	// The harness sets XDG_CONFIG_HOME to ctx.ConfigDir() for spawned processes
	xdgConfigDir := ctx.ConfigDir()
	groveConfigDir := filepath.Join(xdgConfigDir, "grove")
	if err := fs.CreateDir(groveConfigDir); err != nil {
		return fmt.Errorf("failed to create grove config directory: %w", err)
	}
	if err := fs.WriteString(filepath.Join(groveConfigDir, "grove.yml"), groveYAML); err != nil {
		return fmt.Errorf("failed to write grove.yml: %w", err)
	}

	// nav session mappings go to $XDG_STATE_HOME/grove/nav/sessions.yml
	xdgStateDir := ctx.StateDir()
	navStateDir := filepath.Join(xdgStateDir, "grove", "nav")
	if err := fs.CreateDir(navStateDir); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}

	// Create sessions.yml with optional key mappings
	// Format is:
	// sessions:
	//   a:
	//     path: /path/to/project
	//     repository: project-name
	sessionsYAML := "sessions:\n"
	if len(setupOpts.KeyMappings) > 0 {
		for key, projectName := range setupOpts.KeyMappings {
			projectPath := filepath.Join(projectsDir, projectName)
			sessionsYAML += fmt.Sprintf("  %s:\n    path: %s\n    repository: %s\n    description: \"Test %s\"\n",
				key, projectPath, projectName, projectName)
		}
	} else {
		// If no mappings, write an empty sessions block with a placeholder comment
		sessionsYAML += "  # No sessions configured\n"
	}
	if err := fs.WriteString(filepath.Join(navStateDir, "sessions.yml"), sessionsYAML); err != nil {
		return fmt.Errorf("failed to write sessions.yml: %w", err)
	}

	// Create context rules if specified
	// Context rules are stored in $XDG_CONFIG_HOME/grove/cx/rules.yml
	if len(setupOpts.ContextRules) > 0 {
		cxConfigDir := filepath.Join(groveConfigDir, "cx")
		if err := fs.CreateDir(cxConfigDir); err != nil {
			return fmt.Errorf("failed to create cx config directory: %w", err)
		}

		rulesYAML := "rules:\n"
		for projectName, ruleType := range setupOpts.ContextRules {
			projectPath := filepath.Join(projectsDir, projectName)
			rulesYAML += fmt.Sprintf(`  - context: %s
    paths:
      - "%s/**"
`, ruleType, projectPath)
		}
		if err := fs.WriteString(filepath.Join(cxConfigDir, "rules.yml"), rulesYAML); err != nil {
			return fmt.Errorf("failed to write rules.yml: %w", err)
		}
	}

	return nil
}

// szColsSetupOptions configures the test environment
type szColsSetupOptions struct {
	// KeyMappings maps session keys to project names (e.g., "a" -> "proj-a")
	KeyMappings map[string]string
	// ContextRules maps project names to context rule types (e.g., "proj-b" -> "hot")
	ContextRules map[string]string
}

// GmuxSzColsDefaultViewScenario tests the default table layout when no keys are mapped
// and no context rules are active.
// Validates:
// - No K column present
// - No CX column present (no context data)
// - WORKSPACE is the first column
// - Project names don't have (key) suffix
func GmuxSzColsDefaultViewScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "gmux-sz-cols-default-view",
		Description: "Tests the default table layout without keys or context rules",
		LocalOnly:   true, // TUI tests require tmux
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test environment without keys or context", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupSzColsTestEnv(ctx, &szColsSetupOptions{})
			}),
			harness.NewStep("Launch gmux sz and verify default layout", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find gmux binary: %w", err)
				}

				// Start the TUI
				session, err := ctx.StartTUI(gmuxBinary, []string{"sz"})
				if err != nil {
					return fmt.Errorf("failed to start gmux sz: %w", err)
				}

				// Wait for the TUI to render
				if err := session.WaitForText("WORKSPACE", 10*time.Second); err != nil {
					return fmt.Errorf("TUI did not render WORKSPACE header: %w", err)
				}

				// Give a bit more time for full render
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize: %w", err)
				}

				// Capture the screen content
				content, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture screen: %w", err)
				}
				ctx.ShowCommandOutput("TUI Content", content, "")

				// Verify using soft assertions
				return ctx.Verify(func(v *verify.Collector) {
					// No dedicated K column header
					v.NotContains("no K column header", content, "  K  ")

					// No CX column when no context data
					v.NotContains("no CX column header", content, "  CX  ")

					// WORKSPACE column should be present
					v.Contains("WORKSPACE column present", content, "WORKSPACE")

					// All projects should be visible
					v.Contains("proj-a is visible", content, "proj-a")
					v.Contains("proj-b is visible", content, "proj-b")
					v.Contains("proj-c is visible", content, "proj-c")

					// No key suffixes on project names
					v.NotContains("proj-a has no key suffix", content, "proj-a (")
					v.NotContains("proj-b has no key suffix", content, "proj-b (")
					v.NotContains("proj-c has no key suffix", content, "proj-c (")
				})
			}),
		},
	}
}

// GmuxSzColsKeysMappedScenario tests that session keys are correctly appended to workspace names.
// Validates:
// - No dedicated K column
// - Keys appear as suffix on workspace names e.g., "proj-a (a)"
// - Projects without keys have no suffix
func GmuxSzColsKeysMappedScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "gmux-sz-cols-keys-mapped",
		Description: "Tests that session keys are appended to workspace names",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test environment with key mappings", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupSzColsTestEnv(ctx, &szColsSetupOptions{
					KeyMappings: map[string]string{
						"a": "proj-a",
						"c": "proj-c",
					},
				})
			}),
			harness.NewStep("Launch gmux sz and verify key suffix format", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				gmuxBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find gmux binary: %w", err)
				}

				session, err := ctx.StartTUI(gmuxBinary, []string{"sz"})
				if err != nil {
					return fmt.Errorf("failed to start gmux sz: %w", err)
				}

				if err := session.WaitForText("WORKSPACE", 10*time.Second); err != nil {
					return fmt.Errorf("TUI did not render: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize: %w", err)
				}

				content, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture screen: %w", err)
				}
				ctx.ShowCommandOutput("TUI Content with Keys", content, "")

				return ctx.Verify(func(v *verify.Collector) {
					// No dedicated K column
					v.NotContains("no K column header", content, "  K  ")

					// No CX column (no context rules)
					v.NotContains("no CX column header", content, "  CX  ")

					// proj-a should have key suffix (a)
					v.Contains("proj-a has key suffix", content, "(a)")

					// proj-b should NOT have a key suffix
					v.NotContains("proj-b has no key suffix", content, "proj-b (")

					// proj-c should have key suffix (c)
					v.Contains("proj-c has key suffix", content, "(c)")
				})
			}),
		},
	}
}

// GmuxSzColsContextAppliedScenario tests that the CX column appears when context rules are active.
// NOTE: This test is currently marked as ExplicitOnly because setting up context rules in the test
// environment requires a complex integration with grove-context's rule system which reads from
// .grove/rules files in the working directory. The CX column functionality is tested manually.
// TODO: Enable this test when grove-context test utilities are available.
func GmuxSzColsContextAppliedScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:         "gmux-sz-cols-context-applied",
		Description:  "Tests that CX column appears when context rules are active (requires explicit --run)",
		LocalOnly:    true,
		ExplicitOnly: true, // Skipped in 'run all' - requires grove-context setup
		Steps: []harness.Step{
			harness.NewStep("Placeholder - requires grove-context integration", func(ctx *harness.Context) error {
				// This test requires integration with grove-context's rule files
				// which need to be placed in the project directories' .grove/rules
				return nil
			}),
		},
	}
}

// GmuxSzColsCombinedViewScenario tests the layout with both keys and context rules active.
// NOTE: This test is currently marked as ExplicitOnly because setting up context rules in the test
// environment requires a complex integration with grove-context's rule system.
// TODO: Enable this test when grove-context test utilities are available.
func GmuxSzColsCombinedViewScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:         "gmux-sz-cols-combined-view",
		Description:  "Tests the layout with both keys and context rules active (requires explicit --run)",
		LocalOnly:    true,
		ExplicitOnly: true, // Skipped in 'run all' - requires grove-context setup
		Steps: []harness.Step{
			harness.NewStep("Placeholder - requires grove-context integration", func(ctx *harness.Context) error {
				// This test requires integration with grove-context's rule files
				return nil
			}),
		},
	}
}

// GmuxSzColsFilterHidesCxScenario tests that the CX column dynamically hides
// when filtering results in no projects with context data.
// NOTE: This test is currently marked as ExplicitOnly because setting up context rules in the test
// environment requires a complex integration with grove-context's rule system.
// TODO: Enable this test when grove-context test utilities are available.
func GmuxSzColsFilterHidesCxScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:         "gmux-sz-cols-filter-hides-cx",
		Description:  "Tests that CX column hides when filtered projects have no context (requires explicit --run)",
		LocalOnly:    true,
		ExplicitOnly: true, // Skipped in 'run all' - requires grove-context setup
		Steps: []harness.Step{
			harness.NewStep("Placeholder - requires grove-context integration", func(ctx *harness.Context) error {
				// This test requires integration with grove-context's rule files
				return nil
			}),
		},
	}
}
