package main

import (
	"context"
	"fmt"
	"os"
	grovecontext "github.com/mattsolo1/grove-context/pkg/context"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
)

// sessionizeModel is the model for the interactive project picker
type sessionizeModel struct {
	projects                 []*manager.SessionizeProject
	filtered                 []*manager.SessionizeProject
	selected                 *manager.SessionizeProject
	projectMap               map[string]*manager.SessionizeProject // For fast lookups by path
	cursor                   int
	filterInput              textinput.Model
	searchPaths              []string
	manager                  *tmux.Manager
	configDir                string                        // configuration directory
	keyMap                   map[string]string             // path -> key mapping
	runningSessions          map[string]bool   // map[sessionName] -> true
	claudeStatusMap          map[string]string // path -> claude session status mapping
	claudeDurationMap        map[string]string // path -> claude session state duration mapping
	claudeDurationSecondsMap map[string]int    // path -> claude session state duration in seconds
	hasGroveHooks            bool              // whether grove-hooks is available
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
	ecosystemPickerMode bool                          // True when showing only ecosystems for selection
	focusedProject      *manager.SessionizeProject
	worktreesFolded     bool // Whether worktrees are hidden/collapsed

	// View toggles
	showGitStatus      bool // Whether to fetch and show Git status
	showBranch         bool // Whether to show branch names
	showClaudeSessions bool // Whether to fetch and show Claude sessions
	showNoteCounts     bool // Whether to fetch and show note counts
	showPlanStats      bool // Whether to show plan stats from grove-flow
	pathDisplayMode    int  // 0=no paths, 1=compact (~), 2=full paths

	// Filter mode
	filterDirty bool // Whether to filter to only projects with dirty Git status

	// Context-only projects (shown but not selectable during filtered search in focus mode)
	contextOnlyPaths map[string]bool

	// Status message
	statusMessage string
	statusTimeout time.Time

	// Loading state
	isLoading     bool
	usedCache     bool      // Whether we loaded from cache on startup
	spinnerFrame  int       // Current frame of the spinner animation
	lastSpinTime  time.Time // Last time spinner was updated

	// Enrichment loading state
	enrichmentLoading map[string]bool // tracks which enrichments are currently loading

	// Context rules state
	rulesState map[string]grovecontext.RuleStatus // path -> status
}
func newSessionizeModel(projects []*manager.SessionizeProject, searchPaths []string, mgr *tmux.Manager, configDir string, usedCache bool, cwdFocusPath string) sessionizeModel {
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

	helpModel := help.NewBuilder().
		WithKeys(sessionizeKeys).
		WithTitle("Project Sessionizer - Help").
		Build()

	// Build project map for fast lookups and initialize enrichment status
	projectMap := make(map[string]*manager.SessionizeProject, len(projects))
	for _, p := range projects {
		p.EnrichmentStatus = make(map[string]string)
		// Mark cached enrichment data as done so it doesn't get overwritten with "loading"
		if p.GitStatus != nil {
			p.EnrichmentStatus["git"] = "done"
		}
		projectMap[p.Path] = p
	}

	// Load previously focused ecosystem and fold state
	var focusedProject *manager.SessionizeProject
	var worktreesFolded bool
	// Set sensible defaults for toggles
	showGitStatus := true
	showBranch := true
	showClaudeSessions := true
	showNoteCounts := true
	showPlanStats := true
	pathDisplayMode := 1 // Default to compact paths (~)
	if state, err := manager.LoadState(configDir); err == nil {
		// Prioritize CWD focus path over saved state
		if cwdFocusPath != "" {
			state.FocusedEcosystemPath = cwdFocusPath
		}
		if state.FocusedEcosystemPath != "" {
			// Find the project with this path (try exact match first, then case-insensitive)
			if proj, ok := projectMap[state.FocusedEcosystemPath]; ok {
				focusedProject = proj
			} else {
				// Try case-insensitive lookup (needed on case-insensitive filesystems like macOS)
				lowerFocusPath := strings.ToLower(state.FocusedEcosystemPath)
				for path, proj := range projectMap {
					if strings.ToLower(path) == lowerFocusPath {
						focusedProject = proj
						break
					}
				}
			}
		}
		worktreesFolded = state.WorktreesFolded
		// Override defaults with saved state if present
		if state.ShowGitStatus != nil {
			showGitStatus = *state.ShowGitStatus
		}
		if state.ShowBranch != nil {
			showBranch = *state.ShowBranch
		}
		if state.ShowClaudeSessions != nil {
			showClaudeSessions = *state.ShowClaudeSessions
		}
		if state.ShowNoteCounts != nil {
			showNoteCounts = *state.ShowNoteCounts
		}
		if state.ShowPlanStats != nil {
			showPlanStats = *state.ShowPlanStats
		}
		if state.PathDisplayMode != nil {
			pathDisplayMode = *state.PathDisplayMode
		}
	}

	return sessionizeModel{
		rulesState:               make(map[string]grovecontext.RuleStatus),
		projects:                 projects,
		filtered:                 projects,
		projectMap:               projectMap,
		filterInput:              ti,
		searchPaths:              searchPaths,
		manager:                  mgr,
		configDir:                configDir,
		keyMap:                   keyMap,
		runningSessions:          runningSessions,
		claudeStatusMap:          claudeStatusMap,
		claudeDurationMap:        claudeDurationMap,
		claudeDurationSecondsMap: claudeDurationSecondsMap,
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
		showBranch:               showBranch,
		showClaudeSessions:       showClaudeSessions,
		showNoteCounts:           showNoteCounts,
		showPlanStats:            showPlanStats,
		pathDisplayMode:          pathDisplayMode,
		contextOnlyPaths:         make(map[string]bool),
		usedCache:                usedCache,
		isLoading:                usedCache, // Start as loading if we used cache (will refresh in background)
		enrichmentLoading:        make(map[string]bool),
	}
}

