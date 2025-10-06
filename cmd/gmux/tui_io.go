package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
)

// claudeSession represents the structure of a Claude session from grove-hooks
type claudeSession struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	PID                  int    `json:"pid"`
	Status               string `json:"status"`
	WorkingDirectory     string `json:"working_directory"`
	StateDuration        string `json:"state_duration"`
	StateDurationSeconds int    `json:"state_duration_seconds"`
}

// claudeSessionUpdateMsg is sent when claude session data is fetched
type claudeSessionUpdateMsg struct {
	sessions []manager.DiscoveredProject
}

// tickMsg is sent periodically to refresh git status
type tickMsg time.Time

// projectsUpdateMsg is sent when the list of discovered projects is updated
type projectsUpdateMsg struct {
	projects []manager.DiscoveredProject
}

// runningSessionsUpdateMsg is sent with the latest list of running tmux sessions
type runningSessionsUpdateMsg struct {
	sessions map[string]bool // A set of session names for quick lookups
}

// keyMapUpdateMsg is sent when the key mappings from tmux-sessions.yaml are reloaded
type keyMapUpdateMsg struct {
	keyMap   map[string]string     // map[path]key
	sessions []models.TmuxSession // Also pass the full session list
}

// getWorktreeParent checks if a path is a Git worktree and returns the parent path
func getWorktreeParent(path string) string {
	// Check if this is inside a .grove-worktrees directory
	if strings.Contains(path, ".grove-worktrees") {
		parts := strings.Split(path, ".grove-worktrees")
		if len(parts) >= 1 {
			return parts[0]
		}
	}
	return ""
}

// tickCmd returns a command that sends a tick message after a delay
func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchClaudeSessionsCmd returns a command that fetches claude sessions in the background
func fetchClaudeSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions := fetchClaudeSessions()
		return claudeSessionUpdateMsg{sessions: sessions}
	}
}

// fetchClaudeSessions fetches active Claude sessions from grove-hooks
func fetchClaudeSessions() []manager.DiscoveredProject {
	var claudeSessionProjects []manager.DiscoveredProject

	// Execute `grove-hooks sessions list --active --json`
	groveHooksPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "grove-hooks")
	var cmd *exec.Cmd
	if _, err := os.Stat(groveHooksPath); err == nil {
		cmd = exec.Command(groveHooksPath, "sessions", "list", "--active", "--json")
	} else {
		cmd = exec.Command("grove-hooks", "sessions", "list", "--active", "--json")
	}

	output, err := cmd.Output()
	if err == nil {
		var claudeSessions []claudeSession
		if json.Unmarshal(output, &claudeSessions) == nil {
			for _, session := range claudeSessions {
				// Only include sessions with type "claude_session"
				if session.Type == "claude_session" && session.WorkingDirectory != "" {
					absPath, err := filepath.Abs(expandPath(session.WorkingDirectory))
					if err == nil {
						cleanPath := filepath.Clean(absPath)

						sessionProject := manager.DiscoveredProject{
							ProjectInfo: workspace.ProjectInfo{
								Name: filepath.Base(cleanPath),
								Path: cleanPath,
								ClaudeSession: &workspace.ClaudeSessionInfo{
									ID:       session.ID,
									PID:      session.PID,
									Status:   session.Status,
									Duration: session.StateDuration,
								},
							},
						}

						if parentPath := getWorktreeParent(cleanPath); parentPath != "" {
							sessionProject.ParentPath = parentPath
							sessionProject.IsWorktree = true
						}

						claudeSessionProjects = append(claudeSessionProjects, sessionProject)
					}
				}
			}
		}
	}

	return claudeSessionProjects
}

// fetchProjectsCmd returns a command that re-scans the configured search paths
// and fetches Git status for all discovered projects
func fetchProjectsCmd(mgr *tmux.Manager, fetchGit, fetchClaude, fetchNotes bool) tea.Cmd {
	return func() tea.Msg {
		// Fetch enrichment data for all projects
		enrichOpts := buildEnrichmentOptions(fetchGit, fetchClaude, fetchNotes)
		projects, _ := mgr.GetAvailableProjectsWithOptions(enrichOpts)

		// Sort by access history
		if history, err := mgr.GetAccessHistory(); err == nil {
			projects = history.SortProjectsByAccess(projects)
		}

		return projectsUpdateMsg{projects: projects}
	}
}

// fetchRunningSessionsCmd returns a command that gets the list of currently running tmux sessions
func fetchRunningSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessionsMap := make(map[string]bool)
		if os.Getenv("TMUX") != "" {
			client, err := tmuxclient.NewClient()
			if err == nil {
				ctx := context.Background()
				sessionNames, _ := client.ListSessions(ctx)
				for _, name := range sessionNames {
					sessionsMap[name] = true
				}
			}
		}
		return runningSessionsUpdateMsg{sessions: sessionsMap}
	}
}

// fetchKeyMapCmd returns a command that reloads the tmux-sessions.yaml file
func fetchKeyMapCmd(mgr *tmux.Manager) tea.Cmd {
	return func() tea.Msg {
		keyMap := make(map[string]string)
		sessions, err := mgr.GetSessions()
		if err != nil {
			sessions = []models.TmuxSession{}
		}
		for _, s := range sessions {
			if s.Path != "" {
				expandedPath := expandPath(s.Path)
				absPath, err := filepath.Abs(expandedPath)
				if err == nil {
					cleanPath := filepath.Clean(absPath)
					keyMap[cleanPath] = s.Key
				}
			}
		}
		return keyMapUpdateMsg{keyMap: keyMap, sessions: sessions}
	}
}
