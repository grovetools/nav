package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/mux"
	"github.com/grovetools/core/pkg/repo"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/theme"
	"github.com/spf13/cobra"

	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/navapp"
)

var ulogSessionize = grovelogging.NewUnifiedLogger("nav.sessionize")

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

		// Fast check for first run setup - only fetch if no cache exists
		cache, _ := manager.LoadProjectCache(configDir)
		if cache == nil || len(cache.Projects) == 0 {
			projects, err := mgr.GetAvailableProjects()
			if err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "No enabled search paths found")) {
				return handleFirstRunSetup(configDir, mgr)
			}
			if len(projects) == 0 {
				ulogSessionize.Info("No projects found").
					Pretty("No projects found in search paths!\n\nYour grove.yml file needs to have 'groves' configured for project discovery.\nRun the setup wizard to configure your project directories interactively.\n\nRun setup now? [Y/n]: ").
					PrettyOnly().
					Emit()

				reader := bufio.NewReader(os.Stdin)
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))

				if response == "" || response == "y" || response == "yes" {
					return handleFirstRunSetup(configDir, mgr)
				}
				return nil
			}
		}

		// Determine initial focus based on CWD
		var cwdFocusPath string
		cwd, err := os.Getwd()
		if err == nil {
			node, err := workspace.GetProjectByPath(cwd)
			if err == nil {
				if node.Kind == workspace.KindEcosystemRoot || node.Kind == workspace.KindEcosystemWorktree {
					cwdFocusPath = node.Path
				} else if node.ParentEcosystemPath != "" {
					cwdFocusPath = node.ParentEcosystemPath
				}
			}
		}

		// Use unified nav TUI with lazy initialization
		return runNavTUIWithTab(navapp.TabSessionize, NavTUIOptions{CwdFocusPath: cwdFocusPath})
	},
}

// sessionizeProject creates or switches to a mux session for the given project.
func sessionizeProject(project *manager.SessionizeProject) error {
	if project == nil {
		return fmt.Errorf("no project selected")
	}

	sessionName := project.Identifier("_")
	absPath := project.Path

	switch mux.ActiveMux() {
	case mux.MuxTuimux:
		return sessionizeViaTuimux(sessionName, absPath)
	case mux.MuxTmux:
		return sessionizeViaTmux(sessionName, absPath)
	default:
		// Not in any mux — launch a tmux session interactively.
		cmd := tmux.Command("new-session", "-s", sessionName, "-c", absPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
}

func sessionizeViaTuimux(sessionName, absPath string) error {
	ctx := context.Background()
	engine, err := mux.DetectMuxEngine(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect mux engine: %w", err)
	}

	// SwitchSession auto-creates within the current TUI if the session doesn't exist.
	if err := engine.SwitchSession(ctx, sessionName, absPath); err != nil {
		return fmt.Errorf("failed to switch to session: %w", err)
	}

	// Close popup if running in one (best-effort).
	if te, ok := engine.(mux.MuxTUIEngine); ok {
		_ = te.ClosePopup(ctx)
	}

	return nil
}

func sessionizeViaTmux(sessionName, absPath string) error {
	ctx := context.Background()
	client, err := tmuxclient.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create tmux client: %w", err)
	}

	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}

	if !exists {
		opts := tmuxclient.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: absPath,
		}
		if err := client.Launch(ctx, opts); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	if err := client.SwitchClientToSession(ctx, sessionName); err != nil {
		return fmt.Errorf("failed to switch to session: %w", err)
	}

	_ = client.ClosePopupCmd().Run()

	return nil
}

