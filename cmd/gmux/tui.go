package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/tui/components/help"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
)

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
	help          help.Model

	// Focus mode state
	ecosystemPickerMode bool                       // True when showing only ecosystems for selection
	focusedProject      *manager.DiscoveredProject
	worktreesFolded     bool // Whether worktrees are hidden/collapsed

	// View toggles
	showGitStatus      bool // Whether to fetch and show Git status
	showClaudeSessions bool // Whether to fetch and show Claude sessions
	pathDisplayMode    int  // 0=no paths, 1=compact (~), 2=full paths
}
func newSessionizeModel(projects []manager.DiscoveredProject, searchPaths []string, mgr *tmux.Manager, configDir string) sessionizeModel {
	// Create text input for filtering (start unfocused)
	ti := textinput.New()
	ti.Placeholder = "Press / to filter..."
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

	helpModel := help.NewBuilder().
		WithKeys(sessionizeKeys).
		WithTitle("Project Sessionizer - Help").
		Build()

	// Load previously focused ecosystem and fold state
	var focusedProject *manager.DiscoveredProject
	var worktreesFolded bool
	// Set sensible defaults for toggles
	showGitStatus := true
	showClaudeSessions := true
	pathDisplayMode := 1 // Default to compact paths (~)
	if state, err := manager.LoadState(configDir); err == nil {
		if state.FocusedEcosystemPath != "" {
			// Find the project with this path
			for i := range projects {
				if projects[i].Path == state.FocusedEcosystemPath {
					focusedProject = &projects[i]
					break
				}
			}
		}
		worktreesFolded = state.WorktreesFolded
		// Override defaults with saved state if present
		if state.ShowGitStatus != nil {
			showGitStatus = *state.ShowGitStatus
		}
		if state.ShowClaudeSessions != nil {
			showClaudeSessions = *state.ShowClaudeSessions
		}
		if state.PathDisplayMode != nil {
			pathDisplayMode = *state.PathDisplayMode
		}
	}

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
		help:                     helpModel,
		focusedProject:           focusedProject,
		worktreesFolded:          worktreesFolded,
		showGitStatus:            showGitStatus,
		showClaudeSessions:       showClaudeSessions,
		pathDisplayMode:          pathDisplayMode,
	}
}

