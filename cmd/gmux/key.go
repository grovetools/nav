package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	tablecomponent "github.com/mattsolo1/grove-core/tui/components/table"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var ulogKey = grovelogging.NewUnifiedLogger("gmux.key")

// (listStyle is now declared in main.go)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage tmux session key bindings",
	Long:  "Commands for managing tmux session key bindings including updating keys and editing session details.",
}

// displaySessionsTable shows sessions in a styled table and returns true if any sessions have paths
func displaySessionsTable(sessions []models.TmuxSession) bool {
	ctx := context.Background()
	// Define styles
	keyStyle := core_theme.DefaultTheme.Highlight
	repoStyle := core_theme.DefaultTheme.Info
	pathStyle := core_theme.DefaultTheme.Success

	// Build rows
	var rows [][]string
	hasConfiguredSessions := false

	for _, s := range sessions {
		path := s.Path

		// Style the key
		styledKey := keyStyle.Render(s.Key)

		// Use configured repository name
		var repo string
		if s.Repository != "" {
			repo = repoStyle.Render(s.Repository)
		}

		if path != "" {
			hasConfiguredSessions = true
			path = pathStyle.Render(path)
		}

		rows = append(rows, []string{styledKey, repo, path})
	}

	// Create styled table
	t := tablecomponent.NewStyledTable().
		Headers("Key", "Repository", "Path").
		Rows(rows...)

	ulogKey.Info("Sessions table").
		Field("session_count", len(sessions)).
		Pretty(t.String()).
		PrettyOnly().
		Log(ctx)
	return hasConfiguredSessions
}

var keyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured session keys",
	// The new RunE function handles both styles
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			ulogKey.Info("No sessions configured").
				Pretty("No sessions configured").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Check the style flag to determine output format
		if listStyle == "compact" {
			keyStyle := core_theme.DefaultTheme.Highlight
			repoStyle := core_theme.DefaultTheme.Info

			var outputLines []string
			for _, s := range sessions {
				// Only show mapped sessions in compact view
				if s.Path != "" {
					repo := filepath.Base(s.Path)
					line := fmt.Sprintf("%s: %s", keyStyle.Render(s.Key), repoStyle.Render(repo))
					outputLines = append(outputLines, line)
				}
			}
			ulogKey.Info("Sessions list").
				Field("session_count", len(outputLines)).
				Field("style", "compact").
				Pretty(strings.Join(outputLines, "\n")).
				PrettyOnly().
				Log(ctx)
		} else {
			// Default to the existing table display
			displaySessionsTable(sessions)
		}
		return nil
	},
}