func handleFirstRunSetup(configDir string, _ *tmux.Manager) error {
	// Welcome message
	ulogSessionize.Info("First run setup").
		Pretty("Welcome to nav sessionizer!\nIt looks like this is your first time running, or your configuration is missing.\nLet's set up your project directories in your main grove.yml file.\n").
		PrettyOnly().
		Emit()

	reader := bufio.NewReader(os.Stdin)

	// Collect project directories from the user
	var searchPaths []struct {
		key         string
		path        string
		description string
	}

	ulogSessionize.Info("Project directory prompt").
		Pretty("Enter your project directories (press Enter with empty input when done):\nExample: ~/Projects, ~/Work, ~/Code\n").
		PrettyOnly().
		Emit()

	for i := 1; ; i++ {
		ulogSessionize.Info("Directory input prompt").
			Field("directory_number", i).
			Pretty(fmt.Sprintf("Project directory %d (or press Enter to finish): ", i)).
			PrettyOnly().
			Emit()

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
			ulogSessionize.Warn("Directory does not exist").
				Field("path", pathInput).
				Pretty(fmt.Sprintf("%s  Warning: Directory %s doesn't exist. Create it? [Y/n]: ", theme.IconWarning, pathInput)).
				PrettyOnly().
				Emit()
			createResponse, _ := reader.ReadString('\n')
			createResponse = strings.TrimSpace(strings.ToLower(createResponse))

			if createResponse == "" || createResponse == "y" || createResponse == "yes" {
				if err := os.MkdirAll(expandedPath, 0o755); err != nil {
					ulogSessionize.Error("Failed to create directory").
						Field("path", pathInput).
						Err(err).
						Pretty(fmt.Sprintf("%s Failed to create directory: %v\n%s Skipping this directory...", theme.IconError, err, theme.IconInfo)).
						PrettyOnly().
						Emit()
					continue
				}
				ulogSessionize.Success("Directory created").
					Field("path", pathInput).
					Pretty(theme.IconSuccess + " Directory created!").
					PrettyOnly().
					Emit()
			} else {
				ulogSessionize.Info("Skipping directory").
					Field("path", pathInput).
					Pretty(theme.IconInfo + " Skipping non-existent directory...").
					PrettyOnly().
					Emit()
				continue
			}
		}

		// Ask for a description
		ulogSessionize.Info("Description prompt").
			Field("path", pathInput).
			Pretty(fmt.Sprintf("Description for %s (optional): ", pathInput)).
			PrettyOnly().
			Emit()
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

		ulogSessionize.Success("Added project directory").
			Field("path", pathInput).
			Field("key", key).
			Field("description", descInput).
			Pretty(fmt.Sprintf("%s Added %s\n", theme.IconSuccess, pathInput)).
			PrettyOnly().
			Emit()
	}

	// Check if user added any paths
	if len(searchPaths) == 0 {
		ulogSessionize.Info("No directories added").
			Pretty("\nNo directories added. To set up manually, edit your grove.toml file:\n  ~/.config/grove/grove.toml\n\nAnd add a 'nav' section like this:\n" + getDefaultNavConfigContent()).
			PrettyOnly().
			Emit()
		return nil
	}

	// Generate the tmux config object
	tmuxCfg := generateTmuxConfigWithPaths(searchPaths)

	// Use the manager to save the configuration.
	// We need to re-initialize the manager since the config file might not exist yet.
	_, err := tmux.NewManager(configDir)
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
	directorySuffix := map[bool]string{true: "ies", false: "y"}[len(searchPaths) != 1]
	ulogSessionize.Success("Configuration saved").
		Field("config_path", configPath).
		Field("directory_count", len(searchPaths)).
		Pretty(fmt.Sprintf("\n%s Configuration saved to: %s\n%s Added %d project director%s\n\n%s Setup complete! Run 'nav sz' to start using the sessionizer.",
			theme.IconSuccess, configPath,
			theme.IconSuccess, len(searchPaths), directorySuffix,
			theme.IconSuccess)).
		PrettyOnly().
		Emit()
	return nil
}

// generateTmuxConfigWithPaths creates a TmuxConfig object (simplified after DiscoveryService migration)
func generateTmuxConfigWithPaths(searchPaths []struct{ key, path, description string }) manager.TmuxConfig {
	return manager.TmuxConfig{
		AvailableKeys: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"},
	}
}

// getDefaultNavConfigContent returns a well-commented default configuration for the nav section
func getDefaultNavConfigContent() string {
	return `[nav]
available_keys = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t", "u", "v", "w", "x", "y", "z"]

# Search paths are now configured via [groves] section - see grove-core docs
`
}

// groupClonedProjectsAsEcosystem identifies projects cloned via `cx repo` and groups them
// under a virtual "cx-repos" ecosystem node. Projects are identified by their
// ParentEcosystemPath pointing to the cx ecosystem path (~/.local/share/grove/cx).
// This function adds a virtual ecosystem node if one doesn't already exist.
func groupClonedProjectsAsEcosystem(projects []manager.SessionizeProject) []manager.SessionizeProject {
	logger := grovelogging.NewLogger("nav-sessionize")

	// Get the cx ecosystem path to identify cloned repos
	cxEcoPath, err := repo.GetCxEcosystemPath()
	if err != nil {
		logger.Warnf("Could not get cx ecosystem path: %v", err)
		return projects
	}

	var filteredProjects []manager.SessionizeProject
	for i := range projects {
		// Skip temporary source-repo directories created by cx
		if projects[i].ParentEcosystemPath == cxEcoPath && projects[i].Name == "source-repo" {
			continue
		}
		filteredProjects = append(filteredProjects, projects[i])
	}
	projects = filteredProjects

	var clonedProjectIndices []int
	for i := range projects {
		// Cloned repos are now identified by their ParentEcosystemPath pointing to cx ecosystem
		if projects[i].ParentEcosystemPath == cxEcoPath {
			clonedProjectIndices = append(clonedProjectIndices, i)
		}
	}
	logger.Debugf("Total projects: %d, Cloned repos found: %d (cx path: %s)", len(projects), len(clonedProjectIndices), cxEcoPath)

	if len(clonedProjectIndices) == 0 {
		return projects
	}

	// Set RootEcosystemPath for all cloned repos to the cx ecosystem path
	for _, idx := range clonedProjectIndices {
		projects[idx].RootEcosystemPath = cxEcoPath
	}

	// Check if a node for the cx ecosystem path already exists.
	// If it does, we don't need to add a virtual one.
	for i := range projects {
		if projects[i].Path == cxEcoPath {
			return projects
		}
	}

	// Create the virtual ecosystem node for cx-repos.
	ecoNode := manager.SessionizeProject{
		WorkspaceNode: &workspace.WorkspaceNode{
			Name:              "cx-repos",
			Path:              cxEcoPath,
			Kind:              workspace.KindEcosystemRoot,
			RootEcosystemPath: cxEcoPath,
		},
	}

	// Prepend the ecosystem node to create a new slice.
	result := make([]manager.SessionizeProject, 0, len(projects)+1)
	result = append(result, ecoNode)
	result = append(result, projects...)

	return result
}