// buildState creates a SessionizerState from the current model
func (m sessionizeModel) buildState() *manager.SessionizerState {
	state := &manager.SessionizerState{
		FocusedEcosystemPath: "",
		WorktreesFolded:      m.worktreesFolded,
		ShowGitStatus:        boolPtr(m.showGitStatus),
		ShowBranch:           boolPtr(m.showBranch),
		ShowClaudeSessions:   boolPtr(m.showClaudeSessions),
		ShowNoteCounts:       boolPtr(m.showNoteCounts),
		ShowPlanStats:        boolPtr(m.showPlanStats),
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

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}
func (m sessionizeModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		fetchRunningSessionsCmd(),
		fetchKeyMapCmd(m.manager),
		tickCmd(), // Start the periodic refresh cycle
	}

	// Only do full project discovery if we didn't load from cache
	// If we used cache, we already have projects with enrichment data
	if !m.usedCache {
		cmds = append(cmds, fetchProjectsCmd(m.manager, m.configDir))
	} else {
		// if we used cache, we have projects, so we can fetch rules state
		cmds = append(cmds, fetchRulesStateCmd(m.projects))
	}

	// Fetch fresh enrichment data (will update cached data in background)
	// Git status is fetched for visible projects only via enrichVisibleProjects()
	if m.showClaudeSessions {
		m.enrichmentLoading["claude"] = true
		cmds = append(cmds, fetchAllClaudeSessionsCmd())
	}
	if m.showNoteCounts {
		m.enrichmentLoading["notes"] = true
		cmds = append(cmds, fetchAllNoteCountsCmd())
	}
	if m.showPlanStats {
		m.enrichmentLoading["plans"] = true
		cmds = append(cmds, fetchAllPlanStatsCmd())
	}
	if m.showGitStatus {
		// Refresh git status for all projects in background
		m.enrichmentLoading["git"] = true
		cmds = append(cmds, fetchAllGitStatusesCmd(m.projects))
	}

	// Start spinner animation if loading or if any enrichment is loading
	anyEnrichmentLoading := m.isLoading
	for _, loading := range m.enrichmentLoading {
		if loading {
			anyEnrichmentLoading = true
			break
		}
	}
	if anyEnrichmentLoading {
		cmds = append(cmds, spinnerTickCmd())
	}

	return tea.Batch(cmds...)
}
// moveCursorUp moves the cursor up, skipping context-only (non-selectable) items
func (m *sessionizeModel) moveCursorUp() {
	if m.cursor <= 0 {
		return
	}

	// Move up by one
	m.cursor--

	// Skip context-only items
	for m.cursor > 0 && len(m.filtered) > m.cursor {
		project := m.filtered[m.cursor]
		if m.contextOnlyPaths[project.Path] {
			m.cursor--
		} else {
			break
		}
	}

	// If we landed on a context-only item at position 0, find the first selectable item
	if m.cursor == 0 && len(m.filtered) > 0 && m.contextOnlyPaths[m.filtered[0].Path] {
		m.moveCursorToFirstSelectable()
	}
}

// moveCursorDown moves the cursor down, skipping context-only (non-selectable) items
func (m *sessionizeModel) moveCursorDown() {
	if m.cursor >= len(m.filtered)-1 {
		return
	}

	// Move down by one
	m.cursor++

	// Skip context-only items
	for m.cursor < len(m.filtered)-1 {
		project := m.filtered[m.cursor]
		if m.contextOnlyPaths[project.Path] {
			m.cursor++
		} else {
			break
		}
	}

	// If we're at the last item and it's context-only, stay where we were
	if m.cursor == len(m.filtered)-1 && len(m.filtered) > 0 && m.contextOnlyPaths[m.filtered[m.cursor].Path] {
		m.cursor--
		// Move back up to find a selectable item
		for m.cursor > 0 && m.contextOnlyPaths[m.filtered[m.cursor].Path] {
			m.cursor--
		}
	}
}

// moveCursorToFirstSelectable moves the cursor to the first selectable item
func (m *sessionizeModel) moveCursorToFirstSelectable() {
	for i := 0; i < len(m.filtered); i++ {
		if !m.contextOnlyPaths[m.filtered[i].Path] {
			m.cursor = i
			return
		}
	}
	// If no selectable items, stay at 0
	m.cursor = 0
}