// buildState creates a SessionizerState from the current model
func (m sessionizeModel) buildState() *manager.SessionizerState {
	state := &manager.SessionizerState{
		FocusedEcosystemPath: "",
		WorktreesFolded:      m.worktreesFolded,
		ShowGitStatus:        boolPtr(m.showGitStatus),
		ShowClaudeSessions:   boolPtr(m.showClaudeSessions),
		PathDisplayMode:      intPtr(m.pathDisplayMode),
	}
	if m.focusedProject != nil {
		state.FocusedEcosystemPath = m.focusedProject.Path
	}
	return state
}

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// intPtr returns a pointer to an int value
func intPtr(i int) *int {
	return &i
}
func (m sessionizeModel) Init() tea.Cmd {
	return tea.Batch(
		fetchGitStatusCmd(),
		fetchClaudeSessionsCmd(),
		fetchProjectsCmd(m.manager, m.showGitStatus, m.showClaudeSessions),
		fetchRunningSessionsCmd(),
		fetchKeyMapCmd(m.manager),
		tickCmd(), // Start the periodic refresh cycle
	)
}
func (m sessionizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
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
			if session.ClaudeSession != nil {
				newStatusMap[cleanPath] = session.ClaudeSession.Status
				newDurationMap[cleanPath] = session.ClaudeSession.Duration
				newDurationSecondsMap[cleanPath] = session.ClaudeSession.PID
			}
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
			fetchProjectsCmd(m.manager, m.showGitStatus, m.showClaudeSessions),
			fetchRunningSessionsCmd(),
			fetchKeyMapCmd(m.manager),
			tickCmd(), // This reschedules the tick
		)

	case tea.KeyMsg:
		// If help is visible, it consumes all key presses
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		// Handle main key bindings before specific modes
		switch {
		case key.Matches(msg, sessionizeKeys.FocusEcosystem):
			// Enter ecosystem picker mode
			m.ecosystemPickerMode = true
			m.updateFiltered() // Will filter to ecosystems only
			m.cursor = 0
			return m, nil

		case key.Matches(msg, sessionizeKeys.ToggleWorktrees):
			m.worktreesFolded = !m.worktreesFolded
			m.updateFiltered()

			// Save the fold state
			_ = m.buildState().Save(m.configDir)
			return m, nil

		case key.Matches(msg, sessionizeKeys.ToggleGitStatus):
			m.showGitStatus = !m.showGitStatus
			// Save the toggle state
			_ = m.buildState().Save(m.configDir)
			// Refresh projects with new toggle state
			return m, fetchProjectsCmd(m.manager, m.showGitStatus, m.showClaudeSessions)

		case key.Matches(msg, sessionizeKeys.ToggleClaude):
			m.showClaudeSessions = !m.showClaudeSessions
			// Save the toggle state
			_ = m.buildState().Save(m.configDir)
			// Refresh projects with new toggle state
			return m, fetchProjectsCmd(m.manager, m.showGitStatus, m.showClaudeSessions)

		case key.Matches(msg, sessionizeKeys.TogglePaths):
			// Cycle through: compact (~) -> full -> no paths -> compact
			m.pathDisplayMode = (m.pathDisplayMode + 1) % 3
			// Save the toggle state (no need to refresh projects for path display)
			_ = m.buildState().Save(m.configDir)
			return m, nil

		case key.Matches(msg, sessionizeKeys.ClearFocus):
			if m.focusedProject != nil {
				m.focusedProject = nil
				m.updateFiltered()
				m.cursor = 0

				// Clear the focused ecosystem from state
				_ = m.buildState().Save(m.configDir)
			}
			return m, nil
		}

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

		// Check if filter input is focused and handle special keys
		if m.filterInput.Focused() {
			switch msg.Type {
			case tea.KeyEsc:
				if m.ecosystemPickerMode {
					m.ecosystemPickerMode = false
					m.updateFiltered()
					return m, nil
				}
				m.filterInput.Blur()
				return m, nil
			case tea.KeyEnter:
				// Handle ecosystem picker mode
				if m.ecosystemPickerMode {
					if m.cursor < len(m.filtered) {
						// Make a copy to avoid pointer issues
						selected := m.filtered[m.cursor]
						m.focusedProject = &selected
						m.ecosystemPickerMode = false
						m.updateFiltered() // Now filter to focused ecosystem
						m.cursor = 0

						// Save state
						fmt.Fprintf(os.Stderr, "DEBUG: Saving state to %s/gmux/state.yml, focused path: %s\n", m.configDir, m.focusedProject.Path)
						if err := m.buildState().Save(m.configDir); err != nil {
							// Log error but don't fail the operation
							fmt.Fprintf(os.Stderr, "ERROR: failed to save state: %v\n", err)
						} else {
							fmt.Fprintf(os.Stderr, "DEBUG: State saved successfully\n")
						}
					}
					return m, nil
				}
				// Select current project even while filtering
				if m.cursor < len(m.filtered) {
					m.selected = m.filtered[m.cursor]
					return m, tea.Quit
				}
				return m, nil
			case tea.KeyUp:
				// Navigate up while filtering
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case tea.KeyDown:
				// Navigate down while filtering
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, nil
			default:
				// Let filter input handle all other keys when focused
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

		// Normal mode (when filter is not focused)
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyCtrlN:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case tea.KeyCtrlU:
			// Page up (vim-style)
			pageSize := 10
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
		case tea.KeyCtrlD:
			// Page down (vim-style)
			pageSize := 10
			m.cursor += pageSize
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
		case tea.KeyRunes:
			switch msg.String() {
			case "j":
				// Vim-style down navigation
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, nil
			case "k":
				// Vim-style up navigation
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case "g":
				// Handle gg (go to top) - need to check for double g
				// For simplicity, single g goes to top (common in many TUIs)
				m.cursor = 0
				return m, nil
			case "G":
				// Go to bottom
				m.cursor = len(m.filtered) - 1
				if m.cursor < 0 {
					m.cursor = 0
				}
				return m, nil
			case "X":
				// Close session (moved from ctrl+d)
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
				return m, nil
			case "?":
				m.help.Toggle()
				return m, nil
			case "/":
				// Focus filter input for search
				m.filterInput.Focus()
				return m, textinput.Blink
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
		case tea.KeyEnter:
			// Handle ecosystem picker mode
			if m.ecosystemPickerMode {
				if m.cursor < len(m.filtered) {
					// Make a copy to avoid pointer issues
					selected := m.filtered[m.cursor]
					m.focusedProject = &selected
					m.ecosystemPickerMode = false
					m.updateFiltered() // Now filter to focused ecosystem
					m.cursor = 0

					// Save state
					fmt.Fprintf(os.Stderr, "DEBUG: Saving state to %s/gmux/state.yml, focused path: %s\n", m.configDir, m.focusedProject.Path)
					if err := m.buildState().Save(m.configDir); err != nil {
						fmt.Fprintf(os.Stderr, "ERROR: failed to save state: %v\n", err)
					} else {
						fmt.Fprintf(os.Stderr, "DEBUG: State saved successfully\n")
					}
				}
				return m, nil
			}
			// Normal mode - select project and quit
			if m.cursor < len(m.filtered) {
				m.selected = m.filtered[m.cursor]
				return m, tea.Quit
			}
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		default:
			// Handle other keys normally
			return m, nil
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

	// Handle ecosystem picker mode - show ecosystems with their worktrees in a tree
	if m.ecosystemPickerMode {
		m.filtered = []manager.DiscoveredProject{}

		// Separate into main ecosystems and worktrees
		mainEcosystemsMap := make(map[string]manager.DiscoveredProject)
		worktreesByParent := make(map[string][]manager.DiscoveredProject)

		for _, p := range m.projects {
			if !p.IsEcosystem {
				continue
			}

			// Apply filter
			matchesFilter := filter == "" ||
				strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Path), filter)

			if !matchesFilter {
				continue
			}

			if p.IsWorktree && p.ParentPath != "" {
				// This is a worktree - group by parent
				worktreesByParent[p.ParentPath] = append(worktreesByParent[p.ParentPath], p)
			} else {
				// This is a main ecosystem
				mainEcosystemsMap[p.Path] = p
			}
		}

		// Convert map to slice and sort
		var mainEcosystems []manager.DiscoveredProject
		for _, eco := range mainEcosystemsMap {
			mainEcosystems = append(mainEcosystems, eco)
		}
		sort.Slice(mainEcosystems, func(i, j int) bool {
			return strings.ToLower(mainEcosystems[i].Name) < strings.ToLower(mainEcosystems[j].Name)
		})

		// Build filtered list: main ecosystem followed by its worktrees
		for _, eco := range mainEcosystems {
			m.filtered = append(m.filtered, eco)

			if worktrees, hasWorktrees := worktreesByParent[eco.Path]; hasWorktrees {
				// Sort worktrees alphabetically
				sort.Slice(worktrees, func(i, j int) bool {
					return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
				})
				m.filtered = append(m.filtered, worktrees...)
			}
		}
		return
	}

	// Create a working list of projects, either all projects or just the focused ecosystem
	var projectsToFilter []manager.DiscoveredProject
	if m.focusedProject != nil && m.focusedProject.IsEcosystem {
		// Focus is active on an ecosystem - show it and all its children
		// 1. The focused ecosystem itself
		projectsToFilter = append(projectsToFilter, *m.focusedProject)

		// 2. All children of the focused ecosystem (using ParentEcosystemPath)
		for _, p := range m.projects {
			if p.ParentEcosystemPath == m.focusedProject.Path {
				projectsToFilter = append(projectsToFilter, p)
			}
		}
	} else if m.focusedProject != nil {
		// Focused on something that is not an ecosystem (a regular project)
		// Just show that project
		projectsToFilter = append(projectsToFilter, *m.focusedProject)
	} else {
		// No focus, use all projects
		projectsToFilter = m.projects
	}

	// A group is identified by the parent repo's path.
	// For a parent repo, its own path is the key. For a worktree, its ParentPath is the key.
	activeGroups := make(map[string]bool)
	for _, p := range projectsToFilter {
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

		if m.focusedProject != nil {
			// Focus mode: Group repos with their worktrees hierarchically
			m.filtered = []manager.DiscoveredProject{}

			// First add the focused project
			m.filtered = append(m.filtered, *m.focusedProject)

			// Build a map of parents to their worktrees
			parentWorktrees := make(map[string][]manager.DiscoveredProject)
			nonWorktrees := []manager.DiscoveredProject{}

			for _, p := range projectsToFilter {
				if p.Path == m.focusedProject.Path {
					continue // Skip focused project, already added
				}
				if p.IsWorktree {
					parentWorktrees[p.ParentPath] = append(parentWorktrees[p.ParentPath], p)
				} else {
					nonWorktrees = append(nonWorktrees, p)
				}
			}

			// Add non-worktree repos, each followed by their worktrees (if not folded)
			for _, parent := range nonWorktrees {
				m.filtered = append(m.filtered, parent)
				if !m.worktreesFolded {
					if worktrees, exists := parentWorktrees[parent.Path]; exists {
						m.filtered = append(m.filtered, worktrees...)
					}
				}
			}

			// Add any remaining ecosystem worktrees (direct children of focused project) if not folded
			if !m.worktreesFolded {
				if focusedWorktrees, exists := parentWorktrees[m.focusedProject.Path]; exists {
					// Insert these after the focused project (at position 1)
					m.filtered = append(m.filtered[:1], append(focusedWorktrees, m.filtered[1:]...)...)
				}
			}
		} else {
			// Normal mode: Original sorting logic
			// Create a mutable copy for sorting
			sortedProjects := make([]manager.DiscoveredProject, len(projectsToFilter))
			copy(sortedProjects, projectsToFilter)

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
		}
	} else {
		// Filtered View: Show all matching projects, grouped by activity
		
		// sortByMatchQuality sorts projects by match quality while preserving parent-child grouping
		sortByMatchQuality := func(projects []manager.DiscoveredProject, filter string) []manager.DiscoveredProject {
			// Build a map of parents to their worktrees
			parentWorktrees := make(map[string][]manager.DiscoveredProject)
			parents := []manager.DiscoveredProject{}

			for _, p := range projects {
				if p.IsWorktree {
					parentWorktrees[p.ParentPath] = append(parentWorktrees[p.ParentPath], p)
				} else {
					parents = append(parents, p)
				}
			}

			// Calculate match quality for sorting (name only, not path)
			getMatchQuality := func(p manager.DiscoveredProject) int {
				lowerName := strings.ToLower(p.Name)

				if lowerName == filter {
					return 3 // Exact match
				} else if strings.HasPrefix(lowerName, filter) {
					return 2 // Prefix match
				} else if strings.Contains(lowerName, filter) {
					return 1 // Contains in name
				}
				return 0 // No direct match (included because child matched)
			}

			// Sort parents by match quality
			sort.SliceStable(parents, func(i, j int) bool {
				return getMatchQuality(parents[i]) > getMatchQuality(parents[j])
			})

			// Build result with parents followed by their worktrees (if not folded)
			var result []manager.DiscoveredProject
			for _, parent := range parents {
				result = append(result, parent)

				// Add worktrees for this parent only if not folded, sorted by match quality
				if !m.worktreesFolded {
					worktrees := parentWorktrees[parent.Path]
					sort.SliceStable(worktrees, func(i, j int) bool {
						return getMatchQuality(worktrees[i]) > getMatchQuality(worktrees[j])
					})
					result = append(result, worktrees...)
				}
			}

			return result
		}
		
		// Partition matches by group activity, keeping parent-worktree hierarchy
		matchedParents := make(map[string]bool) // Track which parent projects matched
		parentsWithMatchingWorktrees := make(map[string]bool) // Track parents whose worktrees matched
		var activeGroupMatches []manager.DiscoveredProject
		var inactiveGroupMatches []manager.DiscoveredProject

		// First pass: find matching parent projects (search name only)
		for _, p := range projectsToFilter {
			if p.IsWorktree {
				continue // Skip worktrees in first pass
			}

			lowerName := strings.ToLower(p.Name)

			// Check if this parent project matches the filter (name only, not full path)
			if lowerName == filter || strings.HasPrefix(lowerName, filter) ||
			   strings.Contains(lowerName, filter) {
				matchedParents[p.Path] = true
			}
		}

		// Second pass: find worktrees that match and mark their parents for inclusion (only if worktrees not folded)
		if !m.worktreesFolded {
			for _, p := range projectsToFilter {
				if !p.IsWorktree {
					continue
				}

				lowerName := strings.ToLower(p.Name)

				// Check if this worktree matches the filter (name only, not full path)
				if lowerName == filter || strings.HasPrefix(lowerName, filter) ||
				   strings.Contains(lowerName, filter) {
					// Mark parent for inclusion even if parent didn't match directly
					parentsWithMatchingWorktrees[p.ParentPath] = true
				}
			}
		}

		// Third pass: add matched parents and their worktrees
		for _, p := range projectsToFilter {
			shouldInclude := false
			parentPath := p.Path

			if p.IsWorktree {
				parentPath = p.ParentPath
				// Include worktree only if not folded AND (it matches itself OR its parent matched)
				if !m.worktreesFolded {
					lowerName := strings.ToLower(p.Name)
					worktreeMatches := lowerName == filter || strings.HasPrefix(lowerName, filter) ||
						strings.Contains(lowerName, filter)
					parentMatched := matchedParents[p.ParentPath]

					shouldInclude = worktreeMatches || parentMatched
				}
			} else {
				// Include parent if it matched OR if any of its worktrees matched
				shouldInclude = matchedParents[p.Path] || parentsWithMatchingWorktrees[p.Path]
			}

			if shouldInclude {
				// Check group activity
				if activeGroups[parentPath] {
					activeGroupMatches = append(activeGroupMatches, p)
				} else {
					inactiveGroupMatches = append(inactiveGroupMatches, p)
				}
			}
		}

		// Sort both groups by match quality (parents will naturally group with their worktrees)
		activeGroupMatches = sortByMatchQuality(activeGroupMatches, filter)
		inactiveGroupMatches = sortByMatchQuality(inactiveGroupMatches, filter)

		// Combine: active groups first, then inactive groups
		m.filtered = []manager.DiscoveredProject{}
		m.filtered = append(m.filtered, activeGroupMatches...)
		m.filtered = append(m.filtered, inactiveGroupMatches...)
	}
}
func (m sessionizeModel) View() string {
	// If help is visible, show it and return
	if m.help.ShowAll {
		return m.help.View()
	}

	var b strings.Builder

	// Show key editing mode if active
	if m.editingKeys {
		return m.viewKeyEditor()
	}

	// Header with filter input (always at top)
	var header strings.Builder
	if m.ecosystemPickerMode {
		header.WriteString(core_theme.DefaultTheme.Info.Render("[Select Ecosystem to Focus]"))
		header.WriteString(" ")
	} else if m.focusedProject != nil {
		focusIndicator := core_theme.DefaultTheme.Info.Render(fmt.Sprintf("[Focus: %s]", m.focusedProject.Name))
		header.WriteString(focusIndicator)
		header.WriteString(" ")
	}
	header.WriteString(m.filterInput.View())
	b.WriteString(header.String())
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
		if project.ClaudeSession != nil {
			// This is a Claude session entry - use its own status
			statusSymbol := ""
			var statusColor lipgloss.Color
			switch project.ClaudeSession.Status {
			case "running":
				statusSymbol = "▶"
				statusColor = core_theme.DefaultColors.Green
			case "idle":
				statusSymbol = "⏸"
				statusColor = core_theme.DefaultColors.Yellow
			case "completed":
				statusSymbol = "✓"
				statusColor = core_theme.DefaultColors.Cyan
			case "failed", "error":
				statusSymbol = "✗"
				statusColor = core_theme.DefaultColors.Red
			default:
				statusColor = core_theme.DefaultColors.MutedText
			}

			claudeStatusStyled = lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			claudeDuration = project.ClaudeSession.Duration
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
			var statusColor lipgloss.Color
			switch claudeStatus {
			case "running":
				statusSymbol = "▶"
				statusColor = core_theme.DefaultColors.Green
			case "idle":
				statusSymbol = "⏸"
				statusColor = core_theme.DefaultColors.Yellow
			case "completed":
				statusSymbol = "✓"
				statusColor = core_theme.DefaultColors.Cyan
			case "failed", "error":
				statusSymbol = "✗"
				statusColor = core_theme.DefaultColors.Red
			default:
				statusColor = core_theme.DefaultColors.MutedText
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
		} else if project.GetExtendedGitStatus() != nil {
			// Use the enriched git status from the project
			coreExtStatus := project.GetExtendedGitStatus()
			extStatus = &extendedGitStatus{
				StatusInfo:   coreExtStatus.StatusInfo,
				LinesAdded:   coreExtStatus.LinesAdded,
				LinesDeleted: coreExtStatus.LinesDeleted,
			}
		}
		changesStr := formatChanges(project.GetGitStatus(), extStatus)

		// Prepare display elements
		prefix := "  "
		displayName := project.Name

		// Determine prefix based on mode
		if m.ecosystemPickerMode {
			// In ecosystem picker mode - show tree structure
			if project.IsWorktree {
				// Check if this is the last worktree of its parent
				isLast := true
				for j := i + 1; j < len(m.filtered); j++ {
					if m.filtered[j].IsWorktree && m.filtered[j].ParentPath == project.ParentPath {
						isLast = false
						break
					}
				}
				if isLast {
					prefix = "  └─ "
				} else {
					prefix = "  ├─ "
				}
			} else {
				// Main ecosystem - no prefix
				prefix = "  "
			}
		} else if m.focusedProject != nil {
			// In focus mode
			if project.Path == m.focusedProject.Path {
				// This is the focused ecosystem/worktree - show as parent
				prefix = "  "
			} else if project.IsWorktree {
				// This is a worktree - check if it's a direct child or nested
				if project.ParentPath == m.focusedProject.Path {
					// Direct worktree of the focused ecosystem
					prefix = "  └─ "
				} else {
					// Worktree of a repo within the focused ecosystem - show nested
					prefix = "    └─ "
				}
			} else {
				// Regular repo within the focused ecosystem - show as child
				prefix = "  ├─ "
			}
		} else {
			// Normal mode - show worktree indicator
			if project.IsWorktree {
				prefix = "  └─ "
			}
		}

		// If this is a Claude session, add PID to the name
		if project.ClaudeSession != nil {
			displayName = fmt.Sprintf("%s [PID:%d]", project.Name, project.ClaudeSession.PID)
		}

		// Highlight matching search terms
		filter := strings.ToLower(m.filterInput.Value())
		if filter != "" {
			displayName = highlightMatch(displayName, filter)
		}

		if i == m.cursor {
			// Highlight selected line
			indicator := core_theme.DefaultTheme.Highlight.Render("▶ ")

			nameStyle := core_theme.DefaultTheme.Selected
			pathStyle := core_theme.DefaultTheme.Info

			keyIndicator := "  " // Default: 2 spaces
			if keyMapping != "" {
				keyIndicator = core_theme.DefaultTheme.Highlight.Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")

				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Blue).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Green).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("%s%s%s", indicator, keyIndicator, sessionIndicator)
			if m.hasGroveHooks && m.showClaudeSessions {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)

			// Add path based on display mode (0=none, 1=compact, 2=full)
			if m.pathDisplayMode > 0 {
				displayPath := project.Path
				if m.pathDisplayMode == 1 {
					// Compact: replace home with ~
					displayPath = strings.Replace(displayPath, os.Getenv("HOME"), "~", 1)
				}
				line += "  " + pathStyle.Render(displayPath)
			}

			// Add git status if enabled
			if m.showGitStatus && sessionExists && changesStr != "" {
				line += "  " + changesStr
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && m.showClaudeSessions && claudeDuration != "" {
				line += "  " + core_theme.DefaultTheme.Muted.Render(claudeDuration)
			}

			b.WriteString(line)
		} else {
			// Normal line with colored name - style based on project type
			var nameStyle lipgloss.Style
			if project.IsWorktree {
				// Worktrees: Info style (blue)
				nameStyle = core_theme.DefaultTheme.Info
			} else {
				// Primary repos: Highlight style, bold (orange and bold)
				nameStyle = core_theme.DefaultTheme.Highlight.Copy().Bold(true)
			}
			pathStyle := core_theme.DefaultTheme.Muted

			// Always reserve space for key indicator
			keyIndicator := "  "
			if keyMapping != "" {
				keyIndicator = core_theme.DefaultTheme.Highlight.Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")

				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Blue).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Green).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("  %s%s", keyIndicator, sessionIndicator)
			if m.hasGroveHooks && m.showClaudeSessions {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)

			// Add path based on display mode (0=none, 1=compact, 2=full)
			if m.pathDisplayMode > 0 {
				displayPath := project.Path
				if m.pathDisplayMode == 1 {
					// Compact: replace home with ~
					displayPath = strings.Replace(displayPath, os.Getenv("HOME"), "~", 1)
				}
				line += "  " + pathStyle.Render(displayPath)
			}

			// Add git status if enabled
			if m.showGitStatus && sessionExists && changesStr != "" {
				line += "  " + changesStr
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && m.showClaudeSessions && claudeDuration != "" {
				line += "  " + core_theme.DefaultTheme.Muted.Render(claudeDuration)
			}

			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Show scroll indicators if needed
	if start > 0 || end < len(m.filtered) {
		scrollInfo := fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.filtered))
		b.WriteString(core_theme.DefaultTheme.Muted.Render(scrollInfo))
	}

	// Help text at bottom
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No active Claude sessions"))
		} else {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No matching Claude sessions"))
		}
	}

	// Help text
	helpStyle := core_theme.DefaultTheme.Muted
	b.WriteString("\n")

	// Build toggle indicators
	gitToggle := "g:git "
	if m.showGitStatus {
		gitToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render("✓")
	} else {
		gitToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.MutedText).Render("✗")
	}

	claudeToggle := " c:claude "
	if m.showClaudeSessions {
		claudeToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render("✓")
	} else {
		claudeToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.MutedText).Render("✗")
	}

	pathsToggle := " p:paths "
	switch m.pathDisplayMode {
	case 0:
		pathsToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.MutedText).Render("off")
	case 1:
		pathsToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render("~")
	case 2:
		pathsToggle += lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render("full")
	}

	togglesDisplay := fmt.Sprintf("[%s%s%s]", gitToggle, claudeToggle, pathsToggle)

	if m.ecosystemPickerMode {
		b.WriteString(helpStyle.Render("Enter to select • Esc to cancel"))
	} else if m.focusedProject != nil {
		b.WriteString(helpStyle.Render("Press ? for help • Press ctrl+g to clear focus • ") + togglesDisplay)
	} else {
		b.WriteString(helpStyle.Render("Press ? for help • ") + togglesDisplay)
	}

	// Display search paths at the very bottom
	if len(m.searchPaths) > 0 {
		b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("Search paths: "))
		// Truncate search paths if too long
		pathsDisplay := strings.Join(m.searchPaths, " • ")
		if len(pathsDisplay) > m.width-15 && m.width > 50 {
			pathsDisplay = pathsDisplay[:m.width-18] + "..."
		}
		b.WriteString(core_theme.DefaultTheme.Muted.Render(pathsDisplay))
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

	b.WriteString(core_theme.DefaultTheme.Header.Render(fmt.Sprintf("Select key for: %s", selectedProject)))
	b.WriteString("\n")
	b.WriteString(core_theme.DefaultTheme.Muted.Render(selectedPath))
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
			b.WriteString(core_theme.DefaultTheme.Highlight.Render("▶ "))
		} else {
			b.WriteString("  ")
		}

		// Key
		var keyStyle lipgloss.Style
		if d.isCurrent {
			keyStyle = core_theme.DefaultTheme.Warning
		} else if d.repository != "" {
			keyStyle = core_theme.DefaultTheme.Muted
		} else {
			keyStyle = core_theme.DefaultTheme.Success
		}
		b.WriteString(keyStyle.Render(fmt.Sprintf("%s ", d.key)))

		// Repository and path
		if d.repository != "" {
			b.WriteString(core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%-20s", d.repository)))
			b.WriteString(" ")
			b.WriteString(core_theme.DefaultTheme.Muted.Render(d.path))
		} else {
			b.WriteString(core_theme.DefaultTheme.Muted.Render("(available)"))
		}

		// Mark current
		if d.isCurrent {
			b.WriteString(core_theme.DefaultTheme.Warning.Render(" ← current"))
		}

		b.WriteString("\n")
	}

	// Scroll indicator
	if start > 0 || end < len(displays) {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("\n(%d-%d of %d)", start+1, end, len(displays))))
	}

	// Help text for key editor
	helpStyle := core_theme.DefaultTheme.Muted
	keyStyle := core_theme.DefaultTheme.Highlight

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("press ") + keyStyle.Render("key directly") + helpStyle.Render(" or "))
	b.WriteString(keyStyle.Render("↑/↓") + helpStyle.Render(" + ") + keyStyle.Render("enter") + helpStyle.Render(" to assign • "))
	b.WriteString(keyStyle.Render("esc") + helpStyle.Render(": cancel"))

	return b.String()
}
