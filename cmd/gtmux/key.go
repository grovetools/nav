package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage tmux session key bindings",
	Long:  "Commands for managing tmux session key bindings including updating keys and editing session details.",
}

// displaySessionsTable shows sessions in a styled table and returns true if any sessions have paths
func displaySessionsTable(sessions []models.TmuxSession) bool {
	// Define styles
	re := lipgloss.NewRenderer(os.Stdout)
	baseStyle := re.NewStyle().Padding(0, 1)
	headerStyle := baseStyle.Copy().Bold(true).Foreground(lipgloss.Color("255"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Bold(true)
	repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#95e1d3"))

	// Build rows
	var rows [][]string
	hasConfiguredSessions := false
	
	for _, s := range sessions {
		path := s.Path
		
		// Style the key
		styledKey := keyStyle.Render(s.Key)
		
		// Extract repository name from path
		var repo string
		if path == "" {
			// Leave empty for unconfigured sessions
			path = ""
			repo = ""
		} else {
			hasConfiguredSessions = true
			// Extract last component of path as repo name
			repo = filepath.Base(path)
			repo = repoStyle.Render(repo)
			path = pathStyle.Render(path)
		}
		
		rows = append(rows, []string{styledKey, repo, path})
	}

	// Create the table
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers("Key", "Repository", "Path").
		Rows(rows...)

	// Apply styling - only for headers since content is pre-styled
	t.StyleFunc(func(row, col int) lipgloss.Style {
		if row == 0 {
			return headerStyle
		}
		// Return minimal style to preserve pre-styled content
		return lipgloss.NewStyle().Padding(0, 1)
	})

	fmt.Println(t)
	return hasConfiguredSessions
}

var keyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured session keys",
	RunE:  listCmd.RunE, // Use the same handler as the root list command
}

var keyUpdateCmd = &cobra.Command{
	Use:   "update [current-key]",
	Short: "Update the key binding for a tmux session",
	Long:  `Update the key binding for an existing tmux session. If no key is provided, shows all sessions for selection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager(configDir, sessionsFile)
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions configured")
			return nil
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show all sessions and let user choose
			fmt.Println("Available sessions:")
			fmt.Println()
			displaySessionsTable(sessions)
			fmt.Println()
			fmt.Print("Enter the key of the session to update: ")
			
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
		fmt.Printf("\nCurrent session: %s -> %s (%s)\n", targetSession.Key, repo, targetSession.Path)

		// Get available keys
		availableKeys := mgr.GetAvailableKeys()
		usedKeys := make(map[string]bool)
		for _, s := range sessions {
			if s.Path != "" {
				usedKeys[s.Key] = true
			}
		}

		// Show available keys
		fmt.Println("\nAvailable keys:")
		var freeKeys []string
		for _, k := range availableKeys {
			if !usedKeys[k] {
				freeKeys = append(freeKeys, k)
			}
		}
		fmt.Println("  " + strings.Join(freeKeys, ", "))

		fmt.Print("\nEnter new key (or press Enter to cancel): ")
		reader := bufio.NewReader(os.Stdin)
		newKey, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		newKey = strings.TrimSpace(newKey)

		if newKey == "" {
			fmt.Println("Update cancelled")
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

		fmt.Printf("\nSession key updated: %s -> %s\n", targetKey, newKey)

		// Regenerate bindings
		fmt.Println("Regenerating tmux bindings...")
		if err := mgr.RegenerateBindings(); err != nil {
			return fmt.Errorf("failed to regenerate bindings: %w", err)
		}

		fmt.Println("Done! Remember to reload your tmux configuration.")
		return nil
	},
}

var keyEditCmd = &cobra.Command{
	Use:   "edit [key]",
	Short: "Edit the details of a tmux session (path, repository, description)",
	Long:  `Edit the path, repository name, and description for an existing tmux session. If no key is provided, shows all sessions for selection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager(configDir, sessionsFile)
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions configured")
			return nil
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show all sessions and let user choose
			fmt.Println("Available sessions:")
			fmt.Println()
			displaySessionsTable(sessions)
			fmt.Println()
			fmt.Print("Enter the key of the session to edit: ")
			
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
		fmt.Printf("\nCurrent session details for key '%s':\n", targetKey)
		fmt.Printf("  Path: %s\n", targetSession.Path)
		if targetSession.Path != "" {
			fmt.Printf("  Repository: %s (extracted from path)\n", filepath.Base(targetSession.Path))
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		// Get new path
		fmt.Printf("Enter new path (press Enter to keep current): ")
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

		fmt.Printf("\nSession '%s' updated successfully!\n", targetKey)

		// Regenerate bindings if path changed
		if newPath != targetSession.Path {
			fmt.Println("Regenerating tmux bindings...")
			if err := mgr.RegenerateBindings(); err != nil {
				return fmt.Errorf("failed to regenerate bindings: %w", err)
			}
			fmt.Println("Done! Remember to reload your tmux configuration.")
		}

		return nil
	},
}

var keyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new session from available projects in search paths",
	Long:  `Discover projects from configured search paths and quickly map them to available keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager(configDir, sessionsFile)
		
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
			fmt.Println("No available keys! All keys are already mapped to sessions.")
			return nil
		}

		// Get available projects from search paths
		projects, err := mgr.GetAvailableProjects()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found in search paths!")
			fmt.Println("\nMake sure your search paths are configured in one of:")
			fmt.Println("  ~/.config/tmux/project-search-paths.yaml")
			fmt.Println("  ~/.config/grove/project-search-paths.yaml")
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

		var availableProjects []string
		for _, p := range projects {
			absPath, _ := filepath.Abs(expandPath(p))
			if !existingPaths[absPath] {
				availableProjects = append(availableProjects, p)
			}
		}

		if len(availableProjects) == 0 {
			fmt.Println("All discovered projects are already mapped to keys!")
			return nil
		}

		// Show available projects in a table
		fmt.Println("Available projects from search paths:")
		fmt.Println()
		
		// Build project table
		re := lipgloss.NewRenderer(os.Stdout)
		headerStyle := re.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Padding(0, 1)
		indexStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Bold(true)
		projectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4"))
		pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#95e1d3"))

		var rows [][]string
		for i, p := range availableProjects {
			index := indexStyle.Render(fmt.Sprintf("%d", i+1))
			project := projectStyle.Render(filepath.Base(p))
			path := pathStyle.Render(p)
			rows = append(rows, []string{index, project, path})
		}

		t := table.New().
			Border(lipgloss.NormalBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
			Headers("#", "Project", "Path").
			Rows(rows...)

		t.StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

		fmt.Println(t)
		fmt.Println()

		// Get project selection
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Select project number (or press Enter to cancel): ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		input = strings.TrimSpace(input)
		
		if input == "" {
			fmt.Println("Cancelled")
			return nil
		}

		var projectIndex int
		if _, err := fmt.Sscanf(input, "%d", &projectIndex); err != nil || projectIndex < 1 || projectIndex > len(availableProjects) {
			return fmt.Errorf("invalid project selection")
		}

		selectedProject := availableProjects[projectIndex-1]
		
		// Show available keys
		fmt.Printf("\nSelected project: %s\n", selectedProject)
		fmt.Printf("Available keys: %s\n", strings.Join(freeKeys, ", "))
		fmt.Print("\nEnter key to assign (or press Enter to cancel): ")
		
		keyInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		keyInput = strings.TrimSpace(keyInput)
		
		if keyInput == "" {
			fmt.Println("Cancelled")
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
			Path:        selectedProject,
			Repository:  "", // Will be extracted from path
			Description: "", // No longer used
		}

		// Update the session
		err = mgr.UpdateSingleSession(keyInput, newSession)
		if err != nil {
			return fmt.Errorf("failed to add session: %w", err)
		}

		fmt.Printf("\nSuccessfully added session:\n")
		fmt.Printf("  Key: %s\n", keyInput)
		fmt.Printf("  Project: %s\n", filepath.Base(selectedProject))
		fmt.Printf("  Path: %s\n", selectedProject)

		// Regenerate bindings
		fmt.Println("\nRegenerating tmux bindings...")
		if err := mgr.RegenerateBindings(); err != nil {
			return fmt.Errorf("failed to regenerate bindings: %w", err)
		}

		fmt.Println("Done! Remember to reload your tmux configuration.")
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
		mgr := tmux.NewManager(configDir, sessionsFile)
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		var targetKey string
		if len(args) > 0 {
			targetKey = args[0]
		} else {
			// Interactive mode: show mapped sessions
			fmt.Println("Mapped sessions:")
			fmt.Println()
			
			var mappedSessions []models.TmuxSession
			for _, s := range sessions {
				if s.Path != "" {
					mappedSessions = append(mappedSessions, s)
				}
			}
			
			if len(mappedSessions) == 0 {
				fmt.Println("No sessions are currently mapped")
				return nil
			}
			
			displaySessionsTable(mappedSessions)
			fmt.Println()
			fmt.Print("Enter the key to unmap (or press Enter to cancel): ")
			
			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
			targetKey = strings.TrimSpace(input)
			
			if targetKey == "" {
				fmt.Println("Cancelled")
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
				
				fmt.Printf("Unmapped key '%s'\n", targetKey)
				
				// Regenerate bindings
				fmt.Println("Regenerating tmux bindings...")
				if err := mgr.RegenerateBindings(); err != nil {
					return fmt.Errorf("failed to regenerate bindings: %w", err)
				}
				
				// Try to reload tmux config
				if os.Getenv("TMUX") != "" {
					cmd := exec.Command("tmux", "source-file", expandPath("~/.tmux.conf"))
					if err := cmd.Run(); err == nil {
						fmt.Println("Done! Tmux configuration reloaded.")
					} else {
						fmt.Println("Done! Remember to reload your tmux configuration.")
					}
				} else {
					fmt.Println("Done! Remember to reload your tmux configuration.")
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

func init() {
	keyCmd.AddCommand(keyListCmd)
	keyCmd.AddCommand(keyUpdateCmd)
	keyCmd.AddCommand(keyEditCmd)
	keyCmd.AddCommand(keyAddCmd)
	keyCmd.AddCommand(keyUnmapCmd)
}