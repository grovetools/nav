package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/version"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var (
	configDir    string
	sessionsFile string
)

var rootCmd = &cobra.Command{
	Use:   "gmux",
	Short: "Grove tmux management tool",
	Long:  `A CLI tool for managing tmux sessions and configurations in the Grove ecosystem.`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tmux sessions from configuration (alias for 'key list')",
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

		// Check the style flag to determine output format
		if listStyle == "compact" {
			keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Bold(true)
			repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4"))

			var outputLines []string
			for _, s := range sessions {
				// Only show mapped sessions in compact view
				if s.Path != "" {
					repo := filepath.Base(s.Path)
					line := fmt.Sprintf("%s: %s", keyStyle.Render(s.Key), repoStyle.Render(repo))
					outputLines = append(outputLines, line)
				}
			}
			fmt.Println(strings.Join(outputLines, "\n"))
		} else {
			// Default to the existing table display
			displaySessionsTable(sessions)
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

// kmCmd is an alias for key manage
var kmCmd = &cobra.Command{
	Use:   "km",
	Short: "Alias for 'key manage' - Interactively manage tmux session key mappings",
	RunE:  keyManageCmd.RunE,
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

	// Add the --style flag to the alias command as well, so Cobra recognizes it.
	// It will modify the `listStyle` variable from the key.go file.
	listCmd.Flags().StringVar(&listStyle, "style", "table", "Output style: table or compact")

	// Add commands
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(sessionCmd)
	rootCmd.AddCommand(launchCmd)
	rootCmd.AddCommand(waitCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(keyCmd)
	rootCmd.AddCommand(kmCmd)
	rootCmd.AddCommand(sessionizeCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