var keyUpdateCmd = &cobra.Command{
	Use:   "update [current-key]",
	Short: "Update the key binding for a tmux session",
	Long:  `Update the key binding for an existing tmux session. If no key is provided, shows all sessions for selection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			ulogKey.Info("No sessions configured").
				Pretty("No sessions configured").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show all sessions and let user choose
			ulogKey.Info("Session selection prompt").
				Pretty("Available sessions:\n").
				PrettyOnly().
				Log(ctx)
			displaySessionsTable(sessions)
			ulogKey.Info("Key input prompt").
				Pretty("\nEnter the key of the session to update: ").
				PrettyOnly().
				Log(ctx)

			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			targetKey = strings.TrimSpace(input)
		}

		// Find the session with the target key
		var targetSessionIndex int = -1
		for i, s := range sessions {
			if s.Key == targetKey {
				targetSessionIndex = i
				break
			}
		}

		if targetSessionIndex == -1 {
			return fmt.Errorf("no session found with key '%s'", targetKey)
		}

		targetSession := sessions[targetSessionIndex]
		if targetSession.Path == "" {
			return fmt.Errorf("session '%s' has no configured path", targetKey)
		}

		repo := filepath.Base(targetSession.Path)
		ulogKey.Info("Current session display").
			Field("key", targetSession.Key).
			Field("repo", repo).
			Field("path", targetSession.Path).
			Pretty(fmt.Sprintf("\nCurrent session: %s -> %s (%s)\n", targetSession.Key, repo, targetSession.Path)).
			PrettyOnly().
			Log(ctx)

		// Get available keys
		availableKeys := mgr.GetAvailableKeys()
		usedKeys := make(map[string]bool)
		for _, s := range sessions {
			if s.Path != "" {
				usedKeys[s.Key] = true
			}
		}

		// Show available keys
		var freeKeys []string
		for _, k := range availableKeys {
			if !usedKeys[k] {
				freeKeys = append(freeKeys, k)
			}
		}
		ulogKey.Info("Available keys display").
			Field("free_keys", freeKeys).
			Pretty(fmt.Sprintf("\nAvailable keys:\n  %s\n\nEnter new key (or press Enter to cancel): ", strings.Join(freeKeys, ", "))).
			PrettyOnly().
			Log(ctx)

		reader := bufio.NewReader(os.Stdin)
		newKey, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		newKey = strings.TrimSpace(newKey)

		if newKey == "" {
			ulogKey.Info("Update cancelled").
				Pretty("Update cancelled").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Validate new key
		if usedKeys[newKey] && newKey != targetKey {
			return fmt.Errorf("key '%s' is already in use", newKey)
		}

		validKey := false
		for _, k := range availableKeys {
			if k == newKey {
				validKey = true
				break
			}
		}
		if !validKey {
			return fmt.Errorf("'%s' is not a valid key. Available keys: %s", newKey, strings.Join(availableKeys, ", "))
		}

		// Update the session key
		err = mgr.UpdateSessionKey(targetKey, newKey)
		if err != nil {
			return fmt.Errorf("failed to update session key: %w", err)
		}

		ulogKey.Success("Session key updated").
			Field("old_key", targetKey).
			Field("new_key", newKey).
			Pretty(fmt.Sprintf("\n%s Session key updated: %s -> %s\n", core_theme.IconSuccess, targetKey, newKey)).
			PrettyOnly().
			Log(ctx)

		// Regenerate bindings
		ulogKey.Progress("Regenerating bindings").
			Pretty(core_theme.IconRunning + " Regenerating tmux bindings...").
			PrettyOnly().
			Log(ctx)
		if err := mgr.RegenerateBindings(); err != nil {
			return fmt.Errorf("failed to regenerate bindings: %w", err)
		}

		ulogKey.Success("Bindings regenerated").
			Pretty(core_theme.IconSuccess + " Done! Remember to reload your tmux configuration.").
			PrettyOnly().
			Log(ctx)
		return nil
	},
}

var keyEditCmd = &cobra.Command{
	Use:   "edit [key]",
	Short: "Edit the details of a tmux session (path, repository, description)",
	Long:  `Edit the path, repository name, and description for an existing tmux session. If no key is provided, shows all sessions for selection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			ulogKey.Info("No sessions configured").
				Pretty("No sessions configured").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show all sessions and let user choose
			ulogKey.Info("Session selection prompt").
				Pretty("Available sessions:\n").
				PrettyOnly().
				Log(ctx)
			displaySessionsTable(sessions)
			ulogKey.Info("Key input prompt").
				Pretty("\nEnter the key of the session to edit: ").
				PrettyOnly().
				Log(ctx)

			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			targetKey = strings.TrimSpace(input)
		}

		// Find the session with the target key
		var targetSessionIndex int = -1
		for i, s := range sessions {
			if s.Key == targetKey {
				targetSessionIndex = i
				break
			}
		}

		if targetSessionIndex == -1 {
			return fmt.Errorf("no session found with key '%s'", targetKey)
		}

		targetSession := sessions[targetSessionIndex]
		prettyOutput := fmt.Sprintf("\nCurrent session details for key '%s':\n  Path: %s\n", targetKey, targetSession.Path)
		if targetSession.Path != "" {
			prettyOutput += fmt.Sprintf("  Repository: %s (extracted from path)\n", filepath.Base(targetSession.Path))
		}
		prettyOutput += "\n"

		ulogKey.Info("Session details display").
			Field("key", targetKey).
			Field("path", targetSession.Path).
			Pretty(prettyOutput).
			PrettyOnly().
			Log(ctx)

		reader := bufio.NewReader(os.Stdin)

		// Get new path
		ulogKey.Info("Path input prompt").
			Pretty("Enter new path (press Enter to keep current): ").
			PrettyOnly().
			Log(ctx)
		newPath, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		newPath = strings.TrimSpace(newPath)
		if newPath == "" {
			newPath = targetSession.Path
		}

		// Create updated session
		updatedSession := models.TmuxSession{
			Key:         targetKey,
			Path:        newPath,
			Repository:  "", // Repository will be extracted from path
			Description: "", // No longer using description
		}

		// Update the session
		err = mgr.UpdateSingleSession(targetKey, updatedSession)
		if err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}

		ulogKey.Success("Session updated").
			Field("key", targetKey).
			Field("new_path", newPath).
			Pretty(fmt.Sprintf("\n%s Session '%s' updated successfully!\n", core_theme.IconSuccess, targetKey)).
			PrettyOnly().
			Log(ctx)

		// Regenerate bindings if path changed
		if newPath != targetSession.Path {
			ulogKey.Progress("Regenerating bindings").
				Pretty(core_theme.IconRunning + " Regenerating tmux bindings...").
				PrettyOnly().
				Log(ctx)
			if err := mgr.RegenerateBindings(); err != nil {
				return fmt.Errorf("failed to regenerate bindings: %w", err)
			}
			ulogKey.Success("Bindings regenerated").
				Pretty(core_theme.IconSuccess + " Done! Remember to reload your tmux configuration.").
				PrettyOnly().
				Log(ctx)
		}

		return nil
	},
}

var keyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new session from available projects in search paths",
	Long:  `Discover projects from configured search paths and quickly map them to available keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}

		// Get current sessions to see which keys are available
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		// Get available keys
		availableKeys := mgr.GetAvailableKeys()
		usedKeys := make(map[string]bool)
		for _, s := range sessions {
			if s.Path != "" {
				usedKeys[s.Key] = true
			}
		}

		// Get free keys
		var freeKeys []string
		for _, k := range availableKeys {
			if !usedKeys[k] {
				freeKeys = append(freeKeys, k)
			}
		}

		if len(freeKeys) == 0 {
			ulogKey.Warn("No available keys").
				Pretty(core_theme.IconWarning + " No available keys! All keys are already mapped to sessions.").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Get available projects from search paths
		projects, err := mgr.GetAvailableProjects()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}

		// Set parent ecosystem for cloned repos
		setParentForClonedProjectsInKeyAdd(projects)

		if len(projects) == 0 {
			ulogKey.Warn("No projects found").
				Pretty(core_theme.IconWarning + " No projects found in search paths!\n\nMake sure your search paths are configured in one of:\n  ~/.config/tmux/project-search-paths.yaml\n  ~/.config/grove/project-search-paths.yaml").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Filter out projects that are already mapped
		existingPaths := make(map[string]bool)
		for _, s := range sessions {
			if s.Path != "" {
				absPath, _ := filepath.Abs(expandPath(s.Path))
				existingPaths[absPath] = true
			}
		}

		var availableProjects []manager.DiscoveredProject
		for _, p := range projects {
			absPath, _ := filepath.Abs(expandPath(p.Path))
			if !existingPaths[absPath] {
				availableProjects = append(availableProjects, p)
			}
		}

		if len(availableProjects) == 0 {
			ulogKey.Info("All projects mapped").
				Pretty(core_theme.IconInfo + " All discovered projects are already mapped to keys!").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Show available projects in a table
		ulogKey.Info("Available projects display").
			Field("project_count", len(availableProjects)).
			Pretty("Available projects from search paths:\n").
			PrettyOnly().
			Log(ctx)

		// Build project table
		indexStyle := core_theme.DefaultTheme.Warning
		projectStyle := core_theme.DefaultTheme.Info
		pathStyle := core_theme.DefaultTheme.Success

		var rows [][]string
		for i, p := range availableProjects {
			index := indexStyle.Render(fmt.Sprintf("%d", i+1))
			project := projectStyle.Render(p.Name)
			path := pathStyle.Render(p.Path)
			rows = append(rows, []string{index, project, path})
		}

		t := tablecomponent.NewStyledTable().
			Headers("#", "Project", "Path").
			Rows(rows...)

		ulogKey.Info("Projects table").
			Pretty(t.String() + "\n").
			PrettyOnly().
			Log(ctx)

		// Get project selection
		reader := bufio.NewReader(os.Stdin)
		ulogKey.Info("Project selection prompt").
			Pretty("Select project number (or press Enter to cancel): ").
			PrettyOnly().
			Log(ctx)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)

		if input == "" {
			ulogKey.Info("Selection cancelled").
				Pretty("Cancelled").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		var projectIndex int
		if _, err := fmt.Sscanf(input, "%d", &projectIndex); err != nil || projectIndex < 1 || projectIndex > len(availableProjects) {
			return fmt.Errorf("invalid project selection")
		}

		selectedProject := availableProjects[projectIndex-1]

		// Show available keys
		ulogKey.Info("Key selection prompt").
			Field("project", selectedProject.Name).
			Field("available_keys", freeKeys).
			Pretty(fmt.Sprintf("\nSelected project: %s\nAvailable keys: %s\n\nEnter key to assign (or press Enter to cancel): ",
				selectedProject.Name, strings.Join(freeKeys, ", "))).
			PrettyOnly().
			Log(ctx)

		keyInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		keyInput = strings.TrimSpace(keyInput)

		if keyInput == "" {
			ulogKey.Info("Selection cancelled").
				Pretty("Cancelled").
				PrettyOnly().
				Log(ctx)
			return nil
		}

		// Validate key
		validKey := false
		for _, k := range freeKeys {
			if k == keyInput {
				validKey = true
				break
			}
		}
		if !validKey {
			return fmt.Errorf("'%s' is not an available key. Available keys: %s", keyInput, strings.Join(freeKeys, ", "))
		}

		// Create new session
		newSession := models.TmuxSession{
			Key:         keyInput,
			Path:        selectedProject.Path,
			Repository:  "", // Will be extracted from path
			Description: "", // No longer used
		}

		// Update the session
		err = mgr.UpdateSingleSession(keyInput, newSession)
		if err != nil {
			return fmt.Errorf("failed to add session: %w", err)
		}

		ulogKey.Success("Session added").
			Field("key", keyInput).
			Field("project", selectedProject.Name).
			Field("path", selectedProject.Path).
			Pretty(fmt.Sprintf("\n%s Successfully added session:\n  Key: %s\n  Project: %s\n  Path: %s\n",
				core_theme.IconSuccess, keyInput, selectedProject.Name, selectedProject.Path)).
			PrettyOnly().
			Log(ctx)

		// Regenerate bindings
		ulogKey.Progress("Regenerating bindings").
			Pretty("\n" + core_theme.IconRunning + " Regenerating tmux bindings...").
			PrettyOnly().
			Log(ctx)
		if err := mgr.RegenerateBindings(); err != nil {
			return fmt.Errorf("failed to regenerate bindings: %w", err)
		}

		ulogKey.Success("Bindings regenerated").
			Pretty(core_theme.IconSuccess + " Done! Remember to reload your tmux configuration.").
			PrettyOnly().
			Log(ctx)
		return nil
	},
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

var keyUnmapCmd = &cobra.Command{
	Use:   "unmap [key]",
	Short: "Unmap a session from its key binding",
	Long:  `Remove the mapping for a specific key, making it available for future use.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show mapped sessions
			ulogKey.Info("Mapped sessions display").
				Pretty("Mapped sessions:\n").
				PrettyOnly().
				Log(ctx)

			var mappedSessions []models.TmuxSession
			for _, s := range sessions {
				if s.Path != "" {
					mappedSessions = append(mappedSessions, s)
				}
			}

			if len(mappedSessions) == 0 {
				ulogKey.Info("No sessions mapped").
					Pretty("No sessions are currently mapped").
					PrettyOnly().
					Log(ctx)
				return nil
			}

			displaySessionsTable(mappedSessions)
			ulogKey.Info("Key input prompt").
				Pretty("\nEnter the key to unmap (or press Enter to cancel): ").
				PrettyOnly().
				Log(ctx)

			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			targetKey = strings.TrimSpace(input)

			if targetKey == "" {
				ulogKey.Info("Unmap cancelled").
					Pretty("Cancelled").
					PrettyOnly().
					Log(ctx)
				return nil
			}
		}

		// Find and unmap the session
		found := false
		for i, s := range sessions {
			if s.Key == targetKey {
				found = true
				if s.Path == "" {
					return fmt.Errorf("key '%s' is not mapped", targetKey)
				}

				// Clear the mapping
				sessions[i].Path = ""
				sessions[i].Repository = ""
				sessions[i].Description = ""

				// Update sessions
				err = mgr.UpdateSessions(sessions)
				if err != nil {
					return fmt.Errorf("failed to update sessions: %w", err)
				}

				ulogKey.Success("Key unmapped").
					Field("key", targetKey).
					Pretty(fmt.Sprintf("%s Unmapped key '%s'\n", core_theme.IconSuccess, targetKey)).
					PrettyOnly().
					Log(ctx)

				// Regenerate bindings
				ulogKey.Progress("Regenerating bindings").
					Pretty(core_theme.IconRunning + " Regenerating tmux bindings...").
					PrettyOnly().
					Log(ctx)
				if err := mgr.RegenerateBindings(); err != nil {
					return fmt.Errorf("failed to regenerate bindings: %w", err)
				}

				// Try to reload tmux config
				if os.Getenv("TMUX") != "" {
					cmd := exec.Command("tmux", "source-file", expandPath("~/.tmux.conf"))
					if err := cmd.Run(); err == nil {
						ulogKey.Success("Tmux config reloaded").
							Pretty(core_theme.IconSuccess + " Done! Tmux configuration reloaded.").
							PrettyOnly().
							Log(ctx)
					} else {
						ulogKey.Success("Bindings regenerated").
							Pretty(core_theme.IconSuccess + " Done! Remember to reload your tmux configuration.").
							PrettyOnly().
							Log(ctx)
					}
				} else {
					ulogKey.Success("Bindings regenerated").
						Pretty(core_theme.IconSuccess + " Done! Remember to reload your tmux configuration.").
						PrettyOnly().
						Log(ctx)
				}
				break
			}
		}

		if !found {
			return fmt.Errorf("key '%s' not found", targetKey)
		}

		return nil
	},
}

