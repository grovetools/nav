package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattsolo1/grove-core/git"
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

// extendedGitStatus holds git status info plus additional stats
type extendedGitStatus struct {
	*git.StatusInfo
	LinesAdded   int
	LinesDeleted int
}

// gitStatusUpdateMsg is sent when git status data is fetched
type gitStatusUpdateMsg struct {
	statuses map[string]*extendedGitStatus
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

// fetchGitStatusForPath gets the git status for a specific path
func fetchGitStatusForPath(path string) (*extendedGitStatus, error) {
	cleanPath := filepath.Clean(path)

	// Check if it's a git repo before getting status
	if !git.IsGitRepo(cleanPath) {
		return nil, fmt.Errorf("not a git repository")
	}

	status, err := git.GetStatus(cleanPath)
	if err != nil {
		return nil, err
	}

	extStatus := &extendedGitStatus{
		StatusInfo: status,
	}

	// Get line stats using git diff --numstat
	cmd := exec.Command("git", "diff", "--numstat")
	cmd.Dir = cleanPath
	output, err := cmd.Output()
	if err == nil {
		extStatus.LinesAdded, extStatus.LinesDeleted = parseNumstat(string(output))
	}

	// Also get staged changes
	cmd = exec.Command("git", "diff", "--cached", "--numstat")
	cmd.Dir = cleanPath
	output, err = cmd.Output()
	if err == nil {
		stagedAdded, stagedDeleted := parseNumstat(string(output))
		extStatus.LinesAdded += stagedAdded
		extStatus.LinesDeleted += stagedDeleted
	}

	return extStatus, nil
}

// fetchGitStatusForOpenSessions gets the git status for all active tmux sessions.
func fetchGitStatusForOpenSessions() (map[string]*extendedGitStatus, error) {
	client, err := tmuxclient.NewClient()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	sessionNames, err := client.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]*extendedGitStatus)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sessionName := range sessionNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			path, err := client.GetSessionPath(ctx, name)
			if err != nil {
				return
			}

			cleanPath := filepath.Clean(path)

			// Check if it's a git repo before getting status
			if !git.IsGitRepo(cleanPath) {
				return
			}

			status, err := git.GetStatus(cleanPath)
			if err == nil {
				extStatus := &extendedGitStatus{
					StatusInfo: status,
				}

				// Get line stats using git diff --numstat
				cmd := exec.Command("git", "diff", "--numstat")
				cmd.Dir = cleanPath
				output, err := cmd.Output()
				if err == nil {
					extStatus.LinesAdded, extStatus.LinesDeleted = parseNumstat(string(output))
				}

				// Also get staged changes
				cmd = exec.Command("git", "diff", "--cached", "--numstat")
				cmd.Dir = cleanPath
				output, err = cmd.Output()
				if err == nil {
					stagedAdded, stagedDeleted := parseNumstat(string(output))
					extStatus.LinesAdded += stagedAdded
					extStatus.LinesDeleted += stagedDeleted
				}

				mu.Lock()
				statusMap[cleanPath] = extStatus
				mu.Unlock()
			}
		}(sessionName)
	}

	wg.Wait()
	return statusMap, nil
}

// parseNumstat parses git diff --numstat output and returns total lines added/deleted
func parseNumstat(output string) (added, deleted int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// Skip binary files (shown as "-")
			if fields[0] != "-" {
				if a, err := strconv.Atoi(fields[0]); err == nil {
					added += a
				}
			}
			if fields[1] != "-" {
				if d, err := strconv.Atoi(fields[1]); err == nil {
					deleted += d
				}
			}
		}
	}
	return added, deleted
}

// fetchGitStatusCmd returns a command that fetches git status in the background
func fetchGitStatusCmd() tea.Cmd {
	return func() tea.Msg {
		if os.Getenv("TMUX") == "" {
			return nil
		}
		statuses, _ := fetchGitStatusForOpenSessions()
		return gitStatusUpdateMsg{statuses: statuses}
	}
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

						// Try to fetch git status for this path
						if extStatus, err := fetchGitStatusForPath(cleanPath); err == nil {
							sessionProject.GitStatus = extStatus.StatusInfo
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
// It uses selective enrichment to only fetch Git status for active tmux sessions
func fetchProjectsCmd(mgr *tmux.Manager, fetchGit, fetchClaude bool) tea.Cmd {
	return func() tea.Msg {
		// Use selective enrichment to only fetch Git status for active sessions
		enrichOpts := buildEnrichmentOptions(fetchGit, fetchClaude)
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
