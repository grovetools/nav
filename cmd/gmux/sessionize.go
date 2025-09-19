package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

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


var sessionizeCmd = &cobra.Command{
	Use:     "sessionize",
	Aliases: []string{"sz"},
	Short:   "Quickly create or switch to tmux sessions from project directories",
	Long:    `Discover projects from configured search paths and quickly create or switch to tmux sessions. Shows Claude session status indicators when grove-hooks is installed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If a path is provided as argument, use it directly
		if len(args) > 0 {
			// Record access for direct path usage too
			mgr := tmux.NewManager(configDir, sessionsFile)
			_ = mgr.RecordProjectAccess(args[0])
			return sessionizeProject(args[0])
		}

		// Otherwise, show the interactive project picker
		mgr := tmux.NewManager(configDir, sessionsFile)
		projects, err := mgr.GetAvailableProjectsSorted()
		if err != nil {
			// Check if the error is due to missing config file
			if os.IsNotExist(err) {
				// Interactive first-run setup
				return handleFirstRunSetup(configDir)
			}
			return fmt.Errorf("failed to get available projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found in search paths!")
			fmt.Println("\nMake sure your search paths are configured in one of:")
			fmt.Println("  ~/.config/tmux/project-search-paths.yaml")
			fmt.Println("  ~/.config/grove/project-search-paths.yaml")
			return nil
		}

		// Get search paths for display
		searchPaths, err := mgr.GetEnabledSearchPaths()
		if err != nil {
			// Don't fail if we can't get search paths, just continue without them
			searchPaths = []string{}
		}

		// Create the interactive model
		m := newSessionizeModel(projects, searchPaths, mgr, configDir)

		// Run the interactive program
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		// Check if a project was selected
		if sm, ok := finalModel.(sessionizeModel); ok && sm.selected.Path != "" {
			// Record the access before switching
			_ = mgr.RecordProjectAccess(sm.selected.Path)
			// If it's a worktree, also record access for the parent
			if sm.selected.IsWorktree && sm.selected.ParentPath != "" {
				_ = mgr.RecordProjectAccess(sm.selected.ParentPath)
			}
			return sessionizeProject(sm.selected.Path)
		}

		return nil
	},
}

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

// gitStatusEqual compares two git status objects for equality
func gitStatusEqual(a, b *git.StatusInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.HasUpstream == b.HasUpstream &&
		a.AheadCount == b.AheadCount &&
		a.BehindCount == b.BehindCount &&
		a.ModifiedCount == b.ModifiedCount &&
		a.StagedCount == b.StagedCount &&
		a.UntrackedCount == b.UntrackedCount &&
		a.IsDirty == b.IsDirty
}

// extendedGitStatusEqual compares two extended git status objects for equality
func extendedGitStatusEqual(a, b *extendedGitStatus) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return gitStatusEqual(a.StatusInfo, b.StatusInfo) &&
		a.LinesAdded == b.LinesAdded &&
		a.LinesDeleted == b.LinesDeleted
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
	keyMap   map[string]string // map[path]key
	sessions []models.TmuxSession // Also pass the full session list
}

// sessionizeModel is the model for the interactive project picker
type sessionizeModel struct {
	projects                 []manager.DiscoveredProject
	filtered                 []manager.DiscoveredProject
	selected                 manager.DiscoveredProject
	cursor                   int
	filterInput              textinput.Model
	searchPaths              []string
	manager                  *tmux.Manager
	configDir                string                        // configuration directory
	keyMap                   map[string]string             // path -> key mapping
	runningSessions          map[string]bool               // map[sessionName] -> true
	claudeStatusMap          map[string]string             // path -> claude session status mapping
	claudeDurationMap        map[string]string             // path -> claude session state duration mapping
	claudeDurationSecondsMap map[string]int                // path -> claude session state duration in seconds
	gitStatusMap             map[string]*extendedGitStatus // path -> extended git status
	hasGroveHooks            bool                          // whether grove-hooks is available
	currentSession           string                        // name of the current tmux session
	width                    int
	height                   int
	// Key editing mode
	editingKeys   bool
	keyCursor     int
	availableKeys []string
	sessions      []models.TmuxSession
}

func newSessionizeModel(projects []manager.DiscoveredProject, searchPaths []string, mgr *tmux.Manager, configDir string) sessionizeModel {
	// Create text input for filtering
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	// Build key mapping from sessions
	keyMap := make(map[string]string)
	sessions, err := mgr.GetSessions()
	if err != nil {
		sessions = []models.TmuxSession{}
	}

	for _, s := range sessions {
		if s.Path != "" {
			// Get absolute path for consistent matching
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				// Store with clean path
				cleanPath := filepath.Clean(absPath)
				keyMap[cleanPath] = s.Key
			}
		}
	}

	// Get available keys
	availableKeys := mgr.GetAvailableKeys()

	// Create running sessions map
	runningSessions := make(map[string]bool)
	// Will be populated via commands

	// Check if grove-hooks is available
	hasGroveHooks := false
	groveHooksPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "grove-hooks")
	if _, err := os.Stat(groveHooksPath); err == nil {
		hasGroveHooks = true
	} else if _, err := exec.LookPath("grove-hooks"); err == nil {
		hasGroveHooks = true
	}

	// Claude sessions will be fetched asynchronously
	claudeStatusMap := make(map[string]string)
	claudeDurationMap := make(map[string]string)
	claudeDurationSecondsMap := make(map[string]int)

	// Get current session name if we're in tmux
	currentSession := ""
	if os.Getenv("TMUX") != "" {
		client, err := tmuxclient.NewClient()
		if err == nil {
			ctx := context.Background()
			if current, err := client.GetCurrentSession(ctx); err == nil {
				currentSession = current
			}
		}
	}

	// Initialize empty git status map - will be populated asynchronously
	gitStatusMap := make(map[string]*extendedGitStatus)

	return sessionizeModel{
		projects:                 projects,
		filtered:                 projects,
		filterInput:              ti,
		searchPaths:              searchPaths,
		manager:                  mgr,
		configDir:                configDir,
		keyMap:                   keyMap,
		runningSessions:          runningSessions,
		claudeStatusMap:          claudeStatusMap,
		claudeDurationMap:        claudeDurationMap,
		claudeDurationSecondsMap: claudeDurationSecondsMap,
		gitStatusMap:             gitStatusMap,
		hasGroveHooks:            hasGroveHooks,
		currentSession:           currentSession,
		cursor:                   0,
		editingKeys:              false,
		keyCursor:                0,
		availableKeys:            availableKeys,
		sessions:                 sessions,
	}
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

func (m sessionizeModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		fetchGitStatusCmd(),
		fetchClaudeSessionsCmd(),
		fetchProjectsCmd(m.manager),
		fetchRunningSessionsCmd(),
		fetchKeyMapCmd(m.manager),
		tickCmd(), // Start the periodic refresh cycle
	)
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
							Name:                  filepath.Base(cleanPath),
							Path:                  cleanPath,
							ClaudeSessionID:       session.ID,
							ClaudeSessionPID:      session.PID,
							ClaudeSessionStatus:   session.Status,
							ClaudeSessionDuration: session.StateDuration,
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
func fetchProjectsCmd(mgr *tmux.Manager) tea.Cmd {
	return func() tea.Msg {
		projects, _ := mgr.GetAvailableProjectsSorted()
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


func (m sessionizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case gitStatusUpdateMsg:
		// Only update if there are actual changes to prevent flashing
		hasChanges := false
		
		// Check if any status has changed
		for path, newStatus := range msg.statuses {
			oldStatus, exists := m.gitStatusMap[path]
			if !exists || !extendedGitStatusEqual(oldStatus, newStatus) {
				hasChanges = true
				break
			}
		}
		
		// Also check if any status was removed
		if !hasChanges {
			for path := range m.gitStatusMap {
				if _, exists := msg.statuses[path]; !exists {
					hasChanges = true
					break
				}
			}
		}
		
		// Only update if there are changes
		if hasChanges {
			// Update git status map
			m.gitStatusMap = msg.statuses
			// Update projects with new git status
			for i := range m.projects {
				cleanPath := filepath.Clean(m.projects[i].Path)
				if extStatus, found := m.gitStatusMap[cleanPath]; found {
					m.projects[i].GitStatus = extStatus.StatusInfo
				} else {
					// Clear git status if no longer found
					m.projects[i].GitStatus = nil
				}
			}
			// Also update filtered projects
			for i := range m.filtered {
				cleanPath := filepath.Clean(m.filtered[i].Path)
				if extStatus, found := m.gitStatusMap[cleanPath]; found {
					m.filtered[i].GitStatus = extStatus.StatusInfo
				} else {
					// Clear git status if no longer found
					m.filtered[i].GitStatus = nil
				}
			}
		}
		return m, nil

	case claudeSessionUpdateMsg:
		// Create new maps
		newStatusMap := make(map[string]string)
		newDurationMap := make(map[string]string)
		newDurationSecondsMap := make(map[string]int)

		for _, session := range msg.sessions {
			cleanPath := filepath.Clean(session.Path)
			newStatusMap[cleanPath] = session.ClaudeSessionStatus
			newDurationMap[cleanPath] = session.ClaudeSessionDuration
			newDurationSecondsMap[cleanPath] = session.ClaudeSessionPID // Using PID field temporarily
		}

		// Check if there are any changes
		hasChanges := false
		
		// Check if sizes differ
		if len(newStatusMap) != len(m.claudeStatusMap) {
			hasChanges = true
		} else {
			// Check each entry
			for path, newStatus := range newStatusMap {
				oldStatus, exists := m.claudeStatusMap[path]
				oldDuration := m.claudeDurationMap[path]
				newDuration := newDurationMap[path]
				
				if !exists || oldStatus != newStatus || oldDuration != newDuration {
					hasChanges = true
					break
				}
			}
		}
		
		// Only update if there are changes
		if hasChanges {
			m.claudeStatusMap = newStatusMap
			m.claudeDurationMap = newDurationMap
			m.claudeDurationSecondsMap = newDurationSecondsMap
		}

		return m, nil

	case projectsUpdateMsg:
		// Save the current selected project path
		selectedPath := ""
		if m.cursor < len(m.filtered) {
			selectedPath = m.filtered[m.cursor].Path
		}

		// Update the main project list
		m.projects = msg.projects

		// Update the filtered list
		m.updateFiltered()

		// Try to restore cursor position
		if selectedPath != "" {
			for i, p := range m.filtered {
				if p.Path == selectedPath {
					m.cursor = i
					break
				}
			}
		}

		// Clamp cursor to valid range
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}

		return m, nil

	case runningSessionsUpdateMsg:
		// Replace the running sessions map
		m.runningSessions = msg.sessions
		// Re-apply filtering with updated session info
		m.updateFiltered()
		return m, nil

	case keyMapUpdateMsg:
		// Replace the key map and sessions
		m.keyMap = msg.keyMap
		m.sessions = msg.sessions
		return m, nil

	case tickMsg:
		// Refresh all data sources periodically
		return m, tea.Batch(
			fetchGitStatusCmd(),
			fetchClaudeSessionsCmd(),
			fetchProjectsCmd(m.manager),       // Add this
			fetchRunningSessionsCmd(),         // Add this
			fetchKeyMapCmd(m.manager),         // Add this
			tickCmd(),                         // This reschedules the tick
		)

	case tea.KeyMsg:
		// Handle key editing mode
		if m.editingKeys {
			switch msg.Type {
			case tea.KeyUp:
				if m.keyCursor > 0 {
					m.keyCursor--
				}
			case tea.KeyDown:
				if m.keyCursor < len(m.availableKeys)-1 {
					m.keyCursor++
				}
			case tea.KeyEnter:
				// Assign the selected key to the project
				if m.cursor < len(m.filtered) && m.keyCursor < len(m.availableKeys) {
					selectedProject := m.filtered[m.cursor]
					selectedKey := m.availableKeys[m.keyCursor]

					// Update the session
					m.updateKeyMapping(selectedProject.Path, selectedKey)

					// Refresh sessions to reflect changes
					if sessions, err := m.manager.GetSessions(); err == nil {
						m.sessions = sessions
					}
				}
				m.editingKeys = false
				return m, nil
			case tea.KeyEsc:
				m.editingKeys = false
				return m, nil
			default:
				// Check if the pressed key is a valid session key
				pressedKey := strings.ToLower(msg.String())
				for _, availableKey := range m.availableKeys {
					if strings.ToLower(availableKey) == pressedKey {
						// Found the key - assign it directly
						if m.cursor < len(m.filtered) {
							selectedProject := m.filtered[m.cursor]

							// Update the session
							m.updateKeyMapping(selectedProject.Path, availableKey)

							// Refresh sessions to reflect changes
							if sessions, err := m.manager.GetSessions(); err == nil {
								m.sessions = sessions
							}
						}
						m.editingKeys = false
						return m, nil
					}
				}
			}
			return m, nil
		}

		// Normal mode
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyCtrlN:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case tea.KeyCtrlE:
			// Enter key editing mode
			if m.cursor < len(m.filtered) {
				m.editingKeys = true
				m.keyCursor = 0
			}
		case tea.KeyCtrlX:
			// Clear key mapping for the selected project
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				m.clearKeyMapping(project.Path)
			}
		case tea.KeyCtrlY:
			// Yank (copy) the selected project path
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				// Use pbcopy on macOS, xclip on Linux
				var cmd *exec.Cmd
				if runtime.GOOS == "darwin" {
					cmd = exec.Command("pbcopy")
				} else {
					// Try xclip first, then xsel
					if _, err := exec.LookPath("xclip"); err == nil {
						cmd = exec.Command("xclip", "-selection", "clipboard")
					} else if _, err := exec.LookPath("xsel"); err == nil {
						cmd = exec.Command("xsel", "--clipboard", "--input")
					} else {
						// No clipboard utility found
						return m, nil
					}
				}

				if cmd != nil {
					cmd.Stdin = strings.NewReader(project.Path)
					_ = cmd.Run()
				}
			}
		case tea.KeyCtrlD:
			// Close the selected session
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")

				// Check if session exists before trying to close it
				client, err := tmuxclient.NewClient()
				if err == nil {
					ctx := context.Background()
					exists, err := client.SessionExists(ctx, sessionName)
					if err == nil && exists {
						// Check if we're in tmux and if this is the current session
						if os.Getenv("TMUX") != "" {
							currentSession, err := client.GetCurrentSession(ctx)
							if err == nil && currentSession == sessionName {
								// We're closing the current session - need to switch first
								// Get all sessions
								sessions, _ := client.ListSessions(ctx)

								// Find the best session to switch to
								var targetSession string

								// First, try to find the most recently accessed session from our list
								for _, p := range m.filtered {
									candidateName := filepath.Base(p.Path)
									candidateName = strings.ReplaceAll(candidateName, ".", "_")

									// Skip the current session
									if candidateName == sessionName {
										continue
									}

									// Check if this session exists
									for _, s := range sessions {
										if s == candidateName {
											targetSession = candidateName
											break
										}
									}

									if targetSession != "" {
										break
									}
								}

								// If no session from our list, just pick any other session
								if targetSession == "" {
									for _, s := range sessions {
										if s != sessionName {
											targetSession = s
											break
										}
									}
								}

								// Switch to the target session before killing current
								if targetSession != "" {
									_ = client.SwitchClient(ctx, targetSession)
								}
							}
						}

						// Kill the session
						if err := client.KillSession(ctx, sessionName); err == nil {
							// Clear the cached session status
							delete(m.runningSessions, sessionName)
						}
					}
				}
			}
		case tea.KeyEnter:
			if m.cursor < len(m.filtered) {
				m.selected = m.filtered[m.cursor]
				return m, tea.Quit
			}
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		default:
			// Update filter input
			prevValue := m.filterInput.Value()
			m.filterInput, cmd = m.filterInput.Update(msg)

			// If the filter changed, update filtered list
			if m.filterInput.Value() != prevValue {
				m.updateFiltered()
				m.cursor = 0
			}
			return m, cmd
		}
	}

	return m, nil
}

func (m *sessionizeModel) updateKeyMapping(projectPath, newKey string) {
	// Find if there's already a session with this key
	var existingSessionIndex = -1
	var targetSessionIndex = -1

	cleanPath := filepath.Clean(projectPath)

	// First, find any existing session with the new key
	for i, s := range m.sessions {
		if s.Key == newKey {
			existingSessionIndex = i
			break
		}
	}

	// Then find if this project already has a key mapping
	for i, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			if strings.EqualFold(filepath.Clean(absPath), cleanPath) {
				targetSessionIndex = i
				break
			}
		}
	}

	// Handle the key assignment
	if targetSessionIndex >= 0 {
		// Project already has a key mapping
		if existingSessionIndex >= 0 && existingSessionIndex != targetSessionIndex {
			// The new key is already in use by another session
			// Clear the old mapping (let go of it)
			m.sessions[existingSessionIndex].Path = ""
			m.sessions[existingSessionIndex].Repository = ""
		}
		// Update the key
		m.sessions[targetSessionIndex].Key = newKey
	} else {
		// Project doesn't have a key mapping yet
		if existingSessionIndex >= 0 {
			// The key is already in use, update that session with the new project
			m.sessions[existingSessionIndex].Path = projectPath
			m.sessions[existingSessionIndex].Repository = filepath.Base(projectPath)
		} else {
			// Key is not in use, create a new session
			newSession := models.TmuxSession{
				Key:        newKey,
				Path:       projectPath,
				Repository: filepath.Base(projectPath),
			}
			m.sessions = append(m.sessions, newSession)
		}
	}

	// Save the updated sessions
	_ = m.manager.UpdateSessions(m.sessions)
	_ = m.manager.RegenerateBindings()

	// Update our key map to reflect all changes
	m.keyMap = make(map[string]string)
	for _, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				cleanPath := filepath.Clean(absPath)
				m.keyMap[cleanPath] = s.Key
			}
		}
	}

	// Reload tmux config
	_ = reloadTmuxConfig()
}

func (m *sessionizeModel) clearKeyMapping(projectPath string) {
	cleanPath := filepath.Clean(projectPath)

	// Find if this project has a key mapping
	var targetSessionIndex = -1
	for i, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			if strings.EqualFold(filepath.Clean(absPath), cleanPath) {
				targetSessionIndex = i
				break
			}
		}
	}

	if targetSessionIndex >= 0 {
		// Clear the path and repository, but keep the key slot
		m.sessions[targetSessionIndex].Path = ""
		m.sessions[targetSessionIndex].Repository = ""

		// Save the updated sessions
		_ = m.manager.UpdateSessions(m.sessions)
		_ = m.manager.RegenerateBindings()

		// Update our key map
		delete(m.keyMap, cleanPath)

		// Refresh sessions to reflect changes
		if sessions, err := m.manager.GetSessions(); err == nil {
			m.sessions = sessions
		}

		// Reload tmux config
		_ = reloadTmuxConfig()
	}
}



func (m *sessionizeModel) updateFiltered() {
	filter := strings.ToLower(m.filterInput.Value())
	
	// A group is identified by the parent repo's path.
	// For a parent repo, its own path is the key. For a worktree, its ParentPath is the key.
	activeGroups := make(map[string]bool)
	for _, p := range m.projects {
		groupKey := p.Path
		if p.IsWorktree {
			groupKey = p.ParentPath
		}
		if groupKey == "" { // Should not happen, but as a safeguard
			continue
		}

		sessionName := filepath.Base(p.Path)
		sessionName = strings.ReplaceAll(sessionName, ".", "_")
		if m.runningSessions[sessionName] {
			activeGroups[groupKey] = true
		}
	}

	if filter == "" {
		// Default View: Group-aware sorting with inactive worktree filtering
		
		// Create a mutable copy for sorting
		sortedProjects := make([]manager.DiscoveredProject, len(m.projects))
		copy(sortedProjects, m.projects)

		sort.SliceStable(sortedProjects, func(i, j int) bool {
			groupI := sortedProjects[i].Path
			if sortedProjects[i].IsWorktree {
				groupI = sortedProjects[i].ParentPath
			}
			isGroupIActive := activeGroups[groupI]

			groupJ := sortedProjects[j].Path
			if sortedProjects[j].IsWorktree {
				groupJ = sortedProjects[j].ParentPath
			}
			isGroupJActive := activeGroups[groupJ]

			if isGroupIActive && !isGroupJActive {
				return true
			}
			if !isGroupIActive && isGroupJActive {
				return false
			}
			return false // Maintain original order for groups of same activity status
		})

		// Filter inactive worktrees: only include worktrees with running sessions
		m.filtered = []manager.DiscoveredProject{}
		for _, p := range sortedProjects {
			if !p.IsWorktree {
				// Always include parent repositories
				m.filtered = append(m.filtered, p)
			} else {
				// Only include worktrees with active sessions
				sessionName := filepath.Base(p.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")
				if m.runningSessions[sessionName] {
					m.filtered = append(m.filtered, p)
				}
			}
		}
	} else {
		// Filtered View: Show all matching projects, grouped by activity
		
		// sortByMatchQuality sorts projects by match quality (exact, prefix, contains, path)
		sortByMatchQuality := func(projects []manager.DiscoveredProject, filter string) []manager.DiscoveredProject {
			// Separate matches into priority groups
			var exactNameMatches []manager.DiscoveredProject    // Exact match on project name
			var prefixNameMatches []manager.DiscoveredProject   // Prefix match on project name
			var containsNameMatches []manager.DiscoveredProject // Contains in project name
			var pathMatches []manager.DiscoveredProject         // Matches elsewhere in the path

			for _, p := range projects {
				lowerName := strings.ToLower(p.Name)
				lowerPath := strings.ToLower(p.Path)

				if lowerName == filter {
					// Exact match on name - highest priority
					exactNameMatches = append(exactNameMatches, p)
				} else if strings.HasPrefix(lowerName, filter) {
					// Prefix match on name - high priority
					prefixNameMatches = append(prefixNameMatches, p)
				} else if strings.Contains(lowerName, filter) {
					// Contains in name - medium priority
					containsNameMatches = append(containsNameMatches, p)
				} else if strings.Contains(lowerPath, filter) {
					// Match elsewhere in the path - lower priority
					pathMatches = append(pathMatches, p)
				}
			}

			// Combine results in priority order
			var result []manager.DiscoveredProject
			result = append(result, exactNameMatches...)
			result = append(result, prefixNameMatches...)
			result = append(result, containsNameMatches...)
			result = append(result, pathMatches...)
			return result
		}
		
		// Partition matches by group activity
		var activeGroupMatches []manager.DiscoveredProject
		var inactiveGroupMatches []manager.DiscoveredProject

		for _, p := range m.projects {
			lowerName := strings.ToLower(p.Name)
			lowerPath := strings.ToLower(p.Path)

			// Check if this project matches the filter
			if lowerName == filter || strings.HasPrefix(lowerName, filter) || 
			   strings.Contains(lowerName, filter) || strings.Contains(lowerPath, filter) {
				
				// Determine group key
				groupKey := p.Path
				if p.IsWorktree {
					groupKey = p.ParentPath
				}
				
				// Check group activity
				if activeGroups[groupKey] {
					activeGroupMatches = append(activeGroupMatches, p)
				} else {
					inactiveGroupMatches = append(inactiveGroupMatches, p)
				}
			}
		}

		// Sort both groups by match quality
		activeGroupMatches = sortByMatchQuality(activeGroupMatches, filter)
		inactiveGroupMatches = sortByMatchQuality(inactiveGroupMatches, filter)

		// Combine: active groups first, then inactive groups
		m.filtered = []manager.DiscoveredProject{}
		m.filtered = append(m.filtered, activeGroupMatches...)
		m.filtered = append(m.filtered, inactiveGroupMatches...)
	}
}

// formatChanges formats the git status into a styled string.
func formatChanges(status *git.StatusInfo, extStatus *extendedGitStatus) string {
	if status == nil {
		return ""
	}

	var changes []string

	if status.HasUpstream {
		if status.AheadCount > 0 {
			changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#95e1d3")).Bold(true).Render(fmt.Sprintf("↑%d", status.AheadCount)))
		}
		if status.BehindCount > 0 {
			changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#f38181")).Bold(true).Render(fmt.Sprintf("↓%d", status.BehindCount)))
		}
	}

	if status.ModifiedCount > 0 {
		changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Bold(true).Render(fmt.Sprintf("M:%d", status.ModifiedCount)))
	}
	if status.StagedCount > 0 {
		changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4")).Bold(true).Render(fmt.Sprintf("S:%d", status.StagedCount)))
	}
	if status.UntrackedCount > 0 {
		changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Bold(true).Render(fmt.Sprintf("?:%d", status.UntrackedCount)))
	}

	// Add lines added/deleted if available
	if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
		if extStatus.LinesAdded > 0 {
			changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Render(fmt.Sprintf("+%d", extStatus.LinesAdded)))
		}
		if extStatus.LinesDeleted > 0 {
			changes = append(changes, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff4444")).Render(fmt.Sprintf("-%d", extStatus.LinesDeleted)))
		}
	}

	changesStr := strings.Join(changes, " ")
	if !status.IsDirty && changesStr == "" && status.HasUpstream {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Render("✓")
	}

	return changesStr
}

func (m sessionizeModel) View() string {
	var b strings.Builder

	// Show key editing mode if active
	if m.editingKeys {
		return m.viewKeyEditor()
	}

	// Header with filter input (always at top)
	b.WriteString(m.filterInput.View())
	b.WriteString("\n\n")

	// Calculate visible items based on terminal height
	// Reserve space for: header (3 lines), help (1 line), search paths (2 lines)
	visibleHeight := m.height - 6
	if visibleHeight < 5 {
		visibleHeight = 5 // Minimum visible items
	}

	// Determine visible range with scrolling
	start := 0
	end := len(m.filtered)

	// Implement scrolling if there are too many items
	if end > visibleHeight {
		// Center the cursor in the visible area when possible
		if m.cursor < visibleHeight/2 {
			// Near the top
			start = 0
		} else if m.cursor >= len(m.filtered)-visibleHeight/2 {
			// Near the bottom
			start = len(m.filtered) - visibleHeight
		} else {
			// Middle - center the cursor
			start = m.cursor - visibleHeight/2
		}

		end = start + visibleHeight
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		if start < 0 {
			start = 0
		}
	}

	// Render visible projects
	for i := start; i < end && i < len(m.filtered); i++ {
		project := m.filtered[i]

		// Check if this project has a key mapping
		keyMapping := ""
		cleanPath := filepath.Clean(project.Path)
		if key, hasKey := m.keyMap[cleanPath]; hasKey {
			keyMapping = key
		} else {
			// Try case-insensitive match on macOS
			for path, key := range m.keyMap {
				if strings.EqualFold(path, cleanPath) {
					keyMapping = key
					break
				}
			}
		}

		// Check if session exists for this project
		sessionName := filepath.Base(project.Path)
		sessionName = strings.ReplaceAll(sessionName, ".", "_")
		sessionExists := m.runningSessions[sessionName]

		// Get Claude session status
		var claudeStatusStyled string
		var claudeDuration string

		// Check if this is a Claude session project
		if project.ClaudeSessionID != "" {
			// This is a Claude session entry - use its own status
			statusSymbol := ""
			statusColor := lipgloss.Color("#808080")
			switch project.ClaudeSessionStatus {
			case "running":
				statusSymbol = "▶"
				statusColor = lipgloss.Color("#00ff00")
			case "idle":
				statusSymbol = "⏸"
				statusColor = lipgloss.Color("#ffaa00")
			case "completed":
				statusSymbol = "✓"
				statusColor = lipgloss.Color("#4ecdc4")
			case "failed", "error":
				statusSymbol = "✗"
				statusColor = lipgloss.Color("#ff4444")
			}

			claudeStatusStyled = lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			claudeDuration = project.ClaudeSessionDuration
		} else if m.hasGroveHooks {
			// Regular project - check if it has any Claude sessions
			claudeStatus := ""
			if status, found := m.claudeStatusMap[cleanPath]; found {
				claudeStatus = status
				if duration, foundDur := m.claudeDurationMap[cleanPath]; foundDur {
					claudeDuration = duration
				}
			} else {
				// Try case-insensitive match on macOS
				for path, status := range m.claudeStatusMap {
					if strings.EqualFold(path, cleanPath) {
						claudeStatus = status
						if duration, foundDur := m.claudeDurationMap[path]; foundDur {
							claudeDuration = duration
						}
						break
					}
				}
			}

			// Style the claude status (without duration - that goes at the end)
			statusSymbol := ""
			statusColor := lipgloss.Color("#808080")
			switch claudeStatus {
			case "running":
				statusSymbol = "▶"
				statusColor = lipgloss.Color("#00ff00")
			case "idle":
				statusSymbol = "⏸"
				statusColor = lipgloss.Color("#ffaa00")
			case "completed":
				statusSymbol = "✓"
				statusColor = lipgloss.Color("#4ecdc4")
			case "failed", "error":
				statusSymbol = "✗"
				statusColor = lipgloss.Color("#ff4444")
			}

			if statusSymbol != "" {
				claudeStatusStyled = lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			} else {
				claudeStatusStyled = " " // Empty space to maintain alignment
			}
		}

		// Get Git status string
		var extStatus *extendedGitStatus
		if es, found := m.gitStatusMap[cleanPath]; found {
			extStatus = es
		}
		changesStr := formatChanges(project.GitStatus, extStatus)

		// Prepare display elements
		prefix := ""
		displayName := project.Name
		if project.IsWorktree {
			prefix = "└─ "
		}

		// If this is a Claude session, add PID to the name
		if project.ClaudeSessionID != "" {
			displayName = fmt.Sprintf("%s [PID:%d]", project.Name, project.ClaudeSessionPID)
		}

		if i == m.cursor {
			// Highlight selected line
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true).
				Render("▶ ")

			nameStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true)
			pathStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#95e1d3"))

			keyIndicator := "  " // Default: 2 spaces
			if keyMapping != "" {
				keyIndicator = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffaa00")).
					Bold(true).
					Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")

				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00aaff")).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00ff00")).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("%s%s%s", indicator, keyIndicator, sessionIndicator)
			if m.hasGroveHooks {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)
			line += "  " + pathStyle.Render(project.Path)

			// Add git status if session exists
			if sessionExists && changesStr != "" {
				line += "  " + changesStr
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && claudeDuration != "" {
				line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#b19cd9")).Render(claudeDuration)
			}

			b.WriteString(line)
		} else {
			// Normal line with colored name
			nameStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4ecdc4")).
				Bold(true)
			pathStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#808080"))

			// Always reserve space for key indicator
			keyIndicator := "  "
			if keyMapping != "" {
				keyIndicator = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffaa00")).
					Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")

				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00aaff")).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00ff00")).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("  %s%s", keyIndicator, sessionIndicator)
			if m.hasGroveHooks {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)
			line += "  " + pathStyle.Render(project.Path)

			// Add git status if session exists
			if sessionExists && changesStr != "" {
				line += "  " + changesStr
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && claudeDuration != "" {
				line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#b19cd9")).Render(claudeDuration)
			}

			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Show scroll indicators if needed
	if start > 0 || end < len(m.filtered) {
		scrollInfo := fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.filtered))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render(scrollInfo))
	}

	// Help text at bottom
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("No active Claude sessions"))
		} else {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("No matching Claude sessions"))
		}
	}

	// Build help text with highlighted keys
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#95a99c"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#b8d4ce")).Bold(true)

	b.WriteString("\n")
	b.WriteString(keyStyle.Render("↑/↓") + helpStyle.Render(": navigate • "))
	b.WriteString(keyStyle.Render("enter") + helpStyle.Render(": select • "))
	b.WriteString(keyStyle.Render("ctrl+e") + helpStyle.Render(": edit key • "))
	b.WriteString(keyStyle.Render("ctrl+x") + helpStyle.Render(": clear key • "))
	b.WriteString(keyStyle.Render("ctrl+y") + helpStyle.Render(": copy path • "))
	b.WriteString(keyStyle.Render("ctrl+d") + helpStyle.Render(": close • "))
	b.WriteString(keyStyle.Render("esc") + helpStyle.Render(": quit"))

	// Display search paths at the very bottom
	if len(m.searchPaths) > 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("Search paths: "))
		// Truncate search paths if too long
		pathsDisplay := strings.Join(m.searchPaths, " • ")
		if len(pathsDisplay) > m.width-15 && m.width > 50 {
			pathsDisplay = pathsDisplay[:m.width-18] + "..."
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render(pathsDisplay))
	}

	return b.String()
}

func (m sessionizeModel) viewKeyEditor() string {
	var b strings.Builder

	// Header
	selectedProject := ""
	selectedPath := ""
	if m.cursor < len(m.filtered) {
		project := m.filtered[m.cursor]
		selectedPath = project.Path
		selectedProject = project.Name
	}

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff00")).Bold(true).Render(fmt.Sprintf("Select key for: %s", selectedProject)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render(selectedPath))
	b.WriteString("\n\n")

	// Build a sorted list of all sessions for display
	type keyDisplay struct {
		key        string
		repository string
		path       string
		isCurrent  bool
	}

	var displays []keyDisplay
	currentKey := ""

	// Find current key for the selected project
	cleanSelectedPath := filepath.Clean(selectedPath)
	for _, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			if strings.EqualFold(filepath.Clean(absPath), cleanSelectedPath) {
				currentKey = s.Key
				break
			}
		}
	}

	// Build display list
	for _, key := range m.availableKeys {
		display := keyDisplay{
			key:       key,
			isCurrent: key == currentKey,
		}

		// Find if this key is mapped
		for _, s := range m.sessions {
			if s.Key == key {
				if s.Path != "" {
					display.repository = filepath.Base(s.Path)
					display.path = s.Path
				}
				break
			}
		}

		displays = append(displays, display)
	}

	// Calculate visible range
	visibleHeight := m.height - 8 // Account for header and help
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := 0
	end := len(displays)

	if end > visibleHeight {
		// Center the cursor in the visible area
		if m.keyCursor < visibleHeight/2 {
			start = 0
		} else if m.keyCursor >= len(displays)-visibleHeight/2 {
			start = len(displays) - visibleHeight
		} else {
			start = m.keyCursor - visibleHeight/2
		}

		end = start + visibleHeight
		if end > len(displays) {
			end = len(displays)
		}
		if start < 0 {
			start = 0
		}
	}

	// Render the table
	for i := start; i < end; i++ {
		d := displays[i]

		// Selection indicator
		if i == m.keyCursor {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true).
				Render("▶ "))
		} else {
			b.WriteString("  ")
		}

		// Key
		keyStyle := lipgloss.NewStyle().Bold(true)
		if d.isCurrent {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#ffaa00"))
		} else if d.repository != "" {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#808080"))
		} else {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#00ff00"))
		}
		b.WriteString(keyStyle.Render(fmt.Sprintf("%s ", d.key)))

		// Repository and path
		if d.repository != "" {
			repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4"))
			pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))

			b.WriteString(repoStyle.Render(fmt.Sprintf("%-20s", d.repository)))
			b.WriteString(" ")
			b.WriteString(pathStyle.Render(d.path))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render("(available)"))
		}

		// Mark current
		if d.isCurrent {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffaa00")).
				Render(" ← current"))
		}

		b.WriteString("\n")
	}

	// Scroll indicator
	if start > 0 || end < len(displays) {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")).Render(fmt.Sprintf("\n(%d-%d of %d)", start+1, end, len(displays))))
	}

	// Build help text with highlighted keys
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#95a99c"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#b8d4ce")).Bold(true)

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("press ") + keyStyle.Render("key directly") + helpStyle.Render(" or "))
	b.WriteString(keyStyle.Render("↑/↓") + helpStyle.Render(" + ") + keyStyle.Render("enter") + helpStyle.Render(" to assign • "))
	b.WriteString(keyStyle.Render("esc") + helpStyle.Render(": cancel"))

	return b.String()
}

// sessionizeProject creates or switches to a tmux session for the given project path
func sessionizeProject(projectPath string) error {
	// Expand path if needed
	if strings.HasPrefix(projectPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			projectPath = filepath.Join(home, projectPath[2:])
		}
	}

	// Get absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if directory exists
	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		return fmt.Errorf("directory does not exist: %s", absPath)
	}

	// Create session name from path
	sessionName := filepath.Base(absPath)
	sessionName = strings.ReplaceAll(sessionName, ".", "_")

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

	return nil
}

// handleFirstRunSetup creates an interactive setup flow for first-time users
func handleFirstRunSetup(configDir string) error {
	// Welcome message
	fmt.Println("Welcome to gmux sessionizer!")
	fmt.Println("It looks like this is your first time running the sessionizer.")
	fmt.Println("Let's set up your project directories.")
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
		fmt.Println("\nNo directories added. To set up manually, create a file at:")
		fmt.Printf("  %s/project-search-paths.yaml\n", configDir)
		fmt.Println("\nExample configuration:")
		fmt.Println(getDefaultConfigContent())
		return nil
	}
	
	// Create the config directory if needed
	expandedConfigDir := expandPath(configDir)
	if err := os.MkdirAll(expandedConfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	// Generate the config content with user's directories
	content := generateConfigWithPaths(searchPaths)
	
	// Create the config file
	configPath := filepath.Join(expandedConfigDir, "project-search-paths.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	fmt.Printf("\n✅ Configuration file created at: %s\n", configPath)
	fmt.Printf("✅ Added %d project director%s\n", len(searchPaths), 
		map[bool]string{true: "ies", false: "y"}[len(searchPaths) != 1])
	
	fmt.Println("\n✅ Setup complete! Run 'gmux sz' to start using the sessionizer.")
	return nil
}

// generateConfigWithPaths creates a configuration file with the user's specified paths
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
