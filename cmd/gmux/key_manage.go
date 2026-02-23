package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	grovecontext "github.com/grovetools/cx/pkg/context"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/models"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/spf13/cobra"
)

// resolveIcon converts icon references to actual icon characters.
// Supports preset names like "IconEcosystem" or direct unicode characters.
func resolveIcon(iconRef string) string {
	switch iconRef {
	case "IconTree":
		return core_theme.IconTree
	case "IconProject":
		return core_theme.IconProject
	case "IconRepo":
		return core_theme.IconRepo
	case "IconWorktree":
		return core_theme.IconWorktree
	case "IconEcosystem":
		return core_theme.IconEcosystem
	case "IconFolder":
		return core_theme.IconFolder
	case "IconHome":
		return core_theme.IconHome
	case "IconCloud":
		return "󰅧"
	case "IconCode":
		return core_theme.IconCode
	case "IconBriefcase":
		return "󰃖"
	default:
		return iconRef
	}
}

// Message for CWD project enrichment
type cwdProjectEnrichedMsg struct {
	project *manager.SessionizeProject
}

// New message
type rulesStateMsg struct {
	rulesState map[string]grovecontext.RuleStatus
}

// New command
func fetchRulesStateForKeyManageCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		mgr := grovecontext.NewManager("")
		rulesState := make(map[string]grovecontext.RuleStatus)
		for _, p := range projects {
			rule := filepath.Join(p.Path, "**")
			rulesState[p.Path] = mgr.GetRuleStatus(rule)
		}
		return rulesStateMsg{rulesState}
	}
}

