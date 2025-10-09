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

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/components/table"
	"github.com/mattsolo1/grove-core/tui/keymap"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

// Message for enriched data of mapped projects
type enrichedProjectsMsg struct {
	projects []*workspace.ProjectInfo
}

// Message for CWD project enrichment
type cwdProjectEnrichedMsg struct {
	project *workspace.ProjectInfo
}

// Message for Claude session status updates
type claudeSessionsMsg struct {
	statusMap   map[string]string // path -> status
	durationMap map[string]string // path -> duration
}

var keyManageCmd = &cobra.Command{
	Use:     "manage",
	Aliases: []string{"m"},
	Short:   "Interactively manage tmux session key mappings",
	Long:    `Open an interactive table to map/unmap sessions to keys. Use arrow keys to navigate, 'e' to map CWD to an empty key, and space to unmap. Changes are auto-saved on exit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}

		// Get current sessions
		sessions, err := mgr.GetSessions()
		if err != nil {
			return fmt.Errorf("failed to get sessions: %w", err)
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions configured")
			return nil
		}

		// Detect current working directory
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}

		// Try to load cached enriched data for instant startup
		enrichedProjects := make(map[string]*workspace.ProjectInfo)
		usedCache := false
		if cache, err := manager.LoadKeyManageCache(configDir); err == nil && cache != nil && len(cache.EnrichedProjects) > 0 {
			// Convert cached projects to ProjectInfo
			for path, cached := range cache.EnrichedProjects {
				enrichedProjects[path] = &workspace.ProjectInfo{
					Name:                cached.Name,
					Path:                cached.Path,
					ParentPath:          cached.ParentPath,
					IsWorktree:          cached.IsWorktree,
					WorktreeName:        cached.WorktreeName,
					ParentEcosystemPath: cached.ParentEcosystemPath,
					IsEcosystem:         cached.IsEcosystem,
					GitStatus:           cached.GitStatus,
					ClaudeSession:       cached.ClaudeSession,
					NoteCounts:          cached.NoteCounts,
				}
			}
			usedCache = true
		}

		// Create the interactive model
		m := newManageModel(sessions, mgr, cwd, enrichedProjects, usedCache)

		// Run the interactive program
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		return nil
	},
}

// Styles
var (
	titleStyle = core_theme.DefaultTheme.Header

	selectedStyle = core_theme.DefaultTheme.Selected

	dimStyle = core_theme.DefaultTheme.Muted

	helpStyle = core_theme.DefaultTheme.Faint
)

// Model for the interactive session manager
type manageModel struct {
	cursor   int
	sessions []models.TmuxSession
	manager  *tmux.Manager
	keys     keyMap
	help     help.Model
	quitting bool
	message  string
	// CWD state
	cwdPath          string
	cwdProject       *workspace.ProjectInfo
	// Enriched data
	enrichedProjects map[string]*workspace.ProjectInfo // Caches enriched data by path
	// Claude session data
	claudeStatusMap   map[string]string // path -> claude status
	claudeDurationMap map[string]string // path -> claude duration
	// Navigation
	digitBuffer string
	setKeyMode  bool
	// Move mode state
	moveMode   bool
	lockedKeys map[string]bool // Track which keys are locked
	// Loading state
	isLoading    bool
	usedCache    bool
	spinnerFrame int
}

// Key bindings
type keyMap struct {
	keymap.Base
	Up        key.Binding
	Down      key.Binding
	Toggle    key.Binding
	Edit      key.Binding
	SetKey    key.Binding
	Open      key.Binding
	Delete    key.Binding
	Save      key.Binding
	MoveMode  key.Binding
	Lock      key.Binding
	MoveUp    key.Binding
	MoveDown  key.Binding
	ConfirmMove key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	// Return empty to show no help in footer - all help goes in popup
	return []key.Binding{}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up,
			k.Down,
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "Jump to row")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Switch to session")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Edit,
			k.SetKey,
			k.Toggle,
			k.Delete,
			k.Save,
			k.Help,
			k.Quit,
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Reorder")),
			k.MoveMode,
			k.Lock,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move row (in move mode)")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm move")),
		},
	}
}

var keys = keyMap{
	Base: keymap.NewBase(),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "quick toggle"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "map CWD"),
	),
	SetKey: key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "set key mode"),
	),
	Open: key.NewBinding(
		key.WithKeys("o", "enter"),
		key.WithHelp("enter/o", "switch to session"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d", "delete"),
		key.WithHelp("d/del", "clear mapping"),
	),
	Save: key.NewBinding(
		key.WithKeys("s", "ctrl+s"),
		key.WithHelp("s/ctrl+s", "save & exit"),
	),
	MoveMode: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "enter move mode"),
	),
	Lock: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "toggle lock"),
	),
	MoveUp: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "move up"),
	),
	MoveDown: key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "move down"),
	),
	ConfirmMove: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm move"),
	),
}

func newManageModel(sessions []models.TmuxSession, mgr *tmux.Manager, cwdPath string, cachedEnrichedProjects map[string]*workspace.ProjectInfo, usedCache bool) manageModel {
	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Session Key Manager - Help").
		Build()

	// Use cached enriched projects if provided, otherwise start with empty map
	enrichedProjects := cachedEnrichedProjects
	if enrichedProjects == nil {
		enrichedProjects = make(map[string]*workspace.ProjectInfo)
	}

	return manageModel{
		cursor:            0,
		sessions:          sessions,
		manager:           mgr,
		keys:              keys,
		help:              helpModel,
		cwdPath:           cwdPath,
		enrichedProjects:  enrichedProjects,
		claudeStatusMap:   make(map[string]string),
		claudeDurationMap: make(map[string]string),
		lockedKeys:        make(map[string]bool),
		usedCache:         usedCache,
		isLoading:         usedCache, // Start as loading if we used cache
	}
}

func (m manageModel) Init() tea.Cmd {
	// Ensure sessions are ordered with locked keys at bottom
	m.rebuildSessionsOrder()

	cmds := []tea.Cmd{
		enrichMappedProjectsCmd(m.sessions),
		enrichCwdProjectCmd(m.cwdPath),
		fetchClaudeSessionsForKeyManageCmd(),
	}

	// Start spinner animation if loading
	if m.isLoading {
		cmds = append(cmds, spinnerTickCmd())
	}

	return tea.Batch(cmds...)
}

// enrichMappedProjectsCmd fetches enriched data for mapped sessions
func enrichMappedProjectsCmd(sessions []models.TmuxSession) tea.Cmd {
	return func() tea.Msg {
		var projects []*workspace.ProjectInfo

		for _, s := range sessions {
			if s.Path == "" {
				continue
			}

			// Get project info
			projInfo, err := workspace.GetProjectByPath(s.Path)
			if err != nil {
				continue
			}

			projects = append(projects, projInfo)
		}

		// Enrich all projects with Git and Claude status
		ctx := context.Background()
		enrichOpts := &workspace.EnrichmentOptions{
			FetchGitStatus:      true,
			FetchClaudeSessions: true,
		}
		workspace.EnrichProjects(ctx, projects, enrichOpts)

		return enrichedProjectsMsg{projects: projects}
	}
}

// enrichCwdProjectCmd fetches and enriches the CWD project
func enrichCwdProjectCmd(cwdPath string) tea.Cmd {
	return func() tea.Msg {
		// Get project info for CWD
		projInfo, err := workspace.GetProjectByPath(cwdPath)
		if err != nil {
			// CWD is not a valid project
			return cwdProjectEnrichedMsg{project: nil}
		}

		// Enrich with Git status only (Claude sessions fetched separately)
		ctx := context.Background()
		enrichOpts := &workspace.EnrichmentOptions{
			FetchGitStatus:      true,
			FetchClaudeSessions: false,
		}
		workspace.EnrichProjects(ctx, []*workspace.ProjectInfo{projInfo}, enrichOpts)

		return cwdProjectEnrichedMsg{project: projInfo}
	}
}

// claudeSessionInfo matches the structure from grove-hooks
type claudeSessionInfo struct {
	WorkingDirectory      string `json:"working_directory"`
	Status                string `json:"status"`
	StateDuration         string `json:"state_duration"`
	StateDurationSeconds  int    `json:"state_duration_seconds"`
}

// fetchClaudeSessionsForKeyManageCmd fetches Claude session data from grove-hooks
func fetchClaudeSessionsForKeyManageCmd() tea.Cmd {
	return func() tea.Msg {
		statusMap := make(map[string]string)
		durationMap := make(map[string]string)

		// Try to find grove-hooks
		groveHooksPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "grove-hooks")
		var cmd *exec.Cmd
		if _, err := os.Stat(groveHooksPath); err == nil {
			cmd = exec.Command(groveHooksPath, "sessions", "list", "--active", "--json")
		} else {
			cmd = exec.Command("grove-hooks", "sessions", "list", "--active", "--json")
		}

		output, err := cmd.Output()
		if err != nil {
			// grove-hooks not available or no sessions
			return claudeSessionsMsg{statusMap: statusMap, durationMap: durationMap}
		}

		var sessions []claudeSessionInfo
		if err := json.Unmarshal(output, &sessions); err != nil {
			return claudeSessionsMsg{statusMap: statusMap, durationMap: durationMap}
		}

		// Build maps by clean path (case-insensitive on macOS)
		for _, session := range sessions {
			cleanPath := filepath.Clean(session.WorkingDirectory)
			// Use lowercase for case-insensitive matching on macOS
			normalizedPath := strings.ToLower(cleanPath)
			statusMap[normalizedPath] = session.Status
			durationMap[normalizedPath] = session.StateDuration
		}

		return claudeSessionsMsg{statusMap: statusMap, durationMap: durationMap}
	}
}

func (m manageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cwdProjectEnrichedMsg:
		m.cwdProject = msg.project
		return m, nil

	case enrichedProjectsMsg:
		// Populate enriched projects map
		for _, proj := range msg.projects {
			cleanPath := filepath.Clean(proj.Path)
			m.enrichedProjects[cleanPath] = proj
		}

		// Mark loading as complete
		m.isLoading = false

		// Save cache for next startup
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)

		return m, nil

	case claudeSessionsMsg:
		m.claudeStatusMap = msg.statusMap
		m.claudeDurationMap = msg.durationMap
		return m, nil

	case spinnerTickMsg:
		// Update spinner animation frame
		if m.isLoading {
			m.spinnerFrame++
			return m, spinnerTickCmd() // Reschedule next spinner tick
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.help.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		// If help is visible, it consumes all key presses
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		// Handle set key mode
		if m.setKeyMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.setKeyMode = false
				m.message = "Set key cancelled."
			case tea.KeyRunes:
				input := msg.String()
				// Check if it's a number (for indexed binding)
				if num, err := strconv.Atoi(input); err == nil && num > 0 {
					targetIndex := num - 1
					if targetIndex < len(m.sessions) {
						m.mapKeyToCwd(targetIndex)
					} else {
						m.message = fmt.Sprintf("Invalid number: %d", num)
					}
				} else { // It's a letter for direct key binding
					targetKey := strings.ToLower(input)
					targetIndex := -1
					for i, s := range m.sessions {
						if s.Key == targetKey {
							targetIndex = i
							break
						}
					}
					if targetIndex != -1 {
						m.mapKeyToCwd(targetIndex)
					} else {
						m.message = fmt.Sprintf("Invalid key: %s", targetKey)
					}
				}
			}
			return m, nil // Consume keypress
		}

		// Handle numbered navigation - opens session immediately
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			if r := msg.Runes[0]; r >= '0' && r <= '9' {
				m.digitBuffer += string(r)
				if len(m.digitBuffer) > 3 { // Cap buffer length
					m.digitBuffer = m.digitBuffer[len(m.digitBuffer)-3:]
				}

				num, err := strconv.Atoi(m.digitBuffer)
				if err == nil && num > 0 {
					targetIndex := num - 1
					if targetIndex < len(m.sessions) {
						// Open the session immediately
						session := m.sessions[targetIndex]
						if session.Path != "" {
							if os.Getenv("TMUX") != "" {
								// Get project info to generate proper session name
								projInfo, err := workspace.GetProjectByPath(session.Path)
								if err != nil {
									m.message = fmt.Sprintf("Failed to get project info: %v", err)
									m.digitBuffer = ""
									return m, nil
								}
								sessionName := projInfo.Identifier()

								// Create tmux client
								client, err := tmuxclient.NewClient()
								if err != nil {
									m.message = fmt.Sprintf("Failed to create tmux client: %v", err)
									m.digitBuffer = ""
									return m, nil
								}

								ctx := context.Background()

								// Check if session exists
								exists, err := client.SessionExists(ctx, sessionName)
								if err != nil {
									m.message = fmt.Sprintf("Failed to check session: %v", err)
									m.digitBuffer = ""
									return m, nil
								}

								if !exists {
									// Session doesn't exist, create it
									opts := tmuxclient.LaunchOptions{
										SessionName:      sessionName,
										WorkingDirectory: session.Path,
									}
									if err := client.Launch(ctx, opts); err != nil {
										m.message = fmt.Sprintf("Failed to create session: %v", err)
										m.digitBuffer = ""
										return m, nil
									}
								}

								// Switch to the session
								if err := client.SwitchClient(ctx, sessionName); err != nil {
									m.message = fmt.Sprintf("Failed to switch to session: %v", err)
								} else {
									// Exit the manager after switching
									m.message = fmt.Sprintf("Switching to %s...", sessionName)
									m.quitting = true
									return m, tea.Quit
								}
							} else {
								m.message = "Not in a tmux session"
							}
						} else {
							m.message = "No session mapped to this key"
						}
					}
				}
				m.digitBuffer = ""
				return m, nil // Consume digit
			}
		}

		// Any non-digit key press resets the buffer
		m.digitBuffer = ""

		// Handle move mode
		if m.moveMode {
			switch {
			case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.MoveMode), msg.Type == tea.KeyEsc:
				// Exit move mode
				m.moveMode = false
				m.message = "Exited move mode"
				return m, nil

			case key.Matches(msg, m.keys.Lock):
				// Toggle lock for current key
				if m.cursor < len(m.sessions) {
					currentKey := m.sessions[m.cursor].Key
					if m.lockedKeys[currentKey] {
						delete(m.lockedKeys, currentKey)
						m.message = fmt.Sprintf("Unlocked key '%s'", currentKey)
					} else {
						m.lockedKeys[currentKey] = true
						m.message = fmt.Sprintf("Locked key '%s'", currentKey)
					}
					// Rebuild order to move locked keys to bottom
					m.rebuildSessionsOrder()
				}
				return m, nil

			case key.Matches(msg, m.keys.MoveUp):
				// Move row up (swap with previous unlocked row)
				if m.cursor > 0 && m.cursor < len(m.sessions) {
					currentKey := m.sessions[m.cursor].Key

					// Check if current key is locked
					if m.lockedKeys[currentKey] {
						m.message = "Cannot move locked key"
						return m, nil
					}

					// Find the previous unlocked position
					targetPos := m.cursor - 1
					for targetPos >= 0 && m.lockedKeys[m.sessions[targetPos].Key] {
						targetPos--
					}

					if targetPos >= 0 {
						// Swap sessions
						m.sessions[m.cursor], m.sessions[targetPos] = m.sessions[targetPos], m.sessions[m.cursor]
						// Move cursor with the row
						m.cursor = targetPos
						m.message = "Moved up"
					} else {
						m.message = "Cannot move past locked keys"
					}
				}
				return m, nil

			case key.Matches(msg, m.keys.MoveDown):
				// Move row down (swap with next unlocked row)
				if m.cursor >= 0 && m.cursor < len(m.sessions)-1 {
					currentKey := m.sessions[m.cursor].Key

					// Check if current key is locked
					if m.lockedKeys[currentKey] {
						m.message = "Cannot move locked key"
						return m, nil
					}

					// Find the next unlocked position
					targetPos := m.cursor + 1
					for targetPos < len(m.sessions) && m.lockedKeys[m.sessions[targetPos].Key] {
						targetPos++
					}

					if targetPos < len(m.sessions) {
						// Swap sessions
						m.sessions[m.cursor], m.sessions[targetPos] = m.sessions[targetPos], m.sessions[m.cursor]
						// Move cursor with the row
						m.cursor = targetPos
						m.message = "Moved down"
					} else {
						m.message = "Cannot move past locked keys"
					}
				}
				return m, nil

			case key.Matches(msg, m.keys.ConfirmMove):
				// Save and exit move mode
				if err := m.manager.UpdateSessions(m.sessions); err != nil {
					m.message = fmt.Sprintf("Error saving: %v", err)
				} else {
					if err := m.manager.RegenerateBindings(); err != nil {
						m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
					} else {
						m.message = "Order saved!"
					}
				}
				m.moveMode = false
				return m, nil
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.MoveMode):
			// Enter move mode
			m.moveMode = true
			m.message = "Move mode: use j/k to reorder, l to lock/unlock, enter to save, q/m to cancel"
			return m, nil

		case key.Matches(msg, m.keys.Lock):
			// Toggle lock (works outside move mode too)
			if m.cursor < len(m.sessions) {
				currentKey := m.sessions[m.cursor].Key
				if m.lockedKeys[currentKey] {
					delete(m.lockedKeys, currentKey)
					m.message = fmt.Sprintf("Unlocked key '%s'", currentKey)
				} else {
					m.lockedKeys[currentKey] = true
					m.message = fmt.Sprintf("Locked key '%s'", currentKey)
				}
				// Rebuild order to move locked keys to bottom
				m.rebuildSessionsOrder()
			}
			return m, nil

		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Quit):
			// Auto-save on quit
			if err := m.manager.UpdateSessions(m.sessions); err != nil {
				m.message = fmt.Sprintf("Error saving: %v", err)
				// Show error briefly then quit
				m.quitting = true
				return m, tea.Quit
			}

			// Regenerate bindings
			if err := m.manager.RegenerateBindings(); err != nil {
				m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
			} else {
				// Try to reload tmux config
				if err := reloadTmuxConfig(); err != nil {
					m.message = "Changes saved! (Failed to auto-reload tmux: " + err.Error() + ")"
				} else {
					m.message = "Changes saved and tmux config reloaded!"
				}
			}

			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Save):
			// Save changes
			if err := m.manager.UpdateSessions(m.sessions); err != nil {
				m.message = fmt.Sprintf("Error saving: %v", err)
			} else {
				// Regenerate bindings
				if err := m.manager.RegenerateBindings(); err != nil {
					m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
				} else {
					// Try to reload tmux config
					if err := reloadTmuxConfig(); err != nil {
						m.message = "Changes saved! (Failed to auto-reload tmux: " + err.Error() + ")"
					} else {
						m.message = "Changes saved and tmux config reloaded!"
					}
					m.quitting = true
					return m, tea.Quit
				}
			}

		case key.Matches(msg, m.keys.SetKey):
			// Enter set key mode
			if m.cwdProject == nil {
				m.message = "Current directory is not a valid workspace/project"
				return m, nil
			}

			m.setKeyMode = true
			m.message = "Enter key or number to map CWD to. (ESC to cancel)"
			return m, nil

		case key.Matches(msg, m.keys.Edit):
			// Map CWD to the selected empty key slot
			if m.cursor >= len(m.sessions) {
				return m, nil
			}

			session := &m.sessions[m.cursor]

			// Check if the key slot is already mapped
			if session.Path != "" {
				m.message = fmt.Sprintf("Key '%s' is already mapped. Clear it first with 'd' or space.", session.Key)
				return m, nil
			}

			// Check if CWD was successfully resolved
			if m.cwdProject == nil {
				m.message = "Current directory is not a valid workspace/project"
				return m, nil
			}

			// Check if CWD is already mapped to another key
			cwdCleanPath := filepath.Clean(m.cwdProject.Path)
			for _, s := range m.sessions {
				if s.Path != "" {
					sCleanPath := filepath.Clean(s.Path)
					if sCleanPath == cwdCleanPath {
						m.message = fmt.Sprintf("CWD is already mapped to key '%s'", s.Key)
						return m, nil
					}
				}
			}

			// Map the CWD to this key
			session.Path = m.cwdProject.Path
			session.Repository = ""
			session.Description = ""

			// Add to enriched projects map for immediate display
			m.enrichedProjects[cwdCleanPath] = m.cwdProject

			m.message = fmt.Sprintf("Mapped key '%s' to '%s'", session.Key, m.cwdProject.Name)
			return m, nil

		case key.Matches(msg, m.keys.Open):
			// Open/switch to the session
			if m.cursor < len(m.sessions) {
				session := m.sessions[m.cursor]
				if session.Path != "" {
					if os.Getenv("TMUX") != "" {
						// Get project info to generate proper session name
						projInfo, err := workspace.GetProjectByPath(session.Path)
						if err != nil {
							m.message = fmt.Sprintf("Failed to get project info: %v", err)
							return m, nil
						}
						sessionName := projInfo.Identifier()

						// Create tmux client
						client, err := tmuxclient.NewClient()
						if err != nil {
							m.message = fmt.Sprintf("Failed to create tmux client: %v", err)
							return m, nil
						}

						ctx := context.Background()

						// Check if session exists
						exists, err := client.SessionExists(ctx, sessionName)
						if err != nil {
							m.message = fmt.Sprintf("Failed to check session: %v", err)
							return m, nil
						}

						if !exists {
							// Session doesn't exist, create it
							opts := tmuxclient.LaunchOptions{
								SessionName:      sessionName,
								WorkingDirectory: session.Path,
							}
							if err := client.Launch(ctx, opts); err != nil {
								m.message = fmt.Sprintf("Failed to create session: %v", err)
								return m, nil
							}
						}

						// Switch to the session
						if err := client.SwitchClient(ctx, sessionName); err != nil {
							m.message = fmt.Sprintf("Failed to switch to session: %v", err)
						} else {
							// Exit the manager after switching
							m.message = fmt.Sprintf("Switching to %s...", sessionName)
							m.quitting = true
							return m, tea.Quit
						}
					} else {
						m.message = "Not in a tmux session"
					}
				} else {
					m.message = "No session mapped to this key"
				}
			}

		case key.Matches(msg, m.keys.Toggle):
			// Quick toggle - unmap if mapped
			if m.cursor < len(m.sessions) {
				session := &m.sessions[m.cursor]
				if session.Path != "" {
					// Clear the session
					session.Path = ""
					session.Repository = ""
					session.Description = ""
					m.message = fmt.Sprintf("Unmapped key %s", session.Key)
				} else {
					m.message = "Press 'e' or Enter to map this key"
				}
			}

		case key.Matches(msg, m.keys.Delete):
			// Clear the mapping for selected session
			if m.cursor < len(m.sessions) {
				session := &m.sessions[m.cursor]
				if session.Path != "" {
					// Clear the session
					session.Path = ""
					session.Repository = ""
					session.Description = ""

					m.message = fmt.Sprintf("Unmapped key %s", session.Key)
				}
			}

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
				// Rebuild sessions order based on locked status
				m.rebuildSessionsOrder()
			}

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
				// Rebuild sessions order based on locked status
				m.rebuildSessionsOrder()
			}
		}
	}

	return m, nil
}

func (m manageModel) View() string {
	if m.quitting && m.message != "" {
		return m.message + "\n"
	}

	// If help is visible, show it and return
	if m.help.ShowAll {
		return m.help.View()
	}

	var b strings.Builder

	// Title with mode indicators
	inlineTitleStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Bold(true)
	b.WriteString(inlineTitleStyle.Render("Session Hotkeys"))

	// Show move mode indicator
	if m.moveMode {
		moveModeStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Yellow).Bold(true)
		b.WriteString(" " + moveModeStyle.Render("[MOVE MODE]"))
	}

	// Show loading indicator
	if m.isLoading {
		spinnerFrames := []string{"◐", "◓", "◑", "◒"}
		spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		b.WriteString(" " + core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s Updating...", spinner)))
	}
	b.WriteString("\n\n")

	// Separate sessions into unlocked and locked
	var unlockedSessions []models.TmuxSession
	var lockedSessions []models.TmuxSession

	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
			lockedSessions = append(lockedSessions, s)
		} else {
			unlockedSessions = append(unlockedSessions, s)
		}
	}

	// Build table data
	headers := []string{"#", "Key", "Repository", "Worktree", "Git", "Claude", "Ecosystem"}
	var unlockedRows [][]string
	var lockedRows [][]string

	// Build unlocked rows
	for i, s := range unlockedSessions {
		var ecosystem, repository, worktree string
		gitStatus := ""
		claudeStatus := ""

		if s.Path != "" {
			cleanPath := filepath.Clean(s.Path)
			if projInfo, found := m.enrichedProjects[cleanPath]; found {

				// RULE 1: Determine Repository and Worktree.
				// For a worktree, Repository is its parent. Otherwise, it's the project itself.
				if projInfo.IsWorktree && projInfo.ParentPath != "" {
					repository = filepath.Base(projInfo.ParentPath)
					worktree = projInfo.Name
				} else {
					repository = projInfo.Name
				}

				// RULE 2: Determine Ecosystem display.
				if projInfo.ParentEcosystemPath != "" {
					// Project is within an ecosystem.
					baseEcosystem := filepath.Base(projInfo.ParentEcosystemPath)
					ecoWorktreeName := projInfo.WorktreeName // The eco-worktree the project is in.

					// Set ecosystem name
					ecosystem = baseEcosystem

					// If this project is inside an ecosystem worktree, show the eco-worktree in Worktree column with indicator
					if ecoWorktreeName != "" {
						// Repository should be the actual project name (not the ecosystem)
						repository = projInfo.Name

						// If this is also a worktree of that repo, keep the worktree name
						// Otherwise, show the ecosystem worktree name with indicator
						if projInfo.IsWorktree && projInfo.ParentPath != "" {
							// This is a worktree of a repo inside an eco-worktree
							repository = filepath.Base(projInfo.ParentPath)
							worktree = projInfo.Name
						} else {
							// This is a repo inside an eco-worktree (not a worktree itself)
							worktree = ecoWorktreeName + " *"
						}
					}
				} else if projInfo.IsEcosystem {
					// It's a root ecosystem.
					ecosystem = projInfo.Name
				}

				// RULE 3: Clean up redundancies for clarity.
				if projInfo.IsEcosystem && !projInfo.IsWorktree {
					// For a root ecosystem, its name is in the Ecosystem column. Don't repeat it in Repository.
					repository = ""
				}

				// Format Git status (with colors)
				if projInfo.GitStatus != nil {
					if extStatus, ok := projInfo.GitStatus.(*workspace.ExtendedGitStatus); ok {
						gitStatus = formatChanges(extStatus.StatusInfo, extStatus)
					}
				}
			} else {
				// Fallback if no enriched data
				repository = filepath.Base(s.Path)
			}

			// Format Claude status (with colors) - check claudeStatusMap
			// Use lowercase for case-insensitive matching on macOS
			normalizedPath := strings.ToLower(cleanPath)
			if status, found := m.claudeStatusMap[normalizedPath]; found {
				claudeStatus = formatClaudeStatusFromMap(status, m.claudeDurationMap[normalizedPath])
			}
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			s.Key,
			repository,
			worktree,
			gitStatus,
			claudeStatus,
			ecosystem,
		}
		unlockedRows = append(unlockedRows, row)
	}

	// Build locked rows
	for i, s := range lockedSessions {
		var ecosystem, repository, worktree string
		gitStatus := ""
		claudeStatus := ""

		if s.Path != "" {
			cleanPath := filepath.Clean(s.Path)
			if projInfo, found := m.enrichedProjects[cleanPath]; found {

				// RULE 1: Determine Repository and Worktree.
				// For a worktree, Repository is its parent. Otherwise, it's the project itself.
				if projInfo.IsWorktree && projInfo.ParentPath != "" {
					repository = filepath.Base(projInfo.ParentPath)
					worktree = projInfo.Name
				} else {
					repository = projInfo.Name
				}

				// RULE 2: Determine Ecosystem display.
				if projInfo.ParentEcosystemPath != "" {
					// Project is within an ecosystem.
					baseEcosystem := filepath.Base(projInfo.ParentEcosystemPath)
					ecoWorktreeName := projInfo.WorktreeName // The eco-worktree the project is in.

					// Set ecosystem name
					ecosystem = baseEcosystem

					// If this project is inside an ecosystem worktree, show the eco-worktree in Worktree column with indicator
					if ecoWorktreeName != "" {
						// Repository should be the actual project name (not the ecosystem)
						repository = projInfo.Name

						// If this is also a worktree of that repo, keep the worktree name
						// Otherwise, show the ecosystem worktree name with indicator
						if projInfo.IsWorktree && projInfo.ParentPath != "" {
							// This is a worktree of a repo inside an eco-worktree
							repository = filepath.Base(projInfo.ParentPath)
							worktree = projInfo.Name
						} else {
							// This is a repo inside an eco-worktree (not a worktree itself)
							worktree = ecoWorktreeName + " *"
						}
					}
				} else if projInfo.IsEcosystem {
					// It's a root ecosystem.
					ecosystem = projInfo.Name
				}

				// RULE 3: Clean up redundancies for clarity.
				if projInfo.IsEcosystem && !projInfo.IsWorktree {
					// For a root ecosystem, its name is in the Ecosystem column. Don't repeat it in Repository.
					repository = ""
				}

				// Format Git status (with colors)
				if projInfo.GitStatus != nil {
					if extStatus, ok := projInfo.GitStatus.(*workspace.ExtendedGitStatus); ok {
						gitStatus = formatChanges(extStatus.StatusInfo, extStatus)
					}
				}
			} else {
				// Fallback if no enriched data
				repository = filepath.Base(s.Path)
			}

			// Format Claude status (with colors) - check claudeStatusMap
			// Use lowercase for case-insensitive matching on macOS
			normalizedPath := strings.ToLower(cleanPath)
			if status, found := m.claudeStatusMap[normalizedPath]; found {
				claudeStatus = formatClaudeStatusFromMap(status, m.claudeDurationMap[normalizedPath])
			}
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			s.Key,
			repository,
			worktree,
			gitStatus,
			claudeStatus,
			ecosystem,
		}
		lockedRows = append(lockedRows, row)
	}

	// Calculate which section the cursor is in
	cursorInUnlocked := m.cursor < len(unlockedSessions)
	var adjustedCursor int
	if cursorInUnlocked {
		adjustedCursor = m.cursor
	} else {
		adjustedCursor = m.cursor - len(unlockedSessions)
	}

	// Render unlocked table with selection if cursor is in this section
	if len(unlockedRows) > 0 {
		var unlockedTableStr string
		if cursorInUnlocked {
			unlockedTableStr = table.SelectableTableWithOptions(headers, unlockedRows, adjustedCursor, table.SelectableTableOptions{
				HighlightColumn: 2, // Repository column
			})
		} else {
			// No selection in this table
			unlockedTableStr = table.SelectableTableWithOptions(headers, unlockedRows, -1, table.SelectableTableOptions{
				HighlightColumn: 2,
			})
		}
		b.WriteString(unlockedTableStr)
		b.WriteString("\n")
	}

	// Render locked section if there are locked keys
	if len(lockedRows) > 0 {
		b.WriteString("\n")
		lockedHeaderStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Yellow).Bold(true)
		b.WriteString(lockedHeaderStyle.Render("Locked Keys") + "\n")

		var lockedTableStr string
		if !cursorInUnlocked {
			lockedTableStr = table.SelectableTableWithOptions(headers, lockedRows, adjustedCursor, table.SelectableTableOptions{
				HighlightColumn: 2,
			})
		} else {
			lockedTableStr = table.SelectableTableWithOptions(headers, lockedRows, -1, table.SelectableTableOptions{
				HighlightColumn: 2,
			})
		}
		b.WriteString(lockedTableStr)
	}

	b.WriteString("\n\n")

	if m.message != "" {
		b.WriteString(dimStyle.Render(m.message) + "\n\n")
	}

	// Show different help text based on mode
	if m.setKeyMode {
		b.WriteString(helpStyle.Render("SET KEY MODE: Press key or number to map CWD. ESC to cancel."))
	} else if m.moveMode {
		b.WriteString(helpStyle.Render("MOVE MODE: j/k to move • l to lock/unlock • enter to save • q/m to cancel"))
	} else {
		b.WriteString(helpStyle.Render("Press ? for help • m for move mode • l to lock rows • * = part of ecosystem worktree"))
	}

	return b.String()
}

// rebuildSessionsOrder ensures locked keys are always at the bottom
func (m *manageModel) rebuildSessionsOrder() {
	var unlocked []models.TmuxSession
	var locked []models.TmuxSession

	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
			locked = append(locked, s)
		} else {
			unlocked = append(unlocked, s)
		}
	}

	m.sessions = append(unlocked, locked...)
}

// mapKeyToCwd maps the CWD to the target key index
func (m *manageModel) mapKeyToCwd(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(m.sessions) {
		return
	}

	targetSession := &m.sessions[targetIndex]
	cwdCleanPath := filepath.Clean(m.cwdProject.Path)

	// Find and clear any pre-existing mapping for the CWD path
	for i := range m.sessions {
		if m.sessions[i].Path != "" {
			sessionCleanPath := filepath.Clean(m.sessions[i].Path)
			if sessionCleanPath == cwdCleanPath {
				m.sessions[i].Path = ""
				m.sessions[i].Repository = ""
				m.sessions[i].Description = ""
				break
			}
		}
	}

	// Update the target session with CWD
	targetSession.Path = m.cwdProject.Path
	targetSession.Repository = ""
	targetSession.Description = ""

	// Add to enriched projects map for immediate UI refresh
	m.enrichedProjects[cwdCleanPath] = m.cwdProject

	// Exit set key mode
	m.setKeyMode = false

	// Set success message
	m.message = fmt.Sprintf("Mapped key '%s' to '%s'", targetSession.Key, m.cwdProject.Name)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatClaudeStatusFromMap formats Claude status from the status and duration maps
func formatClaudeStatusFromMap(status, duration string) string {
	if status == "" {
		return ""
	}

	statusSymbol := ""
	var statusColor lipgloss.Color
	switch status {
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
		return ""
	}

	statusStyled := lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)

	if duration != "" {
		return statusStyled + " " + duration
	}
	return statusStyled
}

// formatClaudeStatus formats the Claude session status into a styled string
func formatClaudeStatus(session *workspace.ClaudeSessionInfo) string {
	if session == nil {
		return ""
	}

	return formatClaudeStatusFromMap(session.Status, session.Duration)
}

// formatGitStatusPlain formats Git status without ANSI codes for table display
func formatGitStatusPlain(status *git.StatusInfo, extStatus *workspace.ExtendedGitStatus) string {
	if status == nil {
		return ""
	}

	var changes []string

	if status.HasUpstream {
		if status.AheadCount > 0 {
			changes = append(changes, fmt.Sprintf("↑%d", status.AheadCount))
		}
		if status.BehindCount > 0 {
			changes = append(changes, fmt.Sprintf("↓%d", status.BehindCount))
		}
	}

	if status.ModifiedCount > 0 {
		changes = append(changes, fmt.Sprintf("M:%d", status.ModifiedCount))
	}
	if status.StagedCount > 0 {
		changes = append(changes, fmt.Sprintf("S:%d", status.StagedCount))
	}
	if status.UntrackedCount > 0 {
		changes = append(changes, fmt.Sprintf("?:%d", status.UntrackedCount))
	}

	// Add lines added/deleted if available
	if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
		if extStatus.LinesAdded > 0 {
			changes = append(changes, fmt.Sprintf("+%d", extStatus.LinesAdded))
		}
		if extStatus.LinesDeleted > 0 {
			changes = append(changes, fmt.Sprintf("-%d", extStatus.LinesDeleted))
		}
	}

	changesStr := strings.Join(changes, " ")

	// If repo is clean (no changes)
	if !status.IsDirty && changesStr == "" {
		if status.HasUpstream {
			return "✓"
		} else {
			return "○"
		}
	}

	return changesStr
}

// formatClaudeStatusPlain formats Claude status without ANSI codes for table display
func formatClaudeStatusPlain(session *workspace.ClaudeSessionInfo) string {
	if session == nil {
		return ""
	}

	statusSymbol := ""
	switch session.Status {
	case "running":
		statusSymbol = "▶"
	case "idle":
		statusSymbol = "⏸"
	case "completed":
		statusSymbol = "✓"
	case "failed", "error":
		statusSymbol = "✗"
	default:
		return ""
	}

	if session.Duration != "" {
		return statusSymbol + " " + session.Duration
	}
	return statusSymbol
}

// reloadTmuxConfig reloads the tmux configuration
func reloadTmuxConfig() error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not in a tmux session")
	}

	// Run tmux source-file command
	cmd := exec.Command("tmux", "source-file", expandPath("~/.tmux.conf"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux reload failed: %s", string(output))
	}

	return nil
}

func init() {
	keyCmd.AddCommand(keyManageCmd)
}