// setParentForClonedProjectsInKeyAdd modifies a slice of projects in-place, setting the
// parent ecosystem path for any projects cloned via `cx repo`.
func setParentForClonedProjectsInKeyAdd(projects []manager.DiscoveredProject) {
	var clonedProjectIndices []int
	for i := range projects {
		if projects[i].Kind == workspace.KindNonGroveRepo {
			clonedProjectIndices = append(clonedProjectIndices, i)
		}
	}

	if len(clonedProjectIndices) == 0 {
		return
	}

	firstClonedProject := projects[clonedProjectIndices[0]]
	clonedRepoRoot := filepath.Dir(firstClonedProject.Path)

	for _, idx := range clonedProjectIndices {
		projects[idx].ParentEcosystemPath = clonedRepoRoot
		projects[idx].RootEcosystemPath = clonedRepoRoot
	}
}

func init() {
	// Add the new --style flag to the command
	keyListCmd.Flags().StringVar(&listStyle, "style", "table", "Output style: table or compact")

	keyCmd.AddCommand(keyListCmd)
	keyCmd.AddCommand(keyUpdateCmd)
	keyCmd.AddCommand(keyEditCmd)
	keyCmd.AddCommand(keyAddCmd)
	keyCmd.AddCommand(keyUnmapCmd)
}