func (m sessionizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case gitStatusMsg:
		if proj, ok := m.projectMap[msg.path]; ok {
			proj.GitStatus = msg.status
			proj.EnrichmentStatus["git"] = "done"
		}
		return m, nil

	case gitStatusMapMsg:
		for path, status := range msg.statuses {
			if proj, ok := m.projectMap[path]; ok {
				proj.GitStatus = status
				proj.EnrichmentStatus["git"] = "done"
			}
		}
		m.enrichmentLoading["git"] = false
		return m, nil

	case claudeSessionMapMsg:
		// Update Claude sessions - preserve existing data, only update what changed
		// First, clear sessions that no longer exist
		activePaths := make(map[string]bool)
		for path := range msg.sessions {
			activePaths[path] = true
			if parentPath := getWorktreeParent(path); parentPath != "" {
				activePaths[parentPath] = true
			}
		}
		for _, proj := range m.projects {
			if proj.ClaudeSession != nil && !activePaths[proj.Path] {
				proj.ClaudeSession = nil
			}
		}
		// Now update with new session data
		for path, session := range msg.sessions {
			if proj, ok := m.projectMap[path]; ok {
				proj.ClaudeSession = session
			}
			// Also apply to parent project if it's a worktree
			if parentPath := getWorktreeParent(path); parentPath != "" {
				if proj, ok := m.projectMap[parentPath]; ok {
					proj.ClaudeSession = session
				}
			}
		}
		m.enrichmentLoading["claude"] = false
		return m, nil

	case noteCountsMapMsg:
		// Update note counts - only update projects that have counts
		for _, proj := range m.projects {
			if counts, ok := msg.counts[proj.Name]; ok {
				proj.NoteCounts = counts
			}
		}
		m.enrichmentLoading["notes"] = false
		return m, nil

	case planStatsMapMsg:
		// Update plan stats - only update projects that have stats
		for _, proj := range m.projects {
			if stats, ok := msg.stats[proj.Path]; ok {
				proj.PlanStats = stats
			}
		}
		m.enrichmentLoading["plans"] = false
		return m, nil

	case projectsUpdateMsg:
		// Save the current selected project path
		selectedPath := ""
		if m.cursor < len(m.filtered) {
			selectedPath = m.filtered[m.cursor].Path
		}

		// Update the main project list and map
		m.projects = msg.projects
		m.projectMap = make(map[string]*manager.SessionizeProject, len(m.projects))
		for _, p := range m.projects {
			p.EnrichmentStatus = make(map[string]string)
			m.projectMap[p.Path] = p
		}

		m.isLoading = false // Mark loading as complete

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

		return m, tea.Batch(m.enrichVisibleProjects(), fetchRulesStateCmd(m.projects))

	case rulesStateUpdateMsg:
		m.rulesState = msg.rulesState
		return m, nil

	case ruleToggleResultMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Error: %v", msg.err)
			m.statusTimeout = time.Now().Add(3 * time.Second)
			return m, clearStatusCmd(3 * time.Second)
		}
		m.statusMessage = "Context rule updated!"
		m.statusTimeout = time.Now().Add(2 * time.Second)
		// Refresh rules state
		return m, tea.Batch(clearStatusCmd(2*time.Second), fetchRulesStateCmd(m.projects))

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

	case spinnerTickMsg:
		// Update spinner animation frame
		m.spinnerFrame++
		// Keep spinner running if loading or if any enrichment is loading
		anyEnrichmentLoading := m.isLoading
		for _, loading := range m.enrichmentLoading {
			if loading {
				anyEnrichmentLoading = true
				break
			}
		}
		if anyEnrichmentLoading {
			return m, spinnerTickCmd() // Reschedule next spinner tick
		}
		return m, nil

	case tickMsg:
		// Periodically refresh enrichment data, but NOT the project list itself.
		// The project list is only updated on manual refresh (ctrl+r).
		cmds := []tea.Cmd{
			fetchRunningSessionsCmd(),
			fetchKeyMapCmd(m.manager),
			tickCmd(), // This reschedules the tick
		}

		// Track if we're starting any enrichment
		startedEnrichment := false

		if m.showGitStatus {
			m.enrichmentLoading["git"] = true
			startedEnrichment = true
			cmds = append(cmds, fetchAllGitStatusesCmd(m.projects))
		}
		if m.showClaudeSessions {
			m.enrichmentLoading["claude"] = true
			startedEnrichment = true
			cmds = append(cmds, fetchAllClaudeSessionsCmd())
		}
		if m.showNoteCounts {
			m.enrichmentLoading["notes"] = true
			startedEnrichment = true
			cmds = append(cmds, fetchAllNoteCountsCmd())
		}
		if m.showPlanStats {
			m.enrichmentLoading["plans"] = true
			startedEnrichment = true
			cmds = append(cmds, fetchAllPlanStatsCmd())
		}

		// Start spinner if we kicked off any enrichment
		if startedEnrichment {
			cmds = append(cmds, spinnerTickCmd())
		}

		return m, tea.Batch(cmds...)

	case statusMsg:
		m.statusMessage = msg.message
		if msg.message == "" {
			m.statusTimeout = time.Time{}
		}
		return m, nil

	case tea.KeyMsg:
		// If help is visible, it consumes all key presses
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		// Handle non-letter key bindings that should work even in search mode
		switch {
		case key.Matches(msg, sessionizeKeys.RefreshProjects):
			m.isLoading = true
			m.statusMessage = "Refreshing project list..."
			m.statusTimeout = time.Now().Add(5 * time.Second)
			return m, tea.Batch(spinnerTickCmd(), fetchProjectsCmd(m.manager, m.configDir))

		case key.Matches(msg, sessionizeKeys.ClearFocus):
			if m.focusedProject != nil {
				m.focusedProject = nil
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()

				// Clear the focused ecosystem from state
				_ = m.buildState().Save(m.configDir)
			}
			return m, nil

		case key.Matches(msg, sessionizeKeys.FilterDirty):
			// Toggle dirty filter
			m.filterDirty = !m.filterDirty
			// Clear text filter to make them mutually exclusive
			m.filterInput.SetValue("")
			m.updateFiltered()
			m.cursor = 0
			m.moveCursorToFirstSelectable()
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
				m.updateFiltered()
				return m, nil
			case tea.KeyEnter:
				// Handle ecosystem picker mode
				if m.ecosystemPickerMode {
					if m.cursor < len(m.filtered) {
						// Set focused project (already a pointer)
						selected := m.filtered[m.cursor]
						m.focusedProject = selected
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
					// Save cache before quitting to persist enrichment data
					projects := make([]manager.SessionizeProject, len(m.projects))
					for i, p := range m.projects {
						projects[i] = *p
					}
					_ = manager.SaveProjectCache(m.configDir, projects)
					return m, tea.Quit
				}
				return m, nil
			case tea.KeyUp:
				// Navigate up while filtering
				if m.cursor > 0 {
					m.cursor--
				}
				return m, m.enrichVisibleProjects()
			case tea.KeyDown:
				// Navigate down while filtering
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, m.enrichVisibleProjects()
			default:
				// Let filter input handle all other keys when focused
				prevValue := m.filterInput.Value()
				m.filterInput, cmd = m.filterInput.Update(msg)
				
				// If the filter changed, update filtered list
				if m.filterInput.Value() != prevValue {
					m.updateFiltered()
					m.cursor = 0
					m.moveCursorToFirstSelectable()
				}
				return m, cmd
			}
		}

		// Normal mode (when filter is not focused)
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			m.moveCursorUp()
			return m, m.enrichVisibleProjects()
		case tea.KeyDown, tea.KeyCtrlN:
			m.moveCursorDown()
			return m, m.enrichVisibleProjects()
		case tea.KeyCtrlU:
			// Page up (vim-style)
			pageSize := 10
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, m.enrichVisibleProjects()
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
			return m, m.enrichVisibleProjects()
		case tea.KeyRunes:
			switch msg.String() {
			case "j":
				// Vim-style down navigation
				m.moveCursorDown()
				return m, m.enrichVisibleProjects()
			case "k":
				// Vim-style up navigation
				m.moveCursorUp()
				return m, m.enrichVisibleProjects()
			case "g":
				// Handle gg (go to top) - need to check for double g
				// For simplicity, single g goes to top (common in many TUIs)
				m.cursor = 0
				return m, m.enrichVisibleProjects()
			case "G":
				// Go to bottom
				m.cursor = len(m.filtered) - 1
				if m.cursor < 0 {
					m.cursor = 0
				}
				return m, m.enrichVisibleProjects()
			case "X":
				// Close session (moved from ctrl+d)
				if m.cursor < len(m.filtered) {
					project := m.filtered[m.cursor]
					sessionName := project.Identifier()

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
										candidateName := p.Identifier()

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
				// Clear dirty filter to make them mutually exclusive
				if m.filterDirty {
					m.filterDirty = false
				}
				m.filterInput.Focus()
				return m, textinput.Blink
			case "@":
				// Focus ecosystem (handled by key.Matches below for consistency)
				m.ecosystemPickerMode = true
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
				return m, nil
			case "s":
				// Toggle git status
				m.showGitStatus = !m.showGitStatus
				_ = m.buildState().Save(m.configDir)
				return m, m.enrichVisibleProjects()
			case "b":
				// Toggle branch names
				m.showBranch = !m.showBranch
				_ = m.buildState().Save(m.configDir)
				return m, m.enrichVisibleProjects()
			case "C":
				// Toggle claude sessions
				m.showClaudeSessions = !m.showClaudeSessions
				_ = m.buildState().Save(m.configDir)
				// Refetch claude sessions if toggled on
				if m.showClaudeSessions {
					return m, fetchAllClaudeSessionsCmd()
				}
				return m, nil
			case "h": // Toggle hot context
				if m.cursor < len(m.filtered) {
					selected := m.filtered[m.cursor]
					return m, toggleRuleCmd(selected, "hot")
				}
			case "c": // Toggle cold context
				if m.cursor < len(m.filtered) {
					selected := m.filtered[m.cursor]
					return m, toggleRuleCmd(selected, "cold")
				}
			case "x": // Toggle exclude
				if m.cursor < len(m.filtered) {
					selected := m.filtered[m.cursor]
					return m, toggleRuleCmd(selected, "exclude")
				}
			case "n":
				// Toggle note counts
				m.showNoteCounts = !m.showNoteCounts
				_ = m.buildState().Save(m.configDir)
				// Refetch note counts if toggled on
				if m.showNoteCounts {
					return m, fetchAllNoteCountsCmd()
				}
				return m, nil
			case "f":
				// Toggle plan stats
				m.showPlanStats = !m.showPlanStats
				_ = m.buildState().Save(m.configDir)
				// Refetch plan stats if toggled on
				if m.showPlanStats {
					return m, fetchAllPlanStatsCmd()
				}
				return m, nil
			case "p":
				// Toggle paths display mode
				m.pathDisplayMode = (m.pathDisplayMode + 1) % 3
				_ = m.buildState().Save(m.configDir)
				return m, nil
			}
		case tea.KeyTab:
			// Toggle worktrees
			m.worktreesFolded = !m.worktreesFolded
			m.updateFiltered()
			_ = m.buildState().Save(m.configDir)
			return m, m.enrichVisibleProjects()
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
						m.statusMessage = "No clipboard utility found"
						m.statusTimeout = time.Now().Add(2 * time.Second)
						return m, clearStatusCmd(2 * time.Second)
					}
				}

				if cmd != nil {
					cmd.Stdin = strings.NewReader(project.Path)
					if err := cmd.Run(); err == nil {
						m.statusMessage = "Path copied to clipboard"
						m.statusTimeout = time.Now().Add(2 * time.Second)
						return m, clearStatusCmd(2 * time.Second)
					} else {
						m.statusMessage = "Failed to copy path"
						m.statusTimeout = time.Now().Add(2 * time.Second)
						return m, clearStatusCmd(2 * time.Second)
					}
				}
			}
		case tea.KeyEnter:
			// Handle ecosystem picker mode
			if m.ecosystemPickerMode {
				if m.cursor < len(m.filtered) {
					// Set focused project (already a pointer)
					selected := m.filtered[m.cursor]
					m.focusedProject = selected
					m.ecosystemPickerMode = false
					m.updateFiltered() // Now filter to focused ecosystem
					m.cursor = 0
					m.moveCursorToFirstSelectable()

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
				// Save cache before quitting to persist enrichment data
				projects := make([]manager.SessionizeProject, len(m.projects))
				for i, p := range m.projects {
					projects[i] = *p
				}
				_ = manager.SaveProjectCache(m.configDir, projects)
				return m, tea.Quit
			}
		case tea.KeyEsc, tea.KeyCtrlC:
			// If dirty filter is active, clear it first
			if m.filterDirty {
				m.filterDirty = false
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
				return m, m.enrichVisibleProjects()
			}
			// Save cache before quitting to persist enrichment data
			projects := make([]manager.SessionizeProject, len(m.projects))
			for i, p := range m.projects {
				projects[i] = *p
			}
			_ = manager.SaveProjectCache(m.configDir, projects)
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
		m.filtered = []*manager.SessionizeProject{}

		// Separate into main ecosystems and worktrees
		mainEcosystemsMap := make(map[string]*manager.SessionizeProject)
		worktreesByParent := make(map[string][]*manager.SessionizeProject)

		for _, p := range m.projects {
			if !p.IsEcosystem() {
				continue
			}

			// Apply filter
			matchesFilter := filter == "" ||
				strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Path), filter)

			if !matchesFilter {
				continue
			}

			if p.IsWorktree() && p.ParentProjectPath != "" {
				// This is a worktree - group by parent
				worktreesByParent[p.ParentProjectPath] = append(worktreesByParent[p.ParentProjectPath], p)
			} else {
				// This is a main ecosystem
				mainEcosystemsMap[p.Path] = p
			}
		}

		// Convert map to slice and sort
		var mainEcosystems []*manager.SessionizeProject
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
				// Sort worktrees: ecosystem worktrees first, then regular worktrees, both alphabetically
				sort.Slice(worktrees, func(i, j int) bool {
					iIsEcoWT := worktrees[i].Kind == workspace.KindEcosystemWorktree
					jIsEcoWT := worktrees[j].Kind == workspace.KindEcosystemWorktree

					// If one is ecosystem worktree and the other isn't, ecosystem worktree comes first
					if iIsEcoWT != jIsEcoWT {
						return iIsEcoWT
					}

					// Otherwise sort alphabetically
					return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
				})
				m.filtered = append(m.filtered, worktrees...)
			}
		}
		return
	}

	// Create a working list of projects, either all projects or just the focused ecosystem
	var projectsToFilter []*manager.SessionizeProject
	if m.focusedProject != nil {
		// Add the focused project itself
		projectsToFilter = append(projectsToFilter, m.focusedProject)

		// This map will only track sub-projects, not ecosystem worktrees, for grandchild traversal
		directChildrenSubProjects := make(map[string]bool)

		// Add all direct children (ecosystem children and ecosystem worktrees)
		for _, p := range m.projects {
			if p.IsChildOf(m.focusedProject.Path) {
				projectsToFilter = append(projectsToFilter, p)

				// IMPORTANT: Only track sub-projects for finding their worktrees later.
				// An ecosystem worktree (like website-infra) is also an ecosystem, so this check excludes it.
				if !p.IsEcosystem() {
					directChildrenSubProjects[p.Path] = true
				}
			}
		}

		// Now, only add worktrees of the direct *sub-project* children
		// IMPORTANT: Exclude worktrees that are located inside ecosystem worktrees.
		// For KindEcosystemWorktreeSubProjectWorktree, GetHierarchicalParent() returns the
		// ecosystem worktree path (not the git parent), so we can filter them out by checking
		// that the hierarchical parent matches the ParentProjectPath.
		for _, p := range m.projects {
			if p.IsWorktree() && p.ParentProjectPath != "" && directChildrenSubProjects[p.ParentProjectPath] {
				// Only include if the hierarchical parent is the same as the git parent
				// (i.e., this is a normal worktree, not one inside an ecosystem worktree)
				if p.GetHierarchicalParent() == p.ParentProjectPath {
					projectsToFilter = append(projectsToFilter, p)
				}
			}
		}
	} else {
		// No focus, use all projects
		projectsToFilter = m.projects
	}

	// Apply dirty filter if active
	if m.filterDirty {
		pathsToKeep := make(map[string]bool)

		// Iterate over all projects to find dirty ones and their ancestors
		for _, p := range m.projects {
			if p.GetGitStatus() != nil && p.GetGitStatus().IsDirty {
				// Mark this project
				pathsToKeep[p.Path] = true

				// Mark ancestors to preserve hierarchy
				if p.ParentProjectPath != "" {
					pathsToKeep[p.ParentProjectPath] = true
				}
				if p.ParentEcosystemPath != "" {
					pathsToKeep[p.ParentEcosystemPath] = true
				}
			}
		}

		// Filter projectsToFilter to only include paths we want to keep
		var filtered []*manager.SessionizeProject
		for _, p := range projectsToFilter {
			if pathsToKeep[p.Path] {
				filtered = append(filtered, p)
			}
		}
		projectsToFilter = filtered
	}

	// Build map of active groups. A group is identified by GetGroupingKey(), which returns:
	// - For project worktrees: the parent project path
	// - For all other nodes: the node's own path
	activeGroups := make(map[string]bool)
	for _, p := range projectsToFilter {
		groupKey := p.GetGroupingKey()
		if groupKey == "" { // Should not happen, but as a safeguard
			continue
		}

		sessionName := p.Identifier()
		if m.runningSessions[sessionName] {
			activeGroups[groupKey] = true
		}
	}

	if filter == "" {
		// Default View: Group-aware sorting with inactive worktree filtering
		// Clear context-only paths when not filtering
		m.contextOnlyPaths = make(map[string]bool)

		if m.focusedProject != nil {
			// Focus mode: Different handling for ecosystems vs regular projects
			if m.focusedProject.IsEcosystem() {
				// For ecosystems, we show all direct children by default
				// regardless of whether they are worktrees or sub-projects
				m.filtered = []*manager.SessionizeProject{}

				// First, add the focused ecosystem itself
				for _, p := range projectsToFilter {
					if p.Path == m.focusedProject.Path {
						m.filtered = append(m.filtered, p)
						break
					}
				}

				// Collect and sort all direct children (both sub-projects and worktrees)
				var directChildren []*manager.SessionizeProject
				var grandchildren []*manager.SessionizeProject

				for _, p := range projectsToFilter {
					if p.Path == m.focusedProject.Path {
						continue // Skip the focused project itself
					}

					// Check if this is a direct child
					if p.GetHierarchicalParent() == m.focusedProject.Path {
						directChildren = append(directChildren, p)
					} else {
						// This might be a grandchild (worktree of a child project)
						grandchildren = append(grandchildren, p)
					}
				}

				// Sort direct children: ecosystem worktrees first, then other children, both alphabetically
				sort.Slice(directChildren, func(i, j int) bool {
					iIsEcoWT := directChildren[i].Kind == workspace.KindEcosystemWorktree
					jIsEcoWT := directChildren[j].Kind == workspace.KindEcosystemWorktree

					// If one is ecosystem worktree and the other isn't, ecosystem worktree comes first
					if iIsEcoWT != jIsEcoWT {
						return iIsEcoWT
					}

					// Otherwise sort alphabetically
					return strings.ToLower(directChildren[i].Name) < strings.ToLower(directChildren[j].Name)
				})

				// Add direct children, filtering ecosystem worktrees based on fold state
				for _, child := range directChildren {
					// Skip ecosystem worktrees if worktrees are folded
					if m.worktreesFolded && child.Kind == workspace.KindEcosystemWorktree {
						continue
					}

					m.filtered = append(m.filtered, child)

					// If worktrees are not folded, add this child's worktrees (only for non-ecosystem-worktree children)
					if !m.worktreesFolded && child.Kind != workspace.KindEcosystemWorktree {
						// Collect and sort this child's worktrees
						childWorktrees := []*manager.SessionizeProject{}
						for _, gc := range grandchildren {
							if gc.ParentProjectPath == child.Path {
								childWorktrees = append(childWorktrees, gc)
							}
						}
						sort.Slice(childWorktrees, func(i, j int) bool {
							return strings.ToLower(childWorktrees[i].Name) < strings.ToLower(childWorktrees[j].Name)
						})
						m.filtered = append(m.filtered, childWorktrees...)
					}
				}
			} else {
				// Regular project focus: Group repos with their worktrees hierarchically
				m.filtered = []*manager.SessionizeProject{}

				// First add the focused project
				m.filtered = append(m.filtered, m.focusedProject)

				// Build a map of parents to their worktrees
				parentWorktrees := make(map[string][]*manager.SessionizeProject)
				nonWorktrees := []*manager.SessionizeProject{}

				for _, p := range projectsToFilter {
					if p.Path == m.focusedProject.Path {
						continue // Skip focused project, already added
					}
					if p.IsWorktree() {
						parentWorktrees[p.ParentProjectPath] = append(parentWorktrees[p.ParentProjectPath], p)
					} else {
						nonWorktrees = append(nonWorktrees, p)
					}
				}

				// Add non-worktree repos, each followed by their worktrees (if not folded)
				for _, parent := range nonWorktrees {
					m.filtered = append(m.filtered, parent)
					if !m.worktreesFolded {
						if worktrees, exists := parentWorktrees[parent.Path]; exists {
							// Sort worktrees alphabetically before adding
							sort.Slice(worktrees, func(i, j int) bool {
								return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
							})
							m.filtered = append(m.filtered, worktrees...)
						}
					}
				}

				// Add any remaining worktrees if not folded
				if !m.worktreesFolded {
					if focusedWorktrees, exists := parentWorktrees[m.focusedProject.Path]; exists {
						// Sort worktrees alphabetically before inserting
						sort.Slice(focusedWorktrees, func(i, j int) bool {
							return strings.ToLower(focusedWorktrees[i].Name) < strings.ToLower(focusedWorktrees[j].Name)
						})
						// Insert these after the focused project (at position 1)
						m.filtered = append(m.filtered[:1], append(focusedWorktrees, m.filtered[1:]...)...)
					}
				}
			}
		} else {
			// Normal mode: Group worktrees under their parents and respect fold state
			// Separate projects into parents and worktrees
			var parentRepos []*manager.SessionizeProject
			worktreesByParent := make(map[string][]*manager.SessionizeProject)

			for _, p := range projectsToFilter {
				if p.IsWorktree() {
					worktreesByParent[p.ParentProjectPath] = append(worktreesByParent[p.ParentProjectPath], p)
				} else {
					parentRepos = append(parentRepos, p)
				}
			}

			// Sort parents by activity (active sessions first)
			sort.SliceStable(parentRepos, func(i, j int) bool {
				sessionI := parentRepos[i].Identifier()
				sessionJ := parentRepos[j].Identifier()
				activeI := m.runningSessions[sessionI]
				activeJ := m.runningSessions[sessionJ]

				if activeI && !activeJ {
					return true
				}
				if !activeI && activeJ {
					return false
				}
				return false // Maintain original order for same activity status
			})

			// Build filtered list: parent followed by its worktrees (if not folded)
			m.filtered = []*manager.SessionizeProject{}
			for _, parent := range parentRepos {
				m.filtered = append(m.filtered, parent)

				if !m.worktreesFolded {
					if worktrees, exists := worktreesByParent[parent.Path]; exists {
						// Sort worktrees: ecosystem worktrees first, then regular worktrees, both alphabetically
						sort.Slice(worktrees, func(i, j int) bool {
							iIsEcoWT := worktrees[i].Kind == workspace.KindEcosystemWorktree
							jIsEcoWT := worktrees[j].Kind == workspace.KindEcosystemWorktree

							// If one is ecosystem worktree and the other isn't, ecosystem worktree comes first
							if iIsEcoWT != jIsEcoWT {
								return iIsEcoWT
							}

							// Otherwise sort alphabetically
							return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
						})
						m.filtered = append(m.filtered, worktrees...)
					}
				}
			}
		}
	} else {
		// Filtered View: Show all matching projects, grouped by activity

		// sortByMatchQuality sorts projects by match quality while preserving parent-child grouping.
		// This is used in global mode searches where we want to show parent repos alongside their worktrees
		// for context. Note: This groups ALL worktrees (including EcosystemWorktrees) with their parents.
		sortByMatchQuality := func(projects []*manager.SessionizeProject, filter string) []*manager.SessionizeProject {
			// Build a map of parents to their worktrees
			parentWorktrees := make(map[string][]*manager.SessionizeProject)
			parents := []*manager.SessionizeProject{}

			for _, p := range projects {
				// IsWorktree() includes both project worktrees and ecosystem worktrees
				if p.IsWorktree() {
					parentWorktrees[p.ParentProjectPath] = append(parentWorktrees[p.ParentProjectPath], p)
				} else {
					parents = append(parents, p)
				}
			}

			// Calculate match quality for sorting (name only, not path)
			getMatchQuality := func(p *manager.SessionizeProject) int {
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

			// Build result with parents followed by their worktrees
			var result []*manager.SessionizeProject
			for _, parent := range parents {
				result = append(result, parent)

				// Add worktrees for this parent, sorted by match quality, with ecosystem worktrees prioritized
				worktrees := parentWorktrees[parent.Path]
				sort.SliceStable(worktrees, func(i, j int) bool {
					iQuality := getMatchQuality(worktrees[i])
					jQuality := getMatchQuality(worktrees[j])

					// If match quality differs, sort by that
					if iQuality != jQuality {
						return iQuality > jQuality
					}

					// For same match quality, prioritize ecosystem worktrees
					iIsEcoWT := worktrees[i].Kind == workspace.KindEcosystemWorktree
					jIsEcoWT := worktrees[j].Kind == workspace.KindEcosystemWorktree
					if iIsEcoWT != jIsEcoWT {
						return iIsEcoWT
					}

					// Otherwise maintain stable order
					return false
				})
				result = append(result, worktrees...)
			}

			return result
		}

		// Collect matched projects
		var matchedProjects []*manager.SessionizeProject

		if m.focusedProject != nil {
			// Focus mode: simpler, direct search
			// Match any project whose name or path contains the filter
			matchedPaths := make(map[string]bool)
			parentsToInclude := make(map[string]bool)

			// First, find all matches
			for _, p := range projectsToFilter {
				if strings.Contains(strings.ToLower(p.Name), filter) ||
				   strings.Contains(strings.ToLower(p.Path), filter) {
					matchedPaths[p.Path] = true

					// Walk up hierarchy to include all ancestors for context
					current := p
					for current != nil {
						parent := current.GetHierarchicalParent()
						if parent == "" || parent == m.focusedProject.Path {
							// Reached the focused project or root
							break
						}

						// Find the parent project and mark it for inclusion
						for _, ancestor := range projectsToFilter {
							if ancestor.Path == parent {
								parentsToInclude[parent] = true
								current = ancestor
								break
							}
						}

						if current.Path == p.Path {
							// Couldn't find parent, stop walking
							break
						}
					}
				}
			}

			// Add the focused project itself if not already there
			parentsToInclude[m.focusedProject.Path] = true

			// Build context-only paths (parents that aren't actual matches)
			m.contextOnlyPaths = make(map[string]bool)
			for path := range parentsToInclude {
				if !matchedPaths[path] {
					m.contextOnlyPaths[path] = true
				}
			}

			// Collect all projects to display (matches + their ancestors)
			for _, p := range projectsToFilter {
				if matchedPaths[p.Path] || parentsToInclude[p.Path] {
					matchedProjects = append(matchedProjects, p)
				}
			}
		} else {
			// Global mode: complex search with parent-child awareness
			// Clear context-only paths (only used in focus mode)
			m.contextOnlyPaths = make(map[string]bool)

			matchedParents := make(map[string]bool) // Track which parent projects matched
			parentsWithMatchingWorktrees := make(map[string]bool) // Track parents whose worktrees matched

			// First pass: find matching parent projects (search name only)
			for _, p := range projectsToFilter {
				if p.IsWorktree() {
					continue // Skip worktrees in first pass
				}

				lowerName := strings.ToLower(p.Name)

				// Check if this parent project matches the filter (name only, not full path)
				if lowerName == filter || strings.HasPrefix(lowerName, filter) ||
				   strings.Contains(lowerName, filter) {
					matchedParents[p.Path] = true
				}
			}

			// Second pass: find worktrees that match and mark their parents for inclusion
			// (Always search worktrees, even if folded - they'll be shown when filtering)
			for _, p := range projectsToFilter {
				if !p.IsWorktree() {
					continue
				}

				lowerName := strings.ToLower(p.Name)

				// Check if this worktree matches the filter (name only, not full path)
				if lowerName == filter || strings.HasPrefix(lowerName, filter) ||
				   strings.Contains(lowerName, filter) {
					// Mark parent for inclusion even if parent didn't match directly
					parentsWithMatchingWorktrees[p.ParentProjectPath] = true
				}
			}

			// Third pass: add matched parents and their worktrees to matchedProjects
			for _, p := range projectsToFilter {
				shouldInclude := false

				if p.IsWorktree() {
					// Include worktree if it matches or its parent matched
					// (Show worktrees in search results even if they're folded in normal view)
					lowerName := strings.ToLower(p.Name)
					worktreeMatches := lowerName == filter || strings.HasPrefix(lowerName, filter) ||
						strings.Contains(lowerName, filter)
					parentMatched := matchedParents[p.ParentProjectPath]

					shouldInclude = worktreeMatches || parentMatched
				} else {
					// Include parent if it matched OR if any of its worktrees matched
					shouldInclude = matchedParents[p.Path] || parentsWithMatchingWorktrees[p.Path]
				}

				if shouldInclude {
					matchedProjects = append(matchedProjects, p)
				}
			}
		}

		if m.focusedProject != nil {
			// Focus mode: maintain hierarchical order with focused project first
			m.filtered = []*manager.SessionizeProject{}

			// First, add the focused project if it's in the matched set
			var focusedIncluded bool
			for _, p := range matchedProjects {
				if p.Path == m.focusedProject.Path {
					m.filtered = append(m.filtered, p)
					focusedIncluded = true
					break
				}
			}

			// If focused project wasn't included, add it anyway for context
			if !focusedIncluded {
				m.filtered = append(m.filtered, m.focusedProject)
				m.contextOnlyPaths[m.focusedProject.Path] = true
			}

			// Then add all other projects in their existing hierarchical order
			// (projectsToFilter is already hierarchically ordered from BuildWorkspaceTree)
			for _, p := range matchedProjects {
				if p.Path != m.focusedProject.Path {
					m.filtered = append(m.filtered, p)
				}
			}
		} else {
			// Global mode: Partition matched projects by group activity
			var activeGroupMatches []*manager.SessionizeProject
			var inactiveGroupMatches []*manager.SessionizeProject

			for _, p := range matchedProjects {
				// Use GetGroupingKey() which correctly returns ParentProjectPath for project worktrees
				// but returns the node's own path for ecosystem children and other nodes
				groupKey := p.GetGroupingKey()

				// Check group activity
				if activeGroups[groupKey] {
					activeGroupMatches = append(activeGroupMatches, p)
				} else {
					inactiveGroupMatches = append(inactiveGroupMatches, p)
				}
			}

			// Sort both groups by match quality (parents will naturally group with their worktrees)
			activeGroupMatches = sortByMatchQuality(activeGroupMatches, filter)
			inactiveGroupMatches = sortByMatchQuality(inactiveGroupMatches, filter)

			// Combine: active groups first, then inactive groups
			m.filtered = []*manager.SessionizeProject{}
			m.filtered = append(m.filtered, activeGroupMatches...)
			m.filtered = append(m.filtered, inactiveGroupMatches...)
		}
	}
}
func (m sessionizeModel) View() string {
	// If help is visible, show it and return
	if m.help.ShowAll {
		return m.help.View()
	}

	// Show key editing mode if active
	if m.editingKeys {
		return m.viewKeyEditor()
	}

	var b strings.Builder

	// Header with filter input (always at top)
	var header strings.Builder

	if m.filterDirty {
		header.WriteString(core_theme.DefaultTheme.Warning.Render("[DIRTY] "))
	}
	if m.ecosystemPickerMode {
		header.WriteString(core_theme.DefaultTheme.Info.Render("[Select Ecosystem to Focus]"))
		header.WriteString(" ")
	} else if m.focusedProject != nil {
		focusIndicator := core_theme.DefaultTheme.Info.Render(fmt.Sprintf("[Focus: %s]", m.focusedProject.Name))
		header.WriteString(focusIndicator)
		header.WriteString(" ")
	}
	// Show status message if active
	if m.statusMessage != "" && time.Now().Before(m.statusTimeout) {
		header.WriteString(core_theme.DefaultTheme.Success.Render("[" + m.statusMessage + "]"))
		header.WriteString(" ")
	}
	header.WriteString(m.filterInput.View())

	b.WriteString(header.String())
	b.WriteString("\n\n")

	// Render projects using table view
	b.WriteString(m.renderTable())

	// Help text at bottom
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No projects found"))
		} else {
			b.WriteString("\n" + core_theme.DefaultTheme.Muted.Render("No matching projects"))
		}
	}

	// Icon legend
	legendStyle := core_theme.DefaultTheme.Muted
	currentIcon := core_theme.DefaultTheme.Info.Render("●")
	activeIcon := core_theme.DefaultTheme.Highlight.Render("●")
	legend := fmt.Sprintf("Icons: %s current • %s active • %s ecosystem • %s repo • %s eco-worktree • %s worktree • %s branch",
		currentIcon, activeIcon, "◆", "●", "◇", "⑂", "⎇")
	b.WriteString("\n" + legendStyle.Render(legend))

	// Help text
	helpStyle := core_theme.DefaultTheme.Muted
	b.WriteString("\n")

	// Build toggle indicators
	gitToggle := "s:git status "
	if m.showGitStatus {
		gitToggle += core_theme.DefaultTheme.Success.Render("✓")
	} else {
		gitToggle += core_theme.DefaultTheme.Muted.Render("✗")
	}

	branchToggle := " b:branch "
	if m.showBranch {
		branchToggle += core_theme.DefaultTheme.Success.Render("✓")
	} else {
		branchToggle += core_theme.DefaultTheme.Muted.Render("✗")
	}

	claudeToggle := " c:claude "
	if m.showClaudeSessions {
		claudeToggle += core_theme.DefaultTheme.Success.Render("✓")
	} else {
		claudeToggle += core_theme.DefaultTheme.Muted.Render("✗")
	}

	noteToggle := " n:notes "
	if m.showNoteCounts {
		noteToggle += core_theme.DefaultTheme.Success.Render("✓")
	} else {
		noteToggle += core_theme.DefaultTheme.Muted.Render("✗")
	}

	planToggle := " f:plans "
	if m.showPlanStats {
		planToggle += core_theme.DefaultTheme.Success.Render("✓")
	} else {
		planToggle += core_theme.DefaultTheme.Muted.Render("✗")
	}

	pathsToggle := " p:paths "
	switch m.pathDisplayMode {
	case 0:
		pathsToggle += core_theme.DefaultTheme.Muted.Render("off")
	case 1:
		pathsToggle += core_theme.DefaultTheme.Success.Render("~")
	case 2:
		pathsToggle += core_theme.DefaultTheme.Success.Render("full")
	}

	togglesDisplay := fmt.Sprintf("[%s%s%s%s%s%s]", gitToggle, branchToggle, claudeToggle, noteToggle, planToggle, pathsToggle)

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

// enrichVisibleProjects creates commands to fetch git status for visible projects.
func (m *sessionizeModel) enrichVisibleProjects() tea.Cmd {
	if !m.showGitStatus && !m.showBranch {
		return nil
	}

	var cmds []tea.Cmd
	start, end := m.getVisibleRange()

	for i := start; i < end; i++ {
		if i < len(m.filtered) {
			proj := m.filtered[i]
			if proj.EnrichmentStatus["git"] == "" {
				proj.EnrichmentStatus["git"] = "loading"
				cmds = append(cmds, fetchGitStatusCmd(proj.Path))
			}
		}
	}

	return tea.Batch(cmds...)
}

// getVisibleRange calculates the start and end indices of visible projects.
func (m *sessionizeModel) getVisibleRange() (int, int) {
	visibleHeight := m.height - 10
	if visibleHeight < 5 {
		visibleHeight = 5
	}

	start := 0
	end := len(m.filtered)

	if end > visibleHeight {
		if m.cursor < visibleHeight/2 {
			start = 0
		} else if m.cursor >= len(m.filtered)-visibleHeight/2 {
			start = len(m.filtered) - visibleHeight
		} else {
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

	return start, end
}