var keyManageCmd = &cobra.Command{
	Use:     "manage",
	Aliases: []string{"m"},
	Short:   "Interactively manage tmux session key mappings",
	Long:    `Open an interactive table to map/unmap sessions to keys. Use arrow keys to navigate, 'e' to map CWD to an empty key, and space to unmap. Changes are auto-saved on exit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKeyManageTUIImpl(cmd, args)
	},
}

// runKeyManageTUIImpl runs the key manage TUI implementation.
func runKeyManageTUIImpl(cmd *cobra.Command, args []string) error {
	mgr, err := tmux.NewManager(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	// Detect current working directory for auto-selection
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Auto-select group based on last accessed or CWD
	if targetGroup == "" {
		if last := mgr.GetLastAccessedGroup(); last != "" {
			mgr.SetActiveGroup(last)
		} else if matched := mgr.FindGroupForPath(cwd); matched != "" {
			mgr.SetActiveGroup(matched)
		} else {
			mgr.SetActiveGroup("default")
		}
	} else {
		mgr.SetActiveGroup(targetGroup)
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

	// Try to load cached enriched data for instant startup
	enrichedProjects := make(map[string]*manager.SessionizeProject)
	usedCache := false
	if cache, err := manager.LoadKeyManageCache(configDir); err == nil && cache != nil && len(cache.EnrichedProjects) > 0 {
		// Convert cached projects to SessionizeProject, validating paths exist
		for path, cached := range cache.EnrichedProjects {
			// Validate that the path still exists
			if _, err := os.Stat(path); err == nil {
				enrichedProjects[path] = &manager.SessionizeProject{
					WorkspaceNode: cached.WorkspaceNode,
					GitStatus:     cached.GitStatus,
					NoteCounts:    cached.NoteCounts,
					PlanStats:     cached.PlanStats,
				}
			}
			// Skip stale entries (paths that no longer exist)
		}
		usedCache = len(enrichedProjects) > 0
	}

	// Create the interactive model
	m := newManageModel(sessions, mgr, cwd, enrichedProjects, usedCache)

	// Run the interactive program
	p := tea.NewProgram(&m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	// Handle post-TUI logic
	if mm, ok := finalModel.(*manageModel); ok {
		// Save changes if any were made
		if mm.changesMade {
			if err := mgr.UpdateSessionsAndLocks(mm.sessions, mm.getLockedKeysSlice()); err != nil {
				return fmt.Errorf("failed to save sessions: %w", err)
			}

			if err := mgr.RegenerateBindings(); err != nil {
				return fmt.Errorf("failed to regenerate bindings: %w", err)
			}

			_ = reloadTmuxConfig() // Silent reload
		}

		// Execute command on exit if set
		if mm.commandOnExit != nil {
			mm.commandOnExit.Stdin = os.Stdin
			mm.commandOnExit.Stdout = os.Stdout
			mm.commandOnExit.Stderr = os.Stderr
			if err := mm.commandOnExit.Run(); err != nil {
				// Silently ignore popup close errors
			}
		}

		// Handle handoff to other TUI
		if mm.nextCommand == "groups" {
			return runGroupsTUIImpl(cmd, args, true)
		}
	}

	return nil
}

// Styles
var (
	titleStyle = core_theme.DefaultTheme.Header

	selectedStyle = core_theme.DefaultTheme.Selected

	dimStyle = core_theme.DefaultTheme.Muted

	helpStyle = core_theme.DefaultTheme.Muted
)

// Model for the interactive session manager
type manageModel struct {
	cursor   int
	sessions []models.TmuxSession
	manager  *tmux.Manager
	keys     manageKeyMap
	help     help.Model
	quitting bool
	message  string
	// CWD state
	cwdPath    string
	cwdProject *manager.SessionizeProject
	// Enriched data
	enrichedProjects  map[string]*manager.SessionizeProject // Caches enriched data by path
	enrichmentLoading map[string]bool                       // tracks which enrichments are currently loading
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
	// View toggles
	pathDisplayMode int       // 0=no paths, 1=compact (~), 2=full paths
	commandOnExit   *exec.Cmd // Command to run after TUI exits
	// Change tracking
	changesMade bool
	// Save to group state
	saveToGroupMode    bool     // Whether we're in save-to-group mode
	saveToGroupOptions []string // List of group options (existing + "New group...")
	saveToGroupCursor  int      // Current selection in dropdown
	saveToGroupNewMode bool     // Whether we're typing a new group name
	saveToGroupInput   string   // Text input for new group name
	// Confirmation state
	confirmMode   string // "load", "clear", or "" for none
	confirmSource string // Source group name for load confirmation
	// Load from group state (when pressing L on default)
	loadFromGroupMode    bool
	loadFromGroupOptions []string
	loadFromGroupCursor  int
	// Group Creation state
	newGroupMode   bool
	newGroupStep   int
	newGroupName   string
	newGroupPrefix string
	// Default locked sessions (shared across all groups)
	defaultLockedSessions map[string]models.TmuxSession
	// Handoff to other TUIs
	nextCommand string // Command to run after TUI exits (e.g., "groups")
}

// Key bindings are defined in pkg/keymap/manage.go and re-exported via tui_keymap.go

func newManageModel(sessions []models.TmuxSession, mgr *tmux.Manager, cwdPath string, cachedEnrichedProjects map[string]*manager.SessionizeProject, usedCache bool) manageModel {
	helpModel := help.NewBuilder().
		WithKeys(manageKeys).
		WithTitle("Session Key Manager - Help").
		Build()

	// Use cached enriched projects if provided, otherwise start with empty map
	enrichedProjects := cachedEnrichedProjects
	if enrichedProjects == nil {
		enrichedProjects = make(map[string]*manager.SessionizeProject)
	}

	// Load locked keys from manager
	lockedKeysSlice := mgr.GetLockedKeys()
	lockedKeysMap := make(map[string]bool)
	for _, key := range lockedKeysSlice {
		lockedKeysMap[key] = true
	}

	// Load default's sessions for locked keys (shared across all groups)
	currentGroup := mgr.GetActiveGroup()
	mgr.SetActiveGroup("default")
	defaultSessions, _ := mgr.GetSessions()
	defaultLockedSessions := make(map[string]models.TmuxSession)
	for _, s := range defaultSessions {
		if lockedKeysMap[s.Key] {
			defaultLockedSessions[s.Key] = s
		}
	}
	mgr.SetActiveGroup(currentGroup) // Restore original group

	return manageModel{
		cursor:                0,
		sessions:              sessions,
		manager:               mgr,
		keys:                  manageKeys,
		help:                  helpModel,
		cwdPath:               cwdPath,
		enrichedProjects:      enrichedProjects,
		lockedKeys:            lockedKeysMap,
		usedCache:             usedCache,
		isLoading:             usedCache, // Start as loading if we used cache
		enrichmentLoading:     make(map[string]bool),
		pathDisplayMode:       0, // Default to no paths
		defaultLockedSessions: defaultLockedSessions,
	}
}

func (m *manageModel) Init() tea.Cmd {
	// Ensure sessions are ordered with locked keys at bottom
	m.rebuildSessionsOrder()

	cmds := []tea.Cmd{
		enrichInitialProjectsCmd(m.sessions, m.enrichedProjects),
		enrichCwdProjectCmd(m.cwdPath),
	}

	// Start spinner animation if loading
	if m.isLoading {
		cmds = append(cmds, spinnerTickCmd())
	}

	return tea.Batch(cmds...)
}

// fetchAllGitStatusesForKeyManageCmd returns a command to fetch git status for multiple paths concurrently.
// Projects that already have GitStatus pre-populated (from daemon) are skipped.
func fetchAllGitStatusesForKeyManageCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := make(map[string]*git.ExtendedGitStatus)
		semaphore := make(chan struct{}, 10) // Limit to 10 concurrent git processes

		for _, p := range projects {
			// Skip projects that already have git status from daemon
			if p.GitStatus != nil {
				mu.Lock()
				statuses[p.Path] = p.GitStatus
				mu.Unlock()
				continue
			}

			wg.Add(1)
			go func(proj *manager.SessionizeProject) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				status, err := git.GetExtendedStatus(proj.Path)
				if err == nil {
					mu.Lock()
					statuses[proj.Path] = status
					mu.Unlock()
				}
			}(p)
		}

		wg.Wait()
		return gitStatusMapMsg{statuses: statuses}
	}
}

// fetchAllNoteCountsForKeyManageCmd returns a command to fetch all note counts.
func fetchAllNoteCountsForKeyManageCmd() tea.Cmd {
	return func() tea.Msg {
		counts, _ := manager.FetchNoteCountsMap()
		return noteCountsMapMsg{counts: counts}
	}
}

// fetchAllPlanStatsForKeyManageCmd returns a command to fetch all plan stats.
func fetchAllPlanStatsForKeyManageCmd() tea.Cmd {
	return func() tea.Msg {
		stats, _ := manager.FetchPlanStatsMap()
		return planStatsMapMsg{stats: stats}
	}
}

// enrichCwdProjectCmd fetches and enriches the CWD project
func enrichCwdProjectCmd(cwdPath string) tea.Cmd {
	return func() tea.Msg {
		// Get project info for CWD
		node, err := workspace.GetProjectByPath(cwdPath)
		if err != nil {
			// CWD is not a valid project
			return cwdProjectEnrichedMsg{project: nil}
		}

		// Wrap in SessionizeProject (enrichment happens async in the TUI)
		return cwdProjectEnrichedMsg{project: &manager.SessionizeProject{
			WorkspaceNode: node,
		}}
	}
}

func (m *manageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initialProjectsEnrichedMsg:
		m.enrichedProjects = msg.enrichedProjects
		m.isLoading = false // Initial project identification is done

		var cmds []tea.Cmd
		cmds = append(cmds, fetchAllGitStatusesForKeyManageCmd(msg.projectList))
		cmds = append(cmds, fetchAllNoteCountsForKeyManageCmd())
		cmds = append(cmds, fetchAllPlanStatsForKeyManageCmd())
		cmds = append(cmds, fetchRulesStateForKeyManageCmd(msg.projectList))

		m.enrichmentLoading["git"] = true
		m.enrichmentLoading["notes"] = true
		m.enrichmentLoading["plans"] = true

		cmds = append(cmds, spinnerTickCmd())

		// Save to cache
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)

		return m, tea.Batch(cmds...)

	case rulesStateMsg:
		for path, status := range msg.rulesState {
			if proj, ok := m.enrichedProjects[path]; ok {
				switch status {
				case grovecontext.RuleHot:
					proj.ContextStatus = "H"
				case grovecontext.RuleCold:
					proj.ContextStatus = "C"
				case grovecontext.RuleExcluded:
					proj.ContextStatus = "X"
				default:
					proj.ContextStatus = ""
				}
			}
		}
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)
		return m, nil

	case cwdProjectEnrichedMsg:
		m.cwdProject = msg.project
		// Enrich the CWD project immediately
		if m.cwdProject != nil {
			go func() {
				opts := &manager.EnrichmentOptions{
					FetchGitStatus:  true,
					FetchNoteCounts: true,
					FetchPlanStats:  true,
				}
				manager.EnrichProjects(context.Background(), []*manager.SessionizeProject{m.cwdProject}, opts)
			}()
		}
		return m, nil

	case gitStatusMapMsg:
		for path, status := range msg.statuses {
			if proj, ok := m.enrichedProjects[path]; ok {
				proj.GitStatus = status
			}
		}
		m.enrichmentLoading["git"] = false
		m.isLoading = false // Mark initial loading as done
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)
		return m, nil

	case noteCountsMapMsg:
		for _, proj := range m.enrichedProjects {
			if counts, ok := msg.counts[proj.Name]; ok {
				proj.NoteCounts = counts
			}
		}
		m.enrichmentLoading["notes"] = false
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)
		return m, nil

	case planStatsMapMsg:
		for _, proj := range m.enrichedProjects {
			if stats, ok := msg.stats[proj.Path]; ok {
				proj.PlanStats = stats
			}
		}
		m.enrichmentLoading["plans"] = false
		_ = manager.SaveKeyManageCache(configDir, m.enrichedProjects)
		return m, nil

	case spinnerTickMsg:
		// Update spinner animation frame
		anyLoading := m.isLoading
		for _, loading := range m.enrichmentLoading {
			if loading {
				anyLoading = true
				break
			}
		}
		if anyLoading {
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

		// Handle confirmation mode
		if m.confirmMode != "" {
			switch {
			case msg.Type == tea.KeyEsc, msg.String() == "n", msg.String() == "N":
				m.confirmMode = ""
				m.confirmSource = ""
				m.message = "Cancelled"
				return m, nil

			case msg.String() == "y", msg.String() == "Y":
				switch m.confirmMode {
				case "load":
					m.executeLoadIntoDefault(m.confirmSource)
				case "clear":
					m.executeClearGroup()
				case "delete_group":
					groupToDelete := m.manager.GetActiveGroup()
					if err := m.manager.DeleteGroup(groupToDelete); err != nil {
						m.message = fmt.Sprintf("Error deleting group: %v", err)
					} else {
						m.manager.SetActiveGroup("default")
						_ = m.manager.SetLastAccessedGroup("default")
						m.sessions, _ = m.manager.GetSessions()
						m.rebuildSessionsOrder()
						m.message = fmt.Sprintf("Deleted group '%s'", groupToDelete)
					}
				}
				m.confirmMode = ""
				m.confirmSource = ""
				return m, nil
			}
			return m, nil
		}

		// Handle new group mode
		if m.newGroupMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.newGroupMode = false
				m.message = "New group cancelled"
				return m, nil
			case tea.KeyEnter:
				if m.newGroupStep == 0 {
					if m.newGroupName == "" {
						m.message = "Group name cannot be empty"
						return m, nil
					}
					m.newGroupStep = 1
					m.message = ""
				} else {
					if err := m.manager.CreateGroup(m.newGroupName, m.newGroupPrefix); err != nil {
						m.message = fmt.Sprintf("Error creating group: %v", err)
					} else {
						m.manager.SetActiveGroup(m.newGroupName)
						_ = m.manager.SetLastAccessedGroup(m.newGroupName)
						m.sessions, _ = m.manager.GetSessions()
						m.rebuildSessionsOrder()
						m.message = fmt.Sprintf("Created and switched to group '%s'", m.newGroupName)
					}
					m.newGroupMode = false
				}
				return m, nil
			case tea.KeyBackspace:
				if m.newGroupStep == 0 {
					if len(m.newGroupName) > 0 {
						m.newGroupName = m.newGroupName[:len(m.newGroupName)-1]
					}
				} else {
					if len(m.newGroupPrefix) > 0 {
						m.newGroupPrefix = m.newGroupPrefix[:len(m.newGroupPrefix)-1]
					}
				}
				return m, nil
			case tea.KeySpace:
				if m.newGroupStep == 1 {
					m.newGroupPrefix += " "
				}
				return m, nil
			case tea.KeyRunes:
				if m.newGroupStep == 0 {
					for _, r := range msg.Runes {
						if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
							m.newGroupName += string(r)
						}
					}
				} else {
					m.newGroupPrefix += string(msg.Runes)
				}
				return m, nil
			}
			return m, nil
		}

		// Handle load from group mode
		if m.loadFromGroupMode {
			switch {
			case msg.Type == tea.KeyEsc:
				m.loadFromGroupMode = false
				m.message = "Load from group cancelled"
				return m, nil

			case key.Matches(msg, m.keys.Up):
				if m.loadFromGroupCursor > 0 {
					m.loadFromGroupCursor--
				}
				return m, nil

			case key.Matches(msg, m.keys.Down):
				if m.loadFromGroupCursor < len(m.loadFromGroupOptions)-1 {
					m.loadFromGroupCursor++
				}
				return m, nil

			case msg.Type == tea.KeyEnter:
				selected := m.loadFromGroupOptions[m.loadFromGroupCursor]
				m.loadFromGroupMode = false
				if m.manager.ConfirmKeyUpdates() {
					// Enter confirmation mode
					m.confirmMode = "load"
					m.confirmSource = selected
					m.message = fmt.Sprintf("Load '%s' into default? This will replace non-locked mappings.", selected)
				} else {
					// Execute immediately without confirmation
					m.executeLoadIntoDefault(selected)
				}
				return m, nil
			}
			return m, nil
		}

		// Handle save to group mode
		if m.saveToGroupMode {
			switch {
			case msg.Type == tea.KeyEsc:
				m.saveToGroupMode = false
				m.saveToGroupNewMode = false
				m.saveToGroupInput = ""
				m.message = "Save to group cancelled"
				return m, nil

			case m.saveToGroupNewMode:
				// Text input mode for new group name
				switch msg.Type {
				case tea.KeyEnter:
					if m.saveToGroupInput != "" {
						m.saveDefaultToGroup(m.saveToGroupInput)
					} else {
						m.message = "Group name cannot be empty"
					}
					m.saveToGroupMode = false
					m.saveToGroupNewMode = false
					m.saveToGroupInput = ""
					return m, nil
				case tea.KeyBackspace:
					if len(m.saveToGroupInput) > 0 {
						m.saveToGroupInput = m.saveToGroupInput[:len(m.saveToGroupInput)-1]
					}
					return m, nil
				case tea.KeyRunes:
					// Only allow alphanumeric and hyphens/underscores
					for _, r := range msg.Runes {
						if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
							m.saveToGroupInput += string(r)
						}
					}
					return m, nil
				}
				return m, nil

			case key.Matches(msg, m.keys.Up):
				if m.saveToGroupCursor > 0 {
					m.saveToGroupCursor--
				}
				return m, nil

			case key.Matches(msg, m.keys.Down):
				if m.saveToGroupCursor < len(m.saveToGroupOptions)-1 {
					m.saveToGroupCursor++
				}
				return m, nil

			case msg.Type == tea.KeyEnter:
				selected := m.saveToGroupOptions[m.saveToGroupCursor]
				if selected == "+ New group..." {
					m.saveToGroupNewMode = true
					m.saveToGroupInput = ""
					m.message = "Enter new group name:"
				} else {
					m.saveDefaultToGroup(selected)
					m.saveToGroupMode = false
				}
				return m, nil
			}
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
								if err := client.SwitchClientToSession(ctx, sessionName); err != nil {
									m.message = fmt.Sprintf("Failed to switch to session: %v", err)
								} else {
									// Record project access for history
									_ = m.manager.RecordProjectAccess(session.Path)
									// Exit the manager after switching
									m.message = fmt.Sprintf("Switching to %s...", sessionName)
									m.quitting = true
									m.commandOnExit = client.ClosePopupCmd()
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
				// Toggle lock for current key (only in default group)
				if m.manager.GetActiveGroup() != "default" {
					m.message = "Locked keys can only be modified in default group"
					return m, nil
				}
				if m.cursor < len(m.sessions) {
					currentKey := m.sessions[m.cursor].Key
					currentSession := m.sessions[m.cursor]
					if m.lockedKeys[currentKey] {
						delete(m.lockedKeys, currentKey)
						delete(m.defaultLockedSessions, currentKey)
						m.message = fmt.Sprintf("Unlocked key '%s'", currentKey)
					} else {
						m.lockedKeys[currentKey] = true
						m.defaultLockedSessions[currentKey] = currentSession
						m.message = fmt.Sprintf("Locked key '%s'", currentKey)
					}
					// Rebuild order to move locked keys to bottom
					m.rebuildSessionsOrder()
					m.saveChanges()
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
						// Swap only path-related fields, keep keys fixed
						currentPath := m.sessions[m.cursor].Path
						currentRepo := m.sessions[m.cursor].Repository
						currentDesc := m.sessions[m.cursor].Description

						m.sessions[m.cursor].Path = m.sessions[targetPos].Path
						m.sessions[m.cursor].Repository = m.sessions[targetPos].Repository
						m.sessions[m.cursor].Description = m.sessions[targetPos].Description

						m.sessions[targetPos].Path = currentPath
						m.sessions[targetPos].Repository = currentRepo
						m.sessions[targetPos].Description = currentDesc

						// Move cursor with the row
						m.cursor = targetPos
						m.message = "Moved up"
						m.saveChanges()
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
						// Swap only path-related fields, keep keys fixed
						currentPath := m.sessions[m.cursor].Path
						currentRepo := m.sessions[m.cursor].Repository
						currentDesc := m.sessions[m.cursor].Description

						m.sessions[m.cursor].Path = m.sessions[targetPos].Path
						m.sessions[m.cursor].Repository = m.sessions[targetPos].Repository
						m.sessions[m.cursor].Description = m.sessions[targetPos].Description

						m.sessions[targetPos].Path = currentPath
						m.sessions[targetPos].Repository = currentRepo
						m.sessions[targetPos].Description = currentDesc

						// Move cursor with the row
						m.cursor = targetPos
						m.message = "Moved down"
						m.saveChanges()
					} else {
						m.message = "Cannot move past locked keys"
					}
				}
				return m, nil

			case key.Matches(msg, m.keys.ConfirmMove):
				// Save and exit move mode
				if err := m.manager.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice()); err != nil {
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
			// Toggle lock (only in default group)
			if m.manager.GetActiveGroup() != "default" {
				m.message = "Locked keys can only be modified in default group"
				return m, nil
			}
			if m.cursor < len(m.sessions) {
				currentKey := m.sessions[m.cursor].Key
				currentSession := m.sessions[m.cursor]
				if m.lockedKeys[currentKey] {
					delete(m.lockedKeys, currentKey)
					delete(m.defaultLockedSessions, currentKey)
					m.message = fmt.Sprintf("Unlocked key '%s'", currentKey)
				} else {
					m.lockedKeys[currentKey] = true
					m.defaultLockedSessions[currentKey] = currentSession
					m.message = fmt.Sprintf("Locked key '%s'", currentKey)
				}
				// Rebuild order to move locked keys to bottom
				m.rebuildSessionsOrder()
				m.saveChanges()
			}
			return m, nil

		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.TogglePaths):
			// Toggle paths display mode (0 -> 1 -> 2 -> 0)
			m.pathDisplayMode = (m.pathDisplayMode + 1) % 3
			return m, nil

		case key.Matches(msg, m.keys.NextGroup):
			m.cycleGroup(1)
			return m, nil

		case key.Matches(msg, m.keys.PrevGroup):
			m.cycleGroup(-1)
			return m, nil

		case key.Matches(msg, m.keys.LoadDefault):
			if m.manager.GetActiveGroup() == "default" {
				// Show picker to choose which group to load from
				m.loadFromGroupOptions = []string{}
				for _, g := range m.manager.GetGroups() {
					if g != "default" {
						m.loadFromGroupOptions = append(m.loadFromGroupOptions, g)
					}
				}
				if len(m.loadFromGroupOptions) == 0 {
					m.message = "No other groups to load from"
					return m, nil
				}
				m.loadFromGroupMode = true
				m.loadFromGroupCursor = 0
				m.message = "Select group to load from (↑/↓ to navigate, Enter to select, Esc to cancel)"
				return m, nil
			}
			sourceGroup := m.manager.GetActiveGroup()
			if m.manager.ConfirmKeyUpdates() {
				// Enter confirmation mode
				m.confirmMode = "load"
				m.confirmSource = sourceGroup
				m.message = fmt.Sprintf("Load '%s' into default? This will replace non-locked mappings.", sourceGroup)
			} else {
				// Execute immediately without confirmation
				m.executeLoadIntoDefault(sourceGroup)
			}
			return m, nil

		case key.Matches(msg, m.keys.UnloadDefault):
			// Check if there's anything to clear
			hasNonLocked := false
			for _, s := range m.sessions {
				if s.Path != "" && !m.lockedKeys[s.Key] {
					hasNonLocked = true
					break
				}
			}
			if !hasNonLocked {
				m.message = "No non-locked mappings to clear"
				return m, nil
			}
			if m.manager.ConfirmKeyUpdates() {
				// Enter confirmation mode
				m.confirmMode = "clear"
				m.message = fmt.Sprintf("Clear all non-locked mappings from '%s'?", m.manager.GetActiveGroup())
			} else {
				// Execute immediately without confirmation
				m.executeClearGroup()
			}
			return m, nil

		case key.Matches(msg, m.keys.NewGroup):
			m.newGroupMode = true
			m.newGroupStep = 0
			m.newGroupName = ""
			m.newGroupPrefix = ""
			m.message = "Enter new group name:"
			return m, nil

		case key.Matches(msg, m.keys.Groups):
			// Hand off to groups TUI
			m.nextCommand = "groups"
			return m, tea.Quit

		case key.Matches(msg, m.keys.DeleteGroup):
			if m.manager.GetActiveGroup() == "default" {
				m.message = "Cannot delete default group"
				return m, nil
			}
			m.confirmMode = "delete_group"
			m.message = fmt.Sprintf("Delete group '%s'? All mappings will be lost.", m.manager.GetActiveGroup())
			return m, nil

		case key.Matches(msg, m.keys.SaveToGroup):
			// Check if there are any mappings to save
			hasMappings := false
			for _, s := range m.sessions {
				if s.Path != "" {
					hasMappings = true
					break
				}
			}
			if !hasMappings {
				m.message = "No mappings to save"
				return m, nil
			}

			// Build list of existing groups (excluding current group and default)
			m.saveToGroupOptions = []string{}
			currentGroup := m.manager.GetActiveGroup()
			for _, g := range m.manager.GetGroups() {
				if g != currentGroup && g != "default" {
					m.saveToGroupOptions = append(m.saveToGroupOptions, g)
				}
			}
			// Add option to create new group
			m.saveToGroupOptions = append(m.saveToGroupOptions, "+ New group...")

			m.saveToGroupMode = true
			m.saveToGroupCursor = 0
			m.saveToGroupNewMode = false
			m.saveToGroupInput = ""
			m.message = "Select group to save to (↑/↓ to navigate, Enter to select, Esc to cancel)"
			return m, nil

		case key.Matches(msg, m.keys.Quit):
			// Just quit - save happens after TUI exits
			return m, tea.Quit

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
			cwdNormalizedPath, err := pathutil.NormalizeForLookup(m.cwdProject.Path)
			if err != nil {
				m.message = "Failed to normalize CWD path"
				return m, nil
			}
			for _, s := range m.sessions {
				if s.Path != "" {
					sNormalizedPath, err := pathutil.NormalizeForLookup(s.Path)
					if err != nil {
						continue
					}
					if sNormalizedPath == cwdNormalizedPath {
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
			m.enrichedProjects[filepath.Clean(m.cwdProject.Path)] = m.cwdProject

			m.message = fmt.Sprintf("Mapped key '%s' to '%s'", session.Key, m.cwdProject.Name)
			m.saveChanges()
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
						if err := client.SwitchClientToSession(ctx, sessionName); err != nil {
							m.message = fmt.Sprintf("Failed to switch to session: %v", err)
						} else {
							// Record project access for history
							_ = m.manager.RecordProjectAccess(session.Path)
							// Exit the manager after switching
							m.message = fmt.Sprintf("Switching to %s...", sessionName)
							m.quitting = true
							m.commandOnExit = client.ClosePopupCmd()
							return m, tea.Quit
						}
					} else {
						m.message = "Not in a tmux session"
					}
				} else {
					m.message = "No session mapped to this key"
				}
			}

		case key.Matches(msg, m.keys.CopyPath):
			// Copy the session path to clipboard
			if m.cursor < len(m.sessions) {
				session := m.sessions[m.cursor]
				if session.Path != "" {
					if err := clipboard.WriteAll(session.Path); err != nil {
						m.message = fmt.Sprintf("Error copying path: %v", err)
					} else {
						m.message = fmt.Sprintf("Copied: %s", session.Path)
					}
				} else {
					m.message = "No path mapped to this key"
				}
			}
			return m, nil

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
					m.saveChanges()
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
					m.saveChanges()
				}
			}

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keys.PageUp):
			// Move up by half page (5 rows)
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}

		case key.Matches(msg, m.keys.PageDown):
			// Move down by half page (5 rows)
			m.cursor += 5
			if m.cursor >= len(m.sessions) {
				m.cursor = len(m.sessions) - 1
			}

		case key.Matches(msg, m.keys.Top):
			m.cursor = 0

		case key.Matches(msg, m.keys.Bottom):
			m.cursor = len(m.sessions) - 1
		}
	}

	return m, nil
}

func (m *manageModel) View() string {
	if m.quitting && m.message != "" {
		return m.message + "\n"
	}

	// If help is visible, show it and return
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder

	// Title with key mapping
	prefix := m.manager.GetPrefix()
	var hotkey string
	switch prefix {
	case "<prefix>":
		hotkey = "C-b → key"
	case "<grove>":
		hotkey = "C-g → key"
	case "":
		hotkey = "direct"
	default:
		if strings.HasPrefix(prefix, "<prefix> ") {
			key := strings.TrimPrefix(prefix, "<prefix> ")
			hotkey = fmt.Sprintf("C-b %s → key", key)
		} else if strings.HasPrefix(prefix, "<grove> ") {
			key := strings.TrimPrefix(prefix, "<grove> ")
			hotkey = fmt.Sprintf("C-g %s → key", key)
		} else {
			hotkey = fmt.Sprintf("%s → key", prefix)
		}
	}

	title := fmt.Sprintf("%s Session Hotkeys (%s)", core_theme.IconKeyboard, hotkey)
	b.WriteString(core_theme.DefaultTheme.Header.Render(title))

	// Render group tabs if multiple groups exist
	groups := m.manager.GetGroups()
	if len(groups) > 1 {
		b.WriteString("\n")
		activeGroup := m.manager.GetActiveGroup()
		var tabs []string
		for _, g := range groups {
			iconStr := ""
			if g != "default" {
				if cfg, ok := m.manager.GetGroupConfig(g); ok && cfg.Icon != "" {
					iconStr = resolveIcon(cfg.Icon) + " "
				}
			}

			tabText := " " + iconStr + g + " "

			if g == activeGroup {
				// Active tab: highlighted with box characters
				tabs = append(tabs, core_theme.DefaultTheme.Selected.Render(tabText))
			} else {
				// Inactive tab: muted
				tabs = append(tabs, core_theme.DefaultTheme.Muted.Render(tabText))
			}
		}
		b.WriteString(strings.Join(tabs, core_theme.DefaultTheme.Muted.Render("│")))
	}

	// Show move mode indicator
	if m.moveMode {
		b.WriteString(" " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]"))
	}
	b.WriteString("\n")

	// Separate sessions into unlocked and locked
	// Locked sessions always use default's mappings (shared across all groups)
	var unlockedSessions []models.TmuxSession
	var lockedSessions []models.TmuxSession

	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
			// Use default's mapping for locked keys
			if defaultSession, ok := m.defaultLockedSessions[s.Key]; ok {
				lockedSessions = append(lockedSessions, defaultSession)
			} else {
				lockedSessions = append(lockedSessions, s)
			}
		} else {
			unlockedSessions = append(unlockedSessions, s)
		}
	}

	// Build table data with dynamic headers
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	gitHeader := "Git"
	if m.enrichmentLoading["git"] {
		gitHeader = "Git " + spinner
	}

	// Build headers based on path display mode
	headers := []string{"#", "Key", "Repository", "Branch/Worktree", gitHeader, "Ecosystem"}
	if m.pathDisplayMode > 0 {
		headers = append(headers, "Path")
	}

	var unlockedRows [][]string
	var lockedRows [][]string

	// Build unlocked rows
	for i, s := range unlockedSessions {
		var ecosystem, repository, worktree string
		gitStatus := ""

		if s.Path != "" {
			cleanPath := filepath.Clean(s.Path)
			if projInfo, found := m.enrichedProjects[cleanPath]; found {

				// RULE 1: Determine Repository and Worktree.
				// For a worktree, Repository is its parent. Otherwise, it's the project itself.
				if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
					// Get parent project info to determine its icon
					parentName := filepath.Base(projInfo.ParentProjectPath)
					parentIcon := core_theme.IconRepo // Default to repo icon

					// Try to find parent project to get its exact kind
					if parentProj, found := m.enrichedProjects[projInfo.ParentProjectPath]; found {
						if parentProj.Kind == workspace.KindEcosystemRoot {
							parentIcon = core_theme.IconEcosystem
						}
					}

					parentIconStyled := core_theme.DefaultTheme.Muted.Render(parentIcon + " ")
					repository = parentIconStyled + parentName

					// Determine icon for the worktree
					worktreeIcon := ""
					switch projInfo.Kind {
					case workspace.KindEcosystemWorktree:
						worktreeIcon = core_theme.IconWorktree // Use IconWorktree as IconEcosystemWorktree is not in core
					default:
						worktreeIcon = core_theme.IconWorktree
					}
					worktreeIconStyled := core_theme.DefaultTheme.Muted.Render(worktreeIcon + " ")
					worktree = worktreeIconStyled + projInfo.Name
				} else {
					// Determine icon for non-worktree
					icon := ""
					switch projInfo.Kind {
					case workspace.KindEcosystemRoot:
						icon = core_theme.IconEcosystem
					default:
						icon = core_theme.IconRepo
					}
					iconStyled := core_theme.DefaultTheme.Muted.Render(icon + " ")
					repository = iconStyled + projInfo.Name
				}

				// RULE 2: Determine Ecosystem display.
				if projInfo.ParentEcosystemPath != "" {
					// Project is within an ecosystem.
					// Use the root ecosystem for the Ecosystem column
					if projInfo.RootEcosystemPath != "" {
						ecosystem = filepath.Base(projInfo.RootEcosystemPath)
					} else {
						ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
					}

					// If the parent ecosystem path differs from the root, this is inside an ecosystem worktree
					if projInfo.RootEcosystemPath != "" && projInfo.ParentEcosystemPath != projInfo.RootEcosystemPath {
						// This project is inside an ecosystem worktree
						ecoWorktreeName := filepath.Base(projInfo.ParentEcosystemPath)

						// If this is also a worktree of that repo, show the eco worktree name in the Worktree column
						// Otherwise, show the ecosystem worktree name with indicator
						if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
							// This is a worktree of a repo inside an eco-worktree
							repository = filepath.Base(projInfo.ParentProjectPath)
							worktree = ecoWorktreeName
						} else {
							// This is a repo inside an eco-worktree (not a worktree itself)
							repository = projInfo.Name
							worktree = ecoWorktreeName + " *"
						}
					}
				} else if projInfo.IsEcosystem() {
					// It's a root ecosystem.
					ecosystem = projInfo.Name
				}

				// Format Git status (with colors)
				if projInfo.GitStatus != nil {
					gitStatus = formatChanges(projInfo.GitStatus.StatusInfo, projInfo.GitStatus)
				}

			} else {
				// Fallback if no enriched data
				repository = filepath.Base(s.Path)
			}

		}

		// Determine what to show in Branch/Worktree column
		branchWorktreeDisplay := worktree
		if branchWorktreeDisplay == "" && repository != "" {
			// This is a main repo (not a worktree), show branch name with icon
			if s.Path != "" {
				cleanPath := filepath.Clean(s.Path)
				if projInfo, found := m.enrichedProjects[cleanPath]; found {
					if projInfo.GitStatus != nil && projInfo.GitStatus.StatusInfo != nil && projInfo.GitStatus.StatusInfo.Branch != "" {
						// Add branch icon
						branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
						branchWorktreeDisplay = branchIcon + projInfo.GitStatus.StatusInfo.Branch
					} else {
						branchWorktreeDisplay = dimStyle.Render("n/a")
					}
				} else {
					branchWorktreeDisplay = dimStyle.Render("n/a")
				}
			} else {
				branchWorktreeDisplay = dimStyle.Render("n/a")
			}
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			s.Key,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}
		// Add path column if enabled
		if m.pathDisplayMode > 0 {
			pathStr := ""
			if s.Path != "" {
				if m.pathDisplayMode == 1 {
					// Compact mode: replace home with ~
					pathStr = strings.Replace(s.Path, os.Getenv("HOME"), "~", 1)
				} else {
					// Full path mode
					pathStr = s.Path
				}
			}
			row = append(row, pathStr)
		}
		unlockedRows = append(unlockedRows, row)
	}

	// Build locked rows
	for i, s := range lockedSessions {
		var ecosystem, repository, worktree string
		gitStatus := ""

		if s.Path != "" {
			cleanPath := filepath.Clean(s.Path)
			if projInfo, found := m.enrichedProjects[cleanPath]; found {

				// RULE 1: Determine Repository and Worktree.
				// For a worktree, Repository is its parent. Otherwise, it's the project itself.
				if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
					// Get parent project info to determine its icon
					parentName := filepath.Base(projInfo.ParentProjectPath)
					parentIcon := core_theme.IconRepo // Default to repo icon

					// Try to find parent project to get its exact kind
					if parentProj, found := m.enrichedProjects[projInfo.ParentProjectPath]; found {
						if parentProj.Kind == workspace.KindEcosystemRoot {
							parentIcon = core_theme.IconEcosystem
						}
					}

					parentIconStyled := core_theme.DefaultTheme.Muted.Render(parentIcon + " ")
					repository = parentIconStyled + parentName

					// Determine icon for the worktree
					worktreeIcon := ""
					switch projInfo.Kind {
					case workspace.KindEcosystemWorktree:
						worktreeIcon = core_theme.IconWorktree // Use IconWorktree as IconEcosystemWorktree is not in core
					default:
						worktreeIcon = core_theme.IconWorktree
					}
					worktreeIconStyled := core_theme.DefaultTheme.Muted.Render(worktreeIcon + " ")
					worktree = worktreeIconStyled + projInfo.Name
				} else {
					// Determine icon for non-worktree
					icon := ""
					switch projInfo.Kind {
					case workspace.KindEcosystemRoot:
						icon = core_theme.IconEcosystem
					default:
						icon = core_theme.IconRepo
					}
					iconStyled := core_theme.DefaultTheme.Muted.Render(icon + " ")
					repository = iconStyled + projInfo.Name
				}

				// RULE 2: Determine Ecosystem display.
				if projInfo.ParentEcosystemPath != "" {
					// Project is within an ecosystem.
					// Use the root ecosystem for the Ecosystem column
					if projInfo.RootEcosystemPath != "" {
						ecosystem = filepath.Base(projInfo.RootEcosystemPath)
					} else {
						ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
					}

					// If the parent ecosystem path differs from the root, this is inside an ecosystem worktree
					if projInfo.RootEcosystemPath != "" && projInfo.ParentEcosystemPath != projInfo.RootEcosystemPath {
						// This project is inside an ecosystem worktree
						ecoWorktreeName := filepath.Base(projInfo.ParentEcosystemPath)

						// If this is also a worktree of that repo, show the eco worktree name in the Worktree column
						// Otherwise, show the ecosystem worktree name with indicator
						if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
							// This is a worktree of a repo inside an eco-worktree
							repository = filepath.Base(projInfo.ParentProjectPath)
							worktree = ecoWorktreeName
						} else {
							// This is a repo inside an eco-worktree (not a worktree itself)
							repository = projInfo.Name
							worktree = ecoWorktreeName + " *"
						}
					}
				} else if projInfo.IsEcosystem() {
					// It's a root ecosystem.
					ecosystem = projInfo.Name
				}

				// Format Git status (with colors)
				if projInfo.GitStatus != nil {
					gitStatus = formatChanges(projInfo.GitStatus.StatusInfo, projInfo.GitStatus)
				}

			} else {
				// Fallback if no enriched data
				repository = filepath.Base(s.Path)
			}

		}

		// Determine what to show in Branch/Worktree column
		branchWorktreeDisplay := worktree
		if branchWorktreeDisplay == "" && repository != "" {
			// This is a main repo (not a worktree), show branch name with icon
			if s.Path != "" {
				cleanPath := filepath.Clean(s.Path)
				if projInfo, found := m.enrichedProjects[cleanPath]; found {
					if projInfo.GitStatus != nil && projInfo.GitStatus.StatusInfo != nil && projInfo.GitStatus.StatusInfo.Branch != "" {
						// Add branch icon
						branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
						branchWorktreeDisplay = branchIcon + projInfo.GitStatus.StatusInfo.Branch
					} else {
						branchWorktreeDisplay = dimStyle.Render("n/a")
					}
				} else {
					branchWorktreeDisplay = dimStyle.Render("n/a")
				}
			} else {
				branchWorktreeDisplay = dimStyle.Render("n/a")
			}
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			s.Key,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}
		// Add path column if enabled
		if m.pathDisplayMode > 0 {
			pathStr := ""
			if s.Path != "" {
				if m.pathDisplayMode == 1 {
					// Compact mode: replace home with ~
					pathStr = strings.Replace(s.Path, os.Getenv("HOME"), "~", 1)
				} else {
					// Full path mode
					pathStr = s.Path
				}
			}
			row = append(row, pathStr)
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
			unlockedTableStr = table.SelectableTableWithOptions(headers, unlockedRows, adjustedCursor, table.SelectableTableOptions{})
		} else {
			// No selection in this table
			unlockedTableStr = table.SelectableTableWithOptions(headers, unlockedRows, -1, table.SelectableTableOptions{})
		}
		b.WriteString(unlockedTableStr)
		b.WriteString("\n")
	}

	// Render locked section if there are locked keys
	if len(lockedRows) > 0 {
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Muted.Render(core_theme.IconLock + " Locked"))
		b.WriteString("\n")

		var lockedTableStr string
		if !cursorInUnlocked {
			lockedTableStr = table.SelectableTableWithOptions(headers, lockedRows, adjustedCursor, table.SelectableTableOptions{})
		} else {
			lockedTableStr = table.SelectableTableWithOptions(headers, lockedRows, -1, table.SelectableTableOptions{})
		}
		b.WriteString(lockedTableStr)
	}

	b.WriteString("\n\n")

	// Show confirmation dialog if active
	if m.confirmMode != "" {
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Warning.Render("  ⚠ " + m.message))
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Success.Render("    [Y]es") + "  " + core_theme.DefaultTheme.Error.Render("[N]o / Esc"))
		b.WriteString("\n\n")
	}

	// Show load-from-group dropdown if active
	if m.loadFromGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Load from Group") + "\n")
		for i, opt := range m.loadFromGroupOptions {
			if i == m.loadFromGroupCursor {
				b.WriteString(core_theme.DefaultTheme.Selected.Render("  → " + opt))
			} else {
				b.WriteString(dimStyle.Render("    " + opt))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Show new group prompt if active
	if m.newGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Create New Group") + "\n")
		if m.newGroupStep == 0 {
			b.WriteString("  New group name: ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.newGroupName + "█"))
		} else {
			b.WriteString(fmt.Sprintf("  Group name: %s\n", m.newGroupName))
			b.WriteString("  Prefix key (optional): ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.newGroupPrefix + "█"))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("  e.g. '<grove> g' → C-g g key"))
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	}

	// Show save-to-group dropdown if active
	if m.saveToGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Save to Group") + "\n")
		if m.saveToGroupNewMode {
			// Show text input for new group name
			b.WriteString("  New group name: ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.saveToGroupInput + "█"))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("  (Enter to confirm, Esc to cancel)"))
		} else {
			// Show dropdown options
			for i, opt := range m.saveToGroupOptions {
				if i == m.saveToGroupCursor {
					b.WriteString(core_theme.DefaultTheme.Selected.Render("  → " + opt))
				} else {
					b.WriteString(dimStyle.Render("    " + opt))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Always reserve space for message to prevent layout shift
	// Skip if in confirm mode (message shown in confirmation box)
	if m.confirmMode == "" {
		if m.message != "" {
			b.WriteString(dimStyle.Render(m.message) + "\n")
		} else {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("\n")
	}

	// Show different help text based on mode
	var modeIndicator string
	if m.moveMode {
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [MOVE MODE]")
	} else if m.setKeyMode {
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [SET KEY MODE]")
	} else if m.saveToGroupMode {
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [SAVE TO GROUP]")
	} else if m.loadFromGroupMode {
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [LOAD FROM GROUP]")
	} else if m.confirmMode != "" {
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [CONFIRM]")
	}
	b.WriteString(m.help.View() + modeIndicator)

	return pageStyle.Render(b.String())
}

// rebuildSessionsOrder ensures locked keys are always at the bottom
// Locked keys use default's mappings (shared across all groups)
func (m *manageModel) rebuildSessionsOrder() {
	var unlocked []models.TmuxSession
	var locked []models.TmuxSession

	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
			// Use default's mapping for locked keys
			if defaultSession, ok := m.defaultLockedSessions[s.Key]; ok {
				locked = append(locked, defaultSession)
			} else {
				locked = append(locked, s)
			}
		} else {
			unlocked = append(unlocked, s)
		}
	}

	m.sessions = append(unlocked, locked...)
}

// cycleGroup switches to the next or previous workspace group
func (m *manageModel) cycleGroup(dir int) {
	// Save changes for current group before switching
	if m.changesMade {
		_ = m.manager.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice())
	}

	groups := m.manager.GetGroups()
	if len(groups) <= 1 {
		m.message = "No other groups configured"
		return
	}

	currentIdx := 0
	for i, g := range groups {
		if g == m.manager.GetActiveGroup() {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + dir) % len(groups)
	if nextIdx < 0 {
		nextIdx = len(groups) - 1
	}

	newGroup := groups[nextIdx]
	m.manager.SetActiveGroup(newGroup)
	_ = m.manager.SetLastAccessedGroup(newGroup)

	// Reload sessions for the new group
	m.sessions, _ = m.manager.GetSessions()
	lockedKeysSlice := m.manager.GetLockedKeys()
	m.lockedKeys = make(map[string]bool)
	for _, key := range lockedKeysSlice {
		m.lockedKeys[key] = true
	}
	m.cursor = 0
	m.rebuildSessionsOrder()
	m.changesMade = false
	m.message = fmt.Sprintf("Switched to group: %s", newGroup)
}

// getLockedKeysSlice converts the locked keys map to a slice
func (m *manageModel) getLockedKeysSlice() []string {
	lockedKeys := make([]string, 0, len(m.lockedKeys))
	for key := range m.lockedKeys {
		lockedKeys = append(lockedKeys, key)
	}
	return lockedKeys
}

// saveChanges persists the current session state immediately.
// Called after each change to ensure data is never lost.
func (m *manageModel) saveChanges() {
	if err := m.manager.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error saving: %v", err)
		return
	}
	// Regenerate tmux bindings so changes take effect immediately
	_ = m.manager.RegenerateBindings()
	m.changesMade = false
}

// mapKeyToCwd maps the CWD to the target key index
func (m *manageModel) mapKeyToCwd(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(m.sessions) {
		return
	}

	targetSession := &m.sessions[targetIndex]
	cwdNormalizedPath, err := pathutil.NormalizeForLookup(m.cwdProject.Path)
	if err != nil {
		m.message = "Failed to normalize CWD path"
		m.setKeyMode = false
		return
	}

	// Find and clear any pre-existing mapping for the CWD path
	for i := range m.sessions {
		if m.sessions[i].Path != "" {
			sessionNormalizedPath, err := pathutil.NormalizeForLookup(m.sessions[i].Path)
			if err != nil {
				continue
			}
			if sessionNormalizedPath == cwdNormalizedPath {
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
	m.enrichedProjects[filepath.Clean(m.cwdProject.Path)] = m.cwdProject

	// Exit set key mode
	m.setKeyMode = false

	// Set success message
	m.message = fmt.Sprintf("Mapped key '%s' to '%s'", targetSession.Key, m.cwdProject.Name)
	m.saveChanges()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// executeLoadIntoDefault performs the actual load operation after confirmation
func (m *manageModel) executeLoadIntoDefault(sourceGroup string) {
	// Get source sessions
	m.manager.SetActiveGroup(sourceGroup)
	sourceSessions, _ := m.manager.GetSessions()

	// Switch to default
	m.manager.SetActiveGroup("default")
	m.sessions, _ = m.manager.GetSessions()

	// Rebuild locked state
	lockedKeys := m.manager.GetLockedKeys()
	m.lockedKeys = make(map[string]bool)
	for _, k := range lockedKeys {
		m.lockedKeys[k] = true
	}

	// Replace non-locked default keys with source keys
	for i := range m.sessions {
		if !m.lockedKeys[m.sessions[i].Key] {
			m.sessions[i].Path = ""
			m.sessions[i].Repository = ""
			m.sessions[i].Description = ""

			for _, src := range sourceSessions {
				if src.Key == m.sessions[i].Key && src.Path != "" {
					m.sessions[i].Path = src.Path
					m.sessions[i].Repository = src.Repository
					m.sessions[i].Description = src.Description
					break
				}
			}
		}
	}

	m.message = fmt.Sprintf("Loaded '%s' into default", sourceGroup)
	m.rebuildSessionsOrder()
	m.saveChanges()
}

// executeClearGroup performs the actual clear operation after confirmation
func (m *manageModel) executeClearGroup() {
	count := 0
	for i := range m.sessions {
		if m.sessions[i].Path != "" && !m.lockedKeys[m.sessions[i].Key] {
			m.sessions[i].Path = ""
			m.sessions[i].Repository = ""
			m.sessions[i].Description = ""
			count++
		}
	}

	groupName := m.manager.GetActiveGroup()
	m.message = fmt.Sprintf("Cleared %d mappings from '%s'", count, groupName)
	m.rebuildSessionsOrder()
	m.saveChanges()
}

// saveDefaultToGroup saves current mappings to a target group
func (m *manageModel) saveDefaultToGroup(targetGroup string) {
	// Copy current sessions
	sourceSessions := make([]models.TmuxSession, len(m.sessions))
	copy(sourceSessions, m.sessions)
	sourceGroup := m.manager.GetActiveGroup()

	// Create the group if it doesn't exist
	existingGroups := m.manager.GetAllGroups()
	groupExists := false
	for _, g := range existingGroups {
		if g == targetGroup {
			groupExists = true
			break
		}
	}
	if !groupExists {
		if err := m.manager.CreateGroup(targetGroup, ""); err != nil {
			m.message = fmt.Sprintf("Error creating group: %v", err)
			return
		}
	}

	// Switch to target group
	m.manager.SetActiveGroup(targetGroup)
	targetSessions, _ := m.manager.GetSessions()

	// Copy non-locked mappings from source to target
	count := 0
	for i := range targetSessions {
		// Skip locked keys in target
		if m.lockedKeys[targetSessions[i].Key] {
			continue
		}

		// Find matching key in source
		for _, src := range sourceSessions {
			if src.Key == targetSessions[i].Key && src.Path != "" {
				targetSessions[i].Path = src.Path
				targetSessions[i].Repository = src.Repository
				targetSessions[i].Description = src.Description
				count++
				break
			}
		}
	}

	// Save the target group
	if err := m.manager.UpdateSessionsAndLocks(targetSessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error saving to group: %v", err)
		m.manager.SetActiveGroup(sourceGroup)
		return
	}

	// Regenerate bindings
	if err := m.manager.RegenerateBindings(); err != nil {
		m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
	}

	// Stay on target group
	m.sessions = targetSessions
	m.cursor = 0
	m.rebuildSessionsOrder()
	m.message = fmt.Sprintf("Saved %d mappings to '%s'", count, targetGroup)
}

// formatPlanStatsForKeyManage formats plan stats into a styled string
// Shows only job status icons and counts (e.g., "◐ 1 ○ 2 ● 5")
func formatPlanStatsForKeyManage(stats *manager.PlanStats) string {
	if stats == nil || stats.TotalPlans == 0 {
		return ""
	}

	var jobStats []string
	if stats.Running > 0 {
		jobStats = append(jobStats, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s %d", core_theme.IconStatusRunning, stats.Running)))
	}
	if stats.Pending > 0 {
		jobStats = append(jobStats, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("%s %d", core_theme.IconStatusPendingUser, stats.Pending)))
	}
	if stats.Completed > 0 {
		jobStats = append(jobStats, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("%s %d", core_theme.IconStatusCompleted, stats.Completed)))
	}
	if stats.Failed > 0 {
		jobStats = append(jobStats, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("%s %d", core_theme.IconStatusFailed, stats.Failed)))
	}

	return strings.Join(jobStats, " ")
}

// formatGitStatusPlain formats Git status without ANSI codes for table display
func formatGitStatusPlain(status *git.StatusInfo, extStatus *git.ExtendedGitStatus) string {
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
			return core_theme.IconSuccess
		} else {
			return core_theme.IconStatusTodo
		}
	}

	return changesStr
}

// reloadTmuxConfig reloads the tmux configuration
func reloadTmuxConfig() error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not in a tmux session")
	}

	// Run tmux source-file command
	// Use tmux.Command to respect GROVE_TMUX_SOCKET
	cmd := tmux.Command("source-file", expandPath("~/.tmux.conf"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux reload failed: %s", string(output))
	}

	return nil
}

func init() {
	keyCmd.AddCommand(keyManageCmd)
}
