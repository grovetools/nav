package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var sessionizeAddCmd = &cobra.Command{
	Use:     "add [path]",
	Short:   "Add an explicit project to sessionizer",
	Long:    `Add a specific project path to the sessionizer that won't be discovered automatically through search paths.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the path to add
		var projectPath string
		if len(args) > 0 {
			projectPath = args[0]
		} else {
			// Use current directory if no path provided
			var err error
			projectPath, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
		}

		// Expand and validate the path
		projectPath = expandPath(projectPath)
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}

		// Check if directory exists
		info, err := os.Stat(absPath)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("path does not exist or is not a directory: %s", absPath)
		}

		// Find the config file
		configFile := findConfigFile()
		
		// Load existing config
		var config manager.ProjectSearchConfig
		data, err := os.ReadFile(configFile)
		if err == nil {
			if err := yaml.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse config file: %w", err)
			}
		}

		// Check if project already exists
		for _, ep := range config.ExplicitProjects {
			if expandPath(ep.Path) == absPath {
				if ep.Enabled {
					fmt.Printf("Project already added: %s\n", absPath)
					return nil
				} else {
					// Re-enable it
					ep.Enabled = true
					fmt.Printf("Re-enabled project: %s\n", absPath)
					return saveConfig(configFile, &config)
				}
			}
		}

		// Add new explicit project
		projectName := filepath.Base(absPath)
		config.ExplicitProjects = append(config.ExplicitProjects, manager.ExplicitProject{
			Path:        absPath,
			Name:        projectName,
			Description: fmt.Sprintf("Explicitly added project: %s", projectName),
			Enabled:     true,
		})

		// Save config
		if err := saveConfig(configFile, &config); err != nil {
			return err
		}

		fmt.Printf("Successfully added project: %s\n", absPath)
		fmt.Printf("Config saved to: %s\n", configFile)
		return nil
	},
}

var sessionizeRemoveCmd = &cobra.Command{
	Use:     "remove [path]",
	Short:   "Remove an explicit project from sessionizer",
	Long:    `Remove a specific project from the explicit projects list.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get the path to remove
		var projectPath string
		if len(args) > 0 {
			projectPath = args[0]
		} else {
			// Use current directory if no path provided
			var err error
			projectPath, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
		}

		// Expand the path
		projectPath = expandPath(projectPath)
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}

		// Find the config file
		configFile := findConfigFile()
		
		// Load existing config
		var config manager.ProjectSearchConfig
		data, err := os.ReadFile(configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
		
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse config file: %w", err)
		}

		// Find and remove the project
		found := false
		newProjects := []manager.ExplicitProject{}
		for _, ep := range config.ExplicitProjects {
			if expandPath(ep.Path) == absPath {
				found = true
				fmt.Printf("Removed project: %s\n", absPath)
			} else {
				newProjects = append(newProjects, ep)
			}
		}

		if !found {
			return fmt.Errorf("project not found in explicit projects: %s", absPath)
		}

		config.ExplicitProjects = newProjects

		// Save config
		if err := saveConfig(configFile, &config); err != nil {
			return err
		}

		return nil
	},
}

func findConfigFile() string {
	// Try multiple locations for the config file
	possiblePaths := []string{
		expandPath("~/.config/tmux/project-search-paths.yaml"),
		expandPath("~/.config/grove/project-search-paths.yaml"),
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	
	// Default to tmux config location
	return expandPath("~/.config/tmux/project-search-paths.yaml")
}

func saveConfig(configFile string, config *manager.ProjectSearchConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal config
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func init() {
	sessionizeCmd.AddCommand(sessionizeAddCmd)
	sessionizeCmd.AddCommand(sessionizeRemoveCmd)
}