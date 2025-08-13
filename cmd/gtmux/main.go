package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/version"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var (
	configDir    string
	sessionsFile string
)

var rootCmd = &cobra.Command{
	Use:   "gtmux",
	Short: "Grove tmux management tool",
	Long:  `A CLI tool for managing tmux sessions and configurations in the Grove ecosystem.`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tmux sessions from configuration",
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

		// Find the longest key for alignment
		maxKeyLen := 0
		for _, s := range sessions {
			if len(s.Key) > maxKeyLen {
				maxKeyLen = len(s.Key)
			}
		}

		fmt.Printf("%-*s  %-30s  %-40s  %s\n", maxKeyLen, "Key", "Repository", "Path", "Description")
		fmt.Printf("%s  %s  %s  %s\n", 
			strings.Repeat("-", maxKeyLen),
			strings.Repeat("-", 30),
			strings.Repeat("-", 40),
			strings.Repeat("-", 20))

		for _, s := range sessions {
			path := s.Path
			if path == "" {
				path = "<not configured>"
			}
			repo := s.Repository
			if repo == "" {
				repo = "<not configured>"
			}
			fmt.Printf("%-*s  %-30s  %-40s  %s\n", maxKeyLen, s.Key, repo, path, s.Description)
		}

		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show git status for configured sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := tmux.NewManager(configDir, sessionsFile)
		statuses, err := mgr.GetGitStatuses()
		if err != nil {
			return fmt.Errorf("failed to get git statuses: %w", err)
		}

		if len(statuses) == 0 {
			fmt.Println("No repositories configured")
			return nil
		}

		// Find the longest repo name for alignment
		maxRepoLen := 0
		for repo := range statuses {
			if len(repo) > maxRepoLen {
				maxRepoLen = len(repo)
			}
		}

		fmt.Printf("%-*s  %s\n", maxRepoLen, "Repository", "Status")
		fmt.Printf("%s  %s\n", strings.Repeat("-", maxRepoLen), strings.Repeat("-", 30))

		for repo, status := range statuses {
			fmt.Printf("%-*s  %s\n", maxRepoLen, repo, status.Status)
		}

		return nil
	},
}

func init() {
	vInfo := version.GetInfo()
	rootCmd.Version = vInfo.Version
	rootCmd.SetVersionTemplate(`{{.Name}} {{.Version}}
`)

	// Add global flags
	defaultConfigDir := filepath.Join(os.Getenv("HOME"), ".config", "grove")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", defaultConfigDir, "Configuration directory")
	rootCmd.PersistentFlags().StringVar(&sessionsFile, "sessions-file", "", "Sessions file path (default: <config-dir>/tmux-sessions.yaml)")

	// Add commands
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(launchCmd)
	rootCmd.AddCommand(waitCmd)
	rootCmd.AddCommand(startCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}