package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

// buildInitialEnrichmentOptions creates options for enriching project data.
// For initial load, we disable enrichment to show the UI faster.
func buildInitialEnrichmentOptions() *manager.EnrichmentOptions {
	return &manager.EnrichmentOptions{
		FetchClaudeSessions: false,
		FetchGitStatus:      false,
		FetchNoteCounts:     false,
		FetchPlanStats:      false,
	}
}

// buildEnrichmentOptions creates options for enriching project data
// This is used for periodic refreshes in the TUI
func buildEnrichmentOptions(fetchGit, fetchClaude, fetchNotes, fetchPlans bool) *manager.EnrichmentOptions {
	return &manager.EnrichmentOptions{
		FetchClaudeSessions: fetchClaude,
		FetchGitStatus:      fetchGit,
		FetchNoteCounts:     fetchNotes,
		FetchPlanStats:      fetchPlans,
		GitStatusPaths:      nil, // nil means fetch for all projects
	}
}

var sessionizeCmd = &cobra.Command{
	Use:     "sessionize",
	Aliases: []string{"sz"},
	Short:   "Quickly create or switch to tmux sessions from project directories",
	Long:    `Discover projects from configured search paths and quickly create or switch to tmux sessions. Shows Claude session status indicators when grove-hooks is installed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If a path is provided as argument, use it directly
		if len(args) > 0 {
			// Record access for direct path usage too
			mgr, err := tmux.NewManager(configDir)
			if err != nil {
				return fmt.Errorf("failed to initialize manager: %w", err)
			}
			_ = mgr.RecordProjectAccess(args[0])
			// When a path is given, we must still resolve it to a full project object
			// before passing it to the updated sessionizeProject function.
			node, err := workspace.GetProjectByPath(args[0])
			if err != nil {
				return fmt.Errorf("failed to get project info for path %s: %w", args[0], err)
			}
			project := &manager.SessionizeProject{WorkspaceNode: node}
			return sessionizeProject(project)
		}

		// Otherwise, show the interactive project picker
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			// Check if the error is related to config loading, but not a simple "not found"
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to initialize manager (config dir: %s): %w", configDir, err)
			}
			// If config is not found, we'll proceed to first run setup.
		}

		// Try to load cached projects first for instant startup
		var projects []manager.SessionizeProject
		usedCache := false
		if cache, err := manager.LoadProjectCache(configDir); err == nil && cache != nil && len(cache.Projects) > 0 {
			// Convert CachedProject back to SessionizeProject
			projects = make([]manager.SessionizeProject, len(cache.Projects))
			for i, cached := range cache.Projects {
				projects[i] = manager.SessionizeProject{
					WorkspaceNode: cached.WorkspaceNode,
					GitStatus:     cached.GitStatus,
					ClaudeSession: cached.ClaudeSession,
					NoteCounts:    cached.NoteCounts,
					PlanStats:     cached.PlanStats,
				}
			}
			usedCache = true
		}

		// If no cache or cache load failed, fetch projects normally
		if len(projects) == 0 {
			// Enrichment options are now handled by the TUI itself
			fetchedProjects, err := mgr.GetAvailableProjects()
			if err != nil {
				// Check if the error is due to missing config file or no enabled search paths
				if os.IsNotExist(err) || strings.Contains(err.Error(), "No enabled search paths found") {
					// Interactive first-run setup
					return handleFirstRunSetup(configDir, mgr)
				}
				return fmt.Errorf("failed to get available projects (config dir: %s, HOME: %s): %w", configDir, os.Getenv("HOME"), err)
			}

			// Convert to SessionizeProject
			projects = make([]manager.SessionizeProject, len(fetchedProjects))
			for i := range fetchedProjects {
				projects[i] = fetchedProjects[i]
			}

			// Sort by access history
			if history, err := mgr.GetAccessHistory(); err == nil {
				projects = history.SortProjectsByAccess(projects)
			}
		}

		if len(projects) == 0 {
			fmt.Println("No projects found in search paths!")
			fmt.Println("\nYour grove.yml file needs to have 'groves' configured for project discovery.")
			fmt.Println("Run the setup wizard to configure your project directories interactively.")
			fmt.Print("\nRun setup now? [Y/n]: ")

			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "" || response == "y" || response == "yes" {
				return handleFirstRunSetup(configDir, mgr)
			}
			return nil
		}

		// Get search paths for display
		searchPaths, err := mgr.GetEnabledSearchPaths()
		if err != nil {
			// Don't fail if we can't get search paths, just continue without them
			searchPaths = []string{}
		}

		// Convert to pointers for the model
		projectPtrs := make([]*manager.SessionizeProject, len(projects))
		for i := range projects {
			projectPtrs[i] = &projects[i]
		}

		// Create the interactive model
		m := newSessionizeModel(projectPtrs, searchPaths, mgr, configDir, usedCache)

		// If a focused project was loaded from state, update the filtered list
		if m.focusedProject != nil {
			m.updateFiltered()
		}

		// Run the interactive program
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		// Check if a project was selected
		if sm, ok := finalModel.(sessionizeModel); ok && sm.selected != nil && sm.selected.WorkspaceNode != nil && sm.selected.Path != "" {
			// Record the access before switching
			_ = mgr.RecordProjectAccess(sm.selected.Path)
			// If it's a worktree, also record access for the parent
			if sm.selected.IsWorktree() && sm.selected.ParentProjectPath != "" {
				_ = mgr.RecordProjectAccess(sm.selected.ParentProjectPath)
			}
			return sessionizeProject(sm.selected)
		}

		return nil
	},
}

// sessionizeProject creates or switches to a tmux session for the given project.
func sessionizeProject(project *manager.SessionizeProject) error {
	if project == nil {
		return fmt.Errorf("no project selected")
	}

	// The project object already contains all necessary information.
	// We no longer need to call workspace.GetProjectByPath.
	sessionName := project.Identifier()
	absPath := project.Path

	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		// Not in tmux, create new session
		cmd := exec.Command("tmux", "new-session", "-s", sessionName, "-c", absPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// We're in tmux, use the tmux client
	client, err := tmuxclient.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create tmux client: %w", err)
	}

	ctx := context.Background()

	// Check if session exists
	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}

	if !exists {
		// Create new session
		opts := tmuxclient.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: absPath,
		}
		if err := client.Launch(ctx, opts); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Switch to the session
	if err := client.SwitchClient(ctx, sessionName); err != nil {
		return fmt.Errorf("failed to switch to session: %w", err)
	}

	// Close popup if running in one
	cmd := client.ClosePopupCmd()
	cmd.Run() // Ignore errors

	return nil
}
func handleFirstRunSetup(configDir string, mgr *tmux.Manager) error {
	// Welcome message
	fmt.Println("Welcome to gmux sessionizer!")
	fmt.Println("It looks like this is your first time running, or your configuration is missing.")
	fmt.Println("Let's set up your project directories in your main grove.yml file.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Collect project directories from the user
	var searchPaths []struct {
		key         string
		path        string
		description string
	}

	fmt.Println("Enter your project directories (press Enter with empty input when done):")
	fmt.Println("Example: ~/Projects, ~/Work, ~/Code")
	fmt.Println()

	for i := 1; ; i++ {
		fmt.Printf("Project directory %d (or press Enter to finish): ", i)

		pathInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		pathInput = strings.TrimSpace(pathInput)
		if pathInput == "" {
			break
		}

		// Expand the path to check if it exists
		expandedPath := expandPath(pathInput)
		if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
			fmt.Printf("⚠️  Warning: Directory %s doesn't exist. Create it? [Y/n]: ", pathInput)
			createResponse, _ := reader.ReadString('\n')
			createResponse = strings.TrimSpace(strings.ToLower(createResponse))

			if createResponse == "" || createResponse == "y" || createResponse == "yes" {
				if err := os.MkdirAll(expandedPath, 0755); err != nil {
					fmt.Printf("❌ Failed to create directory: %v\n", err)
					fmt.Println("Skipping this directory...")
					continue
				}
				fmt.Println("✅ Directory created!")
			} else {
				fmt.Println("Skipping non-existent directory...")
				continue
			}
		}

		// Ask for a description
		fmt.Printf("Description for %s (optional): ", pathInput)
		descInput, _ := reader.ReadString('\n')
		descInput = strings.TrimSpace(descInput)

		if descInput == "" {
			descInput = fmt.Sprintf("Projects in %s", filepath.Base(pathInput))
		}

		// Generate a key from the path
		key := strings.ToLower(filepath.Base(pathInput))
		key = strings.ReplaceAll(key, " ", "_")
		key = strings.ReplaceAll(key, "-", "_")

		// Ensure unique keys
		for _, sp := range searchPaths {
			if sp.key == key {
				key = fmt.Sprintf("%s_%d", key, i)
				break
			}
		}

		searchPaths = append(searchPaths, struct {
			key         string
			path        string
			description string
		}{
			key:         key,
			path:        pathInput,
			description: descInput,
		})

		fmt.Printf("✅ Added %s\n\n", pathInput)
	}

	// Check if user added any paths
	if len(searchPaths) == 0 {
		fmt.Println("\nNo directories added. To set up manually, edit your grove.yml file:")
		fmt.Println("  ~/.config/grove/grove.yml")
		fmt.Println("\nAnd add a 'tmux' section like this:")
		fmt.Println(getDefaultTmuxConfigContent())
		return nil
	}

	// Generate the tmux config object
	tmuxCfg := generateTmuxConfigWithPaths(searchPaths)

	// Use the manager to save the configuration.
	// We need to re-initialize the manager since the config file might not exist yet.
	mgr, err := tmux.NewManager(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to re-initialize manager: %w", err)
	}

	// This part is a bit tricky, as the manager expects to load a config.
	// We will manually construct what's needed for saving.
	// This highlights a potential improvement area for the manager API.

	// Create a temporary manager to get access to the internal config struct
	// and save method.
	tempMgr, err := manager.NewManager(configDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Manually set the tmux config on the manager
	tempMgr.SetTmuxConfig(&tmuxCfg)

	if err := tempMgr.Save(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	configPath := tempMgr.GetConfigPath()
	fmt.Printf("\n✅ Configuration saved to: %s\n", configPath)
	fmt.Printf("✅ Added %d project director%s\n", len(searchPaths),
		map[bool]string{true: "ies", false: "y"}[len(searchPaths) != 1])

	fmt.Println("\n✅ Setup complete! Run 'gmux sz' to start using the sessionizer.")
	return nil
}

// generateTmuxConfigWithPaths creates a TmuxConfig object (simplified after DiscoveryService migration)
func generateTmuxConfigWithPaths(searchPaths []struct{ key, path, description string }) manager.TmuxConfig {
	return manager.TmuxConfig{
		AvailableKeys: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"},
	}
}

// generateConfigWithPaths creates a configuration file with the user's specified paths (deprecated - kept for compatibility)
func generateConfigWithPaths(searchPaths []struct{ key, path, description string }) string {
	var content strings.Builder
	
	content.WriteString(`# project-search-paths.yaml
# Configuration file for gmux sessionizer
#
# This file defines where to search for projects.
# The sessionizer will scan these directories to find projects
# you can quickly switch between.

# Search paths: your project directories
search_paths:
`)
	
	for _, sp := range searchPaths {
		content.WriteString(fmt.Sprintf("  %s:\n", sp.key))
		content.WriteString(fmt.Sprintf("    path: %s\n", sp.path))
		content.WriteString(fmt.Sprintf("    description: \"%s\"\n", sp.description))
		content.WriteString("    enabled: true\n\n")
	}
	
	content.WriteString(`# Discovery settings control how projects are found
discovery:
  # Maximum depth to search within each path (1 = only immediate subdirectories)
  max_depth: 2
  
  # Minimum depth (0 = include the search path itself as a project)
  min_depth: 0
  
  # Patterns to exclude from search
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

# Explicit projects: specific directories to always include
explicit_projects: []
  # Example:
  # - path: ~/special-project
  #   name: "Special Project"
  #   description: "My special project outside the search paths"
  #   enabled: true

# Tips:
# 1. The sessionizer automatically discovers Git worktrees in .grove-worktrees
# 2. Projects are sorted by recent access
# 3. You can edit this file anytime to add or remove directories
# 4. Set enabled: false to temporarily disable a search path
`)
	
	return content.String()
}

// getDefaultTmuxConfigContent returns a well-commented default configuration for the tmux section
func getDefaultTmuxConfigContent() string {
	return `tmux:
  available_keys: [a, b, c, d, e, f, g, h, i, j, k, l, m, n, o, p, q, r, s, t, u, v, w, x, y, z]

  # Search paths: directories where the sessionizer looks for projects
  search_paths:
    work:
      path: ~/Work
      description: "Work projects"
      enabled: true
    personal:
      path: ~/Projects
      description: "Personal projects"
      enabled: true

  # Discovery settings control how projects are found
  discovery:
    max_depth: 2
    min_depth: 0
    exclude_patterns:
      - node_modules
      - .cache
      - target
      - build
      - dist

  # Explicit projects: specific directories to always include
  explicit_projects: []
`
}

// getDefaultConfigContent returns a well-commented default configuration
func getDefaultConfigContent() string {
	return `# project-search-paths.yaml
# Configuration file for gmux sessionizer
#
# This file defines where to search for projects and how to discover them.
# The sessionizer will scan these directories and their subdirectories
# to find projects you can quickly switch between.

# Search paths: directories where the sessionizer looks for projects
search_paths:
  # Example: Work projects
  work:
    path: ~/Work
    description: "Work projects"
    enabled: true
    
  # Example: Personal projects  
  personal:
    path: ~/Projects
    description: "Personal projects"
    enabled: true
    
  # Example: Learning and experiments
  experiments:
    path: ~/Code
    description: "Code experiments and learning"
    enabled: false  # Set to true to enable

# Discovery settings control how projects are found
discovery:
  # Maximum depth to search within each path (1 = only immediate subdirectories)
  max_depth: 2
  
  # Minimum depth (0 = include the search path itself as a project)
  min_depth: 0
  
  # File types to look for to identify project directories (not currently used)
  file_types:
    - .git
    - package.json
    - Cargo.toml
    - go.mod
    
  # Patterns to exclude from search
  exclude_patterns:
    - node_modules
    - .cache
    - target
    - build
    - dist

# Explicit projects: specific directories to always include
explicit_projects:
  # Example of explicitly adding a project outside the search paths
  - path: ~/important-project
    name: "Important Project"  # Optional custom name
    description: "My important project that lives elsewhere"
    enabled: false  # Set to true to enable

# Tips:
# 1. Use ~ for your home directory
# 2. Each search path needs a unique key (like 'work', 'personal')
# 3. Set enabled: false to temporarily disable a search path
# 4. The sessionizer automatically discovers Git worktrees in .grove-worktrees
# 5. Projects are sorted by recent access when using gmux
`
}
