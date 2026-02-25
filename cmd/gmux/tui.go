package main

import (
	"context"
	"fmt"
	grovecontext "github.com/grovetools/cx/pkg/context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/pkg/models"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/keymap"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/tmux"
)

var pageStyle = lipgloss.NewStyle()

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
	configDir                string            // configuration directory
	keyMap          map[string]string // path -> key mapping
	runningSessions map[string]bool   // map[sessionName] -> true
	currentSession  string            // name of the current tmux session
	width                    int
	height                   int
	keys                     sessionizeKeyMap  // keybindings for this TUI
	availableKeys []string
	sessions      []models.TmuxSession
	help          help.Model

	// Focus mode state
	ecosystemPickerMode bool // True when showing only ecosystems for selection
	focusedProject      *manager.SessionizeProject
	worktreesFolded     bool // Whether worktrees are hidden/collapsed
	foldedPaths         map[string]bool // Paths that are currently folded closed
	hasChildren         map[string]bool // Cheap tracking of parent nodes
	sequence            *keymap.SequenceState // Core standard vim sequence tracker

	// View toggles
	showGitStatus  bool // Whether to fetch and show Git status
	showBranch     bool // Whether to show branch names
	showNoteCounts bool // Whether to fetch and show note counts
	showPlanStats  bool // Whether to show plan stats from grove-flow
	showOnHold         bool // Whether to show on-hold plans
	pathDisplayMode    int  // 0=no paths, 1=compact (~), 2=full paths
	showRelease    bool // Whether to show release info column
	showBinary     bool // Whether to show active binary column
	showLink       bool // Whether to show repository link column
	showCx         bool // Whether to show CX (context) column

	// Filter mode
	filterDirty bool // Whether to filter to only projects with dirty Git status
	filterGroup bool // Whether to filter to only projects in the active group

	// Context-only projects (shown but not selectable during filtered search in focus mode)
	contextOnlyPaths map[string]bool

	// Status message
	statusMessage string
	statusTimeout time.Time

	// Loading state
	isLoading    bool
	usedCache    bool      // Whether we loaded from cache on startup
	spinnerFrame int       // Current frame of the spinner animation
	lastSpinTime time.Time // Last time spinner was updated

	// Enrichment loading state
	enrichmentLoading map[string]bool // tracks which enrichments are currently loading

	// Context rules state
	rulesState map[string]grovecontext.RuleStatus // path -> status

	// New group creation mode (inline)
	newGroupMode   bool
	newGroupStep   int    // 0 = entering name, 1 = entering prefix
	newGroupName   string
	newGroupPrefix string

	// Map to group mode
	mapToGroupMode    bool
	mapToGroupOptions []string
	mapToGroupCursor  int
	mapToGroupPath    string
}

func newSessionizeModel(projects []*manager.SessionizeProject, searchPaths []string, mgr *tmux.Manager, configDir string, usedCache bool, cwdFocusPath string) sessionizeModel {
	// Create text input for filtering (start unfocused)
	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = core_theme.DefaultTheme.Muted.Render("󰍉 ")
	ti.CharLimit = 256
	ti.Width = 50

	// Determine initial group based on context priority
	initialGroup := mgr.GetActiveGroup()
	autoEnableGroupFilter := false
	clearFocus := false

	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		cwdGroup := mgr.FindGroupForPath(cwd)
		if cwdGroup != "" && cwdGroup != "default" {
			// CWD matches a non-default group - auto-enable filter
			autoEnableGroupFilter = true
		} else if _, err := workspace.GetProjectByPath(cwd); err == nil {
			// Workspace in default group or unmapped - clear focus
			clearFocus = true
		}
	}

	// Set the active group before fetching sessions
	mgr.SetActiveGroup(initialGroup)
	_ = mgr.SetLastAccessedGroup(initialGroup)

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

	// Create running sessions map and get current session name if we're in tmux
	runningSessions := make(map[string]bool)
	currentSession := ""
	if os.Getenv("TMUX") != "" {
		client, err := tmuxclient.NewClient()
		if err == nil {
			ctx := context.Background()
			if current, err := client.GetCurrentSession(ctx); err == nil {
				currentSession = current
			}
			// Populate running sessions synchronously to avoid reordering flash
			if sessionNames, err := client.ListSessions(ctx); err == nil {
				for _, name := range sessionNames {
					runningSessions[name] = true
				}
			}
		}
	}

	helpModel := help.NewBuilder().
		WithKeys(sessionizeKeys).
		WithTitle("Project Sessionizer - Help").
		WithLegend("Icons: " + core_theme.IconBullet + " current • " + core_theme.IconBullet + " active • " + core_theme.IconEcosystem + " ecosystem • " + core_theme.IconRepo + " repo • " + core_theme.IconWorktree + " worktree • " + core_theme.IconGitBranch + " branch").
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
	foldedPaths := make(map[string]bool)
	// Set sensible defaults for toggles
	showGitStatus := true
	showBranch := true
	showNoteCounts := true
	showPlanStats := true
	pathDisplayMode := 1 // Default to compact paths (~)
	showRelease := false // Default off - expensive operation
	showBinary := false  // Default off - expensive operation
	showLink := false    // Default off - takes space
	showCx := true       // Default on - show CX column when data available
	if state, err := manager.LoadState(configDir); err == nil {
		// Prioritize CWD focus path over saved state, but clear if in unmapped workspace
		if clearFocus {
			state.FocusedEcosystemPath = ""
		} else if cwdFocusPath != "" {
			state.FocusedEcosystemPath = cwdFocusPath
		}
		if state.FocusedEcosystemPath != "" {
			// Find the project with this path using normalized path comparison
			normalizedFocusPath, err := pathutil.NormalizeForLookup(state.FocusedEcosystemPath)
			if err == nil {
				for path, proj := range projectMap {
					normalizedPath, err := pathutil.NormalizeForLookup(path)
					if err == nil && normalizedPath == normalizedFocusPath {
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
		if state.ShowNoteCounts != nil {
			showNoteCounts = *state.ShowNoteCounts
		}
		if state.ShowPlanStats != nil {
			showPlanStats = *state.ShowPlanStats
		}
		if state.PathDisplayMode != nil {
			pathDisplayMode = *state.PathDisplayMode
		}
		if state.ShowRelease != nil {
			showRelease = *state.ShowRelease
		}
		if state.ShowBinary != nil {
			showBinary = *state.ShowBinary
		}
		if state.ShowLink != nil {
			showLink = *state.ShowLink
		}
		if state.ShowCx != nil {
			showCx = *state.ShowCx
		}
		for _, p := range state.FoldedPaths {
			foldedPaths[p] = true
		}
	}

	m := sessionizeModel{
		rulesState:               make(map[string]grovecontext.RuleStatus),
		projects:                 projects,
		filtered:                 projects,
		projectMap:               projectMap,
		filterInput:              ti,
		searchPaths:              searchPaths,
		manager:                  mgr,
		configDir:                configDir,
		keyMap:          keyMap,
		runningSessions: runningSessions,
		currentSession:  currentSession,
		width:                    0,
		height:                   0,
		keys:                     sessionizeKeys,
		cursor:                   0,
		availableKeys:            availableKeys,
		sessions:                 sessions,
		help:                     helpModel,
		worktreesFolded:          worktreesFolded,
		showGitStatus:            showGitStatus,
		showBranch:               showBranch,
		showNoteCounts:           showNoteCounts,
		showPlanStats:            showPlanStats,
		showOnHold:               false, // Default to hiding on-hold plans
		pathDisplayMode:          pathDisplayMode,
		showRelease:              showRelease,
		showBinary:               showBinary,
		showLink:                 showLink,
		showCx:                   showCx,
		filterGroup:              autoEnableGroupFilter,
		// Clear ecosystem focus when group filter is auto-enabled
		focusedProject: func() *manager.SessionizeProject {
			if autoEnableGroupFilter {
				return nil
			}
			return focusedProject
		}(),
		contextOnlyPaths:         make(map[string]bool),
		usedCache:                usedCache,
		isLoading:                usedCache, // Start as loading if we used cache (will refresh in background)
		enrichmentLoading:        make(map[string]bool),
		foldedPaths:              foldedPaths,
		hasChildren:              make(map[string]bool),
		sequence:                 keymap.NewSequenceState(),
	}

	// Synchronously apply initial filters to prevent UI flash
	m.updateFiltered()
	m.moveCursorToFirstSelectable()

	// Attempt to position cursor on the current project
	if cwd != "" {
		if node, err := workspace.GetProjectByPath(cwd); err == nil && node != nil {
			normalizedProject, _ := pathutil.NormalizeForLookup(filepath.Clean(node.Path))
			for i, p := range m.filtered {
				normalizedPath, _ := pathutil.NormalizeForLookup(filepath.Clean(p.Path))
				if normalizedPath == normalizedProject && !m.contextOnlyPaths[p.Path] {
					m.cursor = i
					break
				}
			}
		}
	}

	return m
}

// cycleGroup switches to the next or previous workspace group
func (m *sessionizeModel) cycleGroup(dir int) {
	groups := m.manager.GetGroups()
	if len(groups) <= 1 {
		m.statusMessage = "No other groups configured"
		m.statusTimeout = time.Now().Add(2 * time.Second)
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

	// Reload sessions and keyMap
	m.sessions, _ = m.manager.GetSessions()
	m.keyMap = make(map[string]string)
	for _, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				m.keyMap[filepath.Clean(absPath)] = s.Key
			}
		}
	}

	if m.filterGroup {
		m.updateFiltered()
		m.cursor = 0
		m.moveCursorToFirstSelectable()
	}
}

// buildState creates a SessionizerState from the current model
func (m sessionizeModel) buildState() *manager.SessionizerState {
	state := &manager.SessionizerState{
		FocusedEcosystemPath: "",
		WorktreesFolded:      m.worktreesFolded,
		ShowGitStatus:        boolPtr(m.showGitStatus),
		ShowBranch:           boolPtr(m.showBranch),
		ShowNoteCounts:       boolPtr(m.showNoteCounts),
		ShowPlanStats:        boolPtr(m.showPlanStats),
		PathDisplayMode:      intPtr(m.pathDisplayMode),
		ShowRelease:          boolPtr(m.showRelease),
		ShowBinary:           boolPtr(m.showBinary),
		ShowLink:             boolPtr(m.showLink),
		ShowCx:               boolPtr(m.showCx),
	}
	if m.focusedProject != nil {
		state.FocusedEcosystemPath = m.focusedProject.Path
	}
	for path, folded := range m.foldedPaths {
		if folded {
			state.FoldedPaths = append(state.FoldedPaths, path)
		}
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
		subscribeToDaemonCmd(), // Subscribe to daemon state updates (if daemon is running)
		updateDaemonFocusCmd(m.getVisiblePaths()), // Set initial focus for daemon
	}

	// Only do full project discovery if we didn't load from cache
	// If we used cache, we already have projects with enrichment data
	if !m.usedCache {
		cmds = append(cmds, fetchProjectsCmd(m.manager, m.configDir))
	} else {
		// if we used cache, we have projects, so we can fetch rules state
		cmds = append(cmds, fetchRulesStateCmd(m.projects))
	}

	// Use daemon's cached data if available (fast startup)
	// Only fetch if daemon didn't provide the data
	hasGitStatus, hasNoteCounts, hasPlanStats := false, false, false
	for _, p := range m.projects {
		if p.GitStatus != nil {
			hasGitStatus = true
		}
		if p.NoteCounts != nil {
			hasNoteCounts = true
		}
		if p.PlanStats != nil {
			hasPlanStats = true
		}
	}

	if m.showNoteCounts && !hasNoteCounts {
		m.enrichmentLoading["notes"] = true
		cmds = append(cmds, fetchAllNoteCountsCmd())
	}
	if m.showPlanStats && !hasPlanStats {
		m.enrichmentLoading["plans"] = true
		cmds = append(cmds, fetchAllPlanStatsCmd())
	}
	if m.showGitStatus && !hasGitStatus {
		m.enrichmentLoading["git"] = true
		cmds = append(cmds, fetchAllGitStatusesCmd(m.projects))
	}
	if m.showRelease {
		m.enrichmentLoading["release"] = true
		cmds = append(cmds, fetchAllReleaseInfoCmd(m.projects))
	}
	if m.showBinary {
		m.enrichmentLoading["binary"] = true
		cmds = append(cmds, fetchAllBinaryStatusCmd(m.projects))
	}
	if m.showLink {
		m.enrichmentLoading["link"] = true
		cmds = append(cmds, fetchAllRemoteURLsCmd(m.projects))
	}
	// Always fetch CX stats in the background to augment the CX column
	m.enrichmentLoading["cxstats"] = true
	cmds = append(cmds, fetchCxPerLineStatsCmd(m.projects))

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
		// Re-fetch rules state now that we have accurate branch info from git status
		return m, fetchRulesStateCmd(m.projects)

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

	case releaseInfoMapMsg:
		for path, info := range msg.releases {
			if proj, ok := m.projectMap[path]; ok {
				proj.ReleaseInfo = info
			}
		}
		m.enrichmentLoading["release"] = false
		return m, nil

	case binaryStatusMapMsg:
		for path, status := range msg.statuses {
			if proj, ok := m.projectMap[path]; ok {
				proj.ActiveBinary = status
			}
		}
		m.enrichmentLoading["binary"] = false
		return m, nil

	case cxStatsMapMsg:
		// First, clear all existing CxStats (projects removed from context won't be in the new stats)
		for _, proj := range m.projects {
			proj.CxStats = nil
		}
		// Then apply the new stats
		for path, stats := range msg.stats {
			if proj, ok := m.projectMap[path]; ok {
				proj.CxStats = stats
			}
		}
		m.enrichmentLoading["cxstats"] = false
		return m, nil

	case remoteURLMapMsg:
		for path, url := range msg.urls {
			if proj, ok := m.projectMap[path]; ok {
				proj.GitRemoteURL = url
			}
		}
		m.enrichmentLoading["link"] = false
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

		// Re-fetch CX stats after project refresh
		m.enrichmentLoading["cxstats"] = true
		return m, tea.Batch(
			m.enrichVisibleProjects(),
			fetchRulesStateCmd(m.projects),
			fetchCxPerLineStatsCmd(m.projects),
			spinnerTickCmd(),
		)

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
		// Refresh rules state and CX stats (rules file changed)
		m.enrichmentLoading["cxstats"] = true
		return m, tea.Batch(
			clearStatusCmd(2*time.Second),
			fetchRulesStateCmd(m.projects),
			fetchCxPerLineStatsCmd(m.projects),
			spinnerTickCmd(),
		)

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

	case daemonStateUpdateMsg:
		// Handle real-time state updates from daemon
		// Update enrichment data for workspaces that changed
		if msg.update.Workspaces != nil {
			for _, ew := range msg.update.Workspaces {
				if proj, ok := m.projectMap[ew.Path]; ok {
					if ew.GitStatus != nil {
						proj.GitStatus = ew.GitStatus
						proj.EnrichmentStatus["git"] = "done"
					}
					if ew.GitRemoteURL != "" {
						proj.GitRemoteURL = ew.GitRemoteURL
					}
					// Map NoteCounts from daemon
					if ew.NoteCounts != nil {
						proj.NoteCounts = &manager.NoteCounts{
							Current:    ew.NoteCounts.Current,
							Issues:     ew.NoteCounts.Issues,
							Inbox:      ew.NoteCounts.Inbox,
							Docs:       ew.NoteCounts.Docs,
							Completed:  ew.NoteCounts.Completed,
							Review:     ew.NoteCounts.Review,
							InProgress: ew.NoteCounts.InProgress,
							Other:      ew.NoteCounts.Other,
						}
					}
					// Map PlanStats from daemon
					if ew.PlanStats != nil {
						proj.PlanStats = &manager.PlanStats{
							TotalPlans: ew.PlanStats.TotalPlans,
							ActivePlan: ew.PlanStats.ActivePlan,
							Running:    ew.PlanStats.Running,
							Pending:    ew.PlanStats.Pending,
							Completed:  ew.PlanStats.Completed,
							Failed:     ew.PlanStats.Failed,
							Todo:       ew.PlanStats.Todo,
							Hold:       ew.PlanStats.Hold,
							Abandoned:  ew.PlanStats.Abandoned,
							PlanStatus: ew.PlanStats.PlanStatus,
						}
					}
				}
			}
		}
		// Continue listening for more updates
		return m, listenToDaemonCmd()

	case daemonStreamStartedMsg:
		// Daemon stream is ready, start listening for updates
		return m, listenToDaemonCmd()

	case daemonStreamErrorMsg:
		// Stream closed or errored - don't restart, just continue without streaming
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
			updateDaemonFocusCmd(m.getVisiblePaths()), // Keep daemon focus in sync
		}

		// Track if we're starting any enrichment
		startedEnrichment := false

		// Only refresh fast/dynamic data on tick.
		// Expensive/static data (release, binary, link, cxstats) only refresh on toggle or manual refresh.
		// Skip enrichment fetches if daemon is streaming updates (it pushes all enrichment data)
		if !daemonStreamState.started {
			if m.showGitStatus {
				m.enrichmentLoading["git"] = true
				startedEnrichment = true
				cmds = append(cmds, fetchAllGitStatusesCmd(m.projects))
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
		}
		// NOTE: release, binary, link, and cxstats are NOT refreshed on tick.
		// They spawn many processes and contain relatively static data.
		// Users can press ctrl+r to force a full refresh.

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
		// If help is visible, pass navigation keys through for scrolling
		if m.help.ShowAll {
			switch {
			case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Help), msg.Type == tea.KeyEsc:
				m.help.Toggle()
				return m, nil
			default:
				var cmd tea.Cmd
				m.help, cmd = m.help.Update(msg)
				return m, cmd
			}
		}

		// Handle map to group mode
		if m.mapToGroupMode {
			switch {
			case msg.Type == tea.KeyEsc:
				m.mapToGroupMode = false
				m.statusMessage = "Cancelled"
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)

			case key.Matches(msg, m.keys.Up):
				if m.mapToGroupCursor > 0 {
					m.mapToGroupCursor--
				} else {
					m.mapToGroupCursor = len(m.mapToGroupOptions) - 1
				}
				return m, nil

			case key.Matches(msg, m.keys.Down):
				if m.mapToGroupCursor < len(m.mapToGroupOptions)-1 {
					m.mapToGroupCursor++
				} else {
					m.mapToGroupCursor = 0
				}
				return m, nil

			case msg.Type == tea.KeyEnter, msg.Type == tea.KeySpace:
				targetGroup := m.mapToGroupOptions[m.mapToGroupCursor]
				m.executeMapToGroup(targetGroup)
				m.mapToGroupMode = false
				return m, nil
			}
			return m, nil
		}

		// Handle new group mode
		if m.newGroupMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.newGroupMode = false
				m.statusMessage = "Cancelled"
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			case tea.KeyEnter:
				if m.newGroupStep == 0 {
					if m.newGroupName == "" {
						m.statusMessage = "Group name cannot be empty"
						m.statusTimeout = time.Now().Add(2 * time.Second)
						return m, nil
					}
					m.newGroupStep = 1
					m.statusMessage = "Enter prefix key (optional, e.g. '<grove> g'):"
					m.statusTimeout = time.Now().Add(30 * time.Second)
				} else {
					// Create the group
					if err := m.manager.CreateGroup(m.newGroupName, m.newGroupPrefix); err != nil {
						m.statusMessage = fmt.Sprintf("Error: %v", err)
					} else {
						m.manager.SetActiveGroup(m.newGroupName)
						_ = m.manager.SetLastAccessedGroup(m.newGroupName)
						// Reload sessions for the new group
						m.sessions, _ = m.manager.GetSessions()
						m.keyMap = make(map[string]string)
						for _, s := range m.sessions {
							if s.Path != "" {
								expandedPath := expandPath(s.Path)
								absPath, err := filepath.Abs(expandedPath)
								if err == nil {
									m.keyMap[filepath.Clean(absPath)] = s.Key
								}
							}
						}
						if m.filterGroup {
							m.updateFiltered()
							m.cursor = 0
							m.moveCursorToFirstSelectable()
						}
						m.statusMessage = fmt.Sprintf("Created and switched to group '%s'", m.newGroupName)
					}
					m.newGroupMode = false
					m.statusTimeout = time.Now().Add(2 * time.Second)
				}
				return m, clearStatusCmd(2 * time.Second)
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

		// Process standard vim sequences (folding, dd)
		res, idx := m.sequence.Process(msg,
			m.keys.FoldOpen,
			m.keys.FoldClose,
			m.keys.FoldToggle,
			m.keys.FoldOpenAll,
			m.keys.FoldCloseAll,
			m.keys.Delete,
		)
		switch res {
		case keymap.SequenceMatch:
			m.sequence.Clear()
			switch idx {
			case 0: // FoldOpen (zo)
				if m.cursor < len(m.filtered) {
					delete(m.foldedPaths, m.filtered[m.cursor].Path)
					_ = m.buildState().Save(m.configDir)
					m.updateFiltered()
				}
			case 1: // FoldClose (zc)
				if m.cursor < len(m.filtered) {
					p := m.filtered[m.cursor]
					if m.hasChildren[p.Path] {
						m.foldedPaths[p.Path] = true
						_ = m.buildState().Save(m.configDir)
						m.updateFiltered()
					}
				}
			case 2: // FoldToggle (za)
				if m.cursor < len(m.filtered) {
					p := m.filtered[m.cursor]
					if m.hasChildren[p.Path] {
						if m.foldedPaths[p.Path] {
							delete(m.foldedPaths, p.Path)
						} else {
							m.foldedPaths[p.Path] = true
						}
						_ = m.buildState().Save(m.configDir)
						m.updateFiltered()
					}
				}
			case 3: // FoldOpenAll (zR)
				m.foldedPaths = make(map[string]bool)
				_ = m.buildState().Save(m.configDir)
				m.updateFiltered()
			case 4: // FoldCloseAll (zM)
				for path := range m.hasChildren {
					m.foldedPaths[path] = true
				}
				_ = m.buildState().Save(m.configDir)
				m.updateFiltered()
			case 5: // Delete (dd) - clear filter
				m.filterInput.SetValue("")
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
			}
			return m, nil
		case keymap.SequencePending:
			return m, nil // Wait for the rest of the sequence
		}
		m.sequence.Clear()

		// Handle non-letter key bindings that should work even in search mode
		switch {
		case key.Matches(msg, sessionizeKeys.RefreshProjects):
			m.isLoading = true
			return m, tea.Batch(spinnerTickCmd(), fetchProjectsCmd(m.manager, m.configDir))

		case key.Matches(msg, sessionizeKeys.ClearFocus):
			// Clear group filter if active
			if m.filterGroup {
				m.filterGroup = false
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
				return m, updateDaemonFocusCmd(m.getVisiblePaths())
			}
			// Clear ecosystem picker mode if active
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
				return m, updateDaemonFocusCmd(m.getVisiblePaths())
			}
			// Clear focused project if set
			if m.focusedProject != nil {
				m.focusedProject = nil
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()

				// Clear the focused ecosystem from state
				_ = m.buildState().Save(m.configDir)
			}
			return m, updateDaemonFocusCmd(m.getVisiblePaths())

		case key.Matches(msg, sessionizeKeys.FilterDirty):
			// Toggle dirty filter
			m.filterDirty = !m.filterDirty
			// Clear text filter to make them mutually exclusive
			m.filterInput.SetValue("")
			if m.filterDirty {
				m.filterGroup = false
			}
			m.updateFiltered()
			m.cursor = 0
			m.moveCursorToFirstSelectable()
			return m, nil

		case key.Matches(msg, sessionizeKeys.FilterGroup):
			m.filterGroup = !m.filterGroup
			m.filterInput.SetValue("") // Clear text filter
			if m.filterGroup {
				m.filterDirty = false
				// Check if there are any mappings in the current group
				if len(m.keyMap) == 0 {
					m.statusMessage = fmt.Sprintf("No key mappings in group '%s'. Press Tab to switch groups.", m.manager.GetActiveGroup())
					m.statusTimeout = time.Now().Add(3 * time.Second)
				}
			}
			m.updateFiltered()
			m.cursor = 0
			m.moveCursorToFirstSelectable()
			return m, nil

		case key.Matches(msg, sessionizeKeys.NextGroup):
			m.cycleGroup(1)
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, sessionizeKeys.PrevGroup):
			m.cycleGroup(-1)
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, sessionizeKeys.ToggleHold):
			// Toggle on-hold plans visibility
			m.showOnHold = !m.showOnHold
			m.updateFiltered()
			m.cursor = 0
			m.moveCursorToFirstSelectable()
			return m, nil
		}

		// Check if filter input is focused and handle special keys
		if m.filterInput.Focused() {
			switch msg.Type {
			case tea.KeyEsc:
				// Vim-style: Escape exits search but preserves filter value
				// Stay in ecosystem picker mode if active - second Escape will cancel it
				m.filterInput.Blur()
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
					return m, updateDaemonFocusCmd(m.getVisiblePaths())
				}
				// Vim-style: Enter confirms filter and blurs (keeps value), press Enter again to select
				m.filterInput.Blur()
				return m, nil
			case tea.KeyUp:
				// Navigate up while filtering
				if m.cursor > 0 {
					m.cursor--
				}
				return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))
			case tea.KeyDown:
				// Navigate down while filtering
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))
			default:
				// Let filter input handle all other keys when focused
				prevValue := m.filterInput.Value()
				m.filterInput, cmd = m.filterInput.Update(msg)

				// If the filter changed, update filtered list
				if m.filterInput.Value() != prevValue {
					m.updateFiltered()
					m.cursor = 0
					m.moveCursorToFirstSelectable()
					return m, tea.Batch(cmd, updateDaemonFocusCmd(m.getVisiblePaths()))
				}
				return m, cmd
			}
		}

		// Normal mode (when filter is not focused)
		// Use key.Matches() for all keybindings to respect user config overrides
		switch {
		case key.Matches(msg, m.keys.Up):
			m.moveCursorUp()
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.Down):
			m.moveCursorDown()
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.PageUp):
			// Page up (vim-style)
			pageSize := 10
			m.cursor -= pageSize
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.PageDown):
			// Page down (vim-style)
			pageSize := 10
			m.cursor += pageSize
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.Bottom):
			m.cursor = len(m.filtered) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.CloseSession):
			// Close session
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

		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Search):
			// Focus filter input for search
			// Clear dirty filter to make them mutually exclusive
			if m.filterDirty {
				m.filterDirty = false
			}
			m.filterInput.Focus()
			return m, textinput.Blink

		// Vim-style: 'i' re-enters search (insert mode) if filter has value
		case msg.Type == tea.KeyRunes && string(msg.Runes) == "i":
			if m.filterInput.Value() != "" {
				m.filterInput.Focus()
				return m, textinput.Blink
			}

		case key.Matches(msg, m.keys.FocusEcosystem):
			m.filterGroup = false // Clear group filter when entering ecosystem picker
			m.ecosystemPickerMode = true
			m.updateFiltered()
			m.cursor = 0
			m.moveCursorToFirstSelectable()
			return m, updateDaemonFocusCmd(m.getVisiblePaths())

		case key.Matches(msg, m.keys.FocusEcosystemCwd):
			// Focus on the ecosystem (or ecosystem worktree) containing the current working directory
			return m, m.focusEcosystemForPath("")

		case key.Matches(msg, m.keys.FocusEcosystemCursor):
			// Focus on the ecosystem (or ecosystem worktree) containing the project under cursor
			if m.cursor >= len(m.filtered) {
				return m, nil
			}
			project := m.filtered[m.cursor]
			if project == nil {
				return m, nil
			}
			return m, m.focusEcosystemForPath(project.Path)

		case key.Matches(msg, m.keys.OpenEcosystem):
			// Open (focus into) the ecosystem at cursor if it's an ecosystem/ecosystem-worktree
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				if project.Kind == workspace.KindEcosystemRoot || project.Kind == workspace.KindEcosystemWorktree {
					m.filterGroup = false
					m.filterDirty = false
					m.filterInput.SetValue("")
					m.ecosystemPickerMode = false
					m.focusedProject = project
					m.updateFiltered()
					m.cursor = 0
					m.moveCursorToFirstSelectable()
					_ = m.buildState().Save(m.configDir)
					return m, updateDaemonFocusCmd(m.getVisiblePaths())
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleGitStatus):
			m.showGitStatus = !m.showGitStatus
			_ = m.buildState().Save(m.configDir)
			return m, m.enrichVisibleProjects()

		case key.Matches(msg, m.keys.ToggleBranch):
			m.showBranch = !m.showBranch
			_ = m.buildState().Save(m.configDir)
			return m, m.enrichVisibleProjects()

		case key.Matches(msg, m.keys.ToggleHotContext):
			if m.cursor < len(m.filtered) {
				selected := m.filtered[m.cursor]
				currentStatus := m.rulesState[selected.Path]
				return m, toggleRuleCmd(selected, "hot", currentStatus)
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleNoteCounts):
			m.showNoteCounts = !m.showNoteCounts
			_ = m.buildState().Save(m.configDir)
			// Refetch note counts if toggled on
			if m.showNoteCounts {
				return m, fetchAllNoteCountsCmd()
			}
			return m, nil

		case key.Matches(msg, m.keys.TogglePlanStats):
			m.showPlanStats = !m.showPlanStats
			_ = m.buildState().Save(m.configDir)
			// Refetch plan stats if toggled on
			if m.showPlanStats {
				return m, fetchAllPlanStatsCmd()
			}
			return m, nil

		case key.Matches(msg, m.keys.TogglePaths):
			m.pathDisplayMode = (m.pathDisplayMode + 1) % 3
			_ = m.buildState().Save(m.configDir)
			return m, nil

		case key.Matches(msg, m.keys.ToggleRelease):
			m.showRelease = !m.showRelease
			_ = m.buildState().Save(m.configDir)
			if m.showRelease {
				m.enrichmentLoading["release"] = true
				return m, tea.Batch(spinnerTickCmd(), fetchAllReleaseInfoCmd(m.projects))
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleBinary):
			m.showBinary = !m.showBinary
			_ = m.buildState().Save(m.configDir)
			if m.showBinary {
				m.enrichmentLoading["binary"] = true
				return m, tea.Batch(spinnerTickCmd(), fetchAllBinaryStatusCmd(m.projects))
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleLink):
			m.showLink = !m.showLink
			_ = m.buildState().Save(m.configDir)
			if m.showLink {
				m.enrichmentLoading["link"] = true
				return m, tea.Batch(spinnerTickCmd(), fetchAllRemoteURLsCmd(m.projects))
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleCx):
			m.showCx = !m.showCx
			_ = m.buildState().Save(m.configDir)
			return m, nil

		case key.Matches(msg, m.keys.ManageGroups):
			// Switch to groups management view
			return m, func() tea.Msg { return switchViewMsg{to: viewGroups} }

		case key.Matches(msg, m.keys.NewGroup):
			// Enter new group creation mode (inline in sessionize)
			m.newGroupMode = true
			m.newGroupStep = 0
			m.newGroupName = ""
			m.newGroupPrefix = ""
			m.statusMessage = "Enter new group name:"
			m.statusTimeout = time.Now().Add(30 * time.Second)
			return m, nil

		case key.Matches(msg, m.keys.MapToGroup):
			// Map current project to a group (show group picker)
			if m.cursor >= len(m.filtered) {
				return m, nil
			}
			project := m.filtered[m.cursor]
			if project == nil {
				return m, nil
			}
			// Build list of target groups
			m.mapToGroupOptions = []string{}
			currentGroup := m.manager.GetActiveGroup()
			for _, g := range m.manager.GetGroups() {
				if g != currentGroup {
					m.mapToGroupOptions = append(m.mapToGroupOptions, g)
				}
			}
			if len(m.mapToGroupOptions) == 0 {
				m.statusMessage = "No other groups to map to"
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			}
			m.mapToGroupMode = true
			m.mapToGroupCursor = 0
			m.mapToGroupPath = project.Path
			m.statusMessage = fmt.Sprintf("Map '%s' to group:", project.Name)
			m.statusTimeout = time.Now().Add(30 * time.Second)
			return m, nil

		case key.Matches(msg, m.keys.GoToMappingCursor):
			// Switch to the group containing this project's mapping and apply group filter
			if m.cursor >= len(m.filtered) {
				return m, nil
			}
			project := m.filtered[m.cursor]
			if project == nil {
				return m, nil
			}
			// Check if project has a direct key mapping (not just inside a mapped parent)
			cleanPath := filepath.Clean(project.Path)
			normalizedPath, _ := pathutil.NormalizeForLookup(cleanPath)
			hasDirectMapping := false
			for path := range m.keyMap {
				normPath, _ := pathutil.NormalizeForLookup(path)
				if normPath == normalizedPath {
					hasDirectMapping = true
					break
				}
			}
			// If not directly mapped, prompt to add a mapping
			if !hasDirectMapping {
				return m, func() tea.Msg {
					return initiateMappingMsg{
						project:  project,
						returnTo: viewSessionize,
					}
				}
			}
			return m, m.goToMappingForPath(project.Path)

		case key.Matches(msg, m.keys.GoToMappingCwd):
			// Switch to the group containing CWD's mapping and apply group filter
			cwd, err := os.Getwd()
			if err != nil {
				m.statusMessage = "Could not get current directory"
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			}
			// Find the project for CWD
			cwdNormalized, _ := pathutil.NormalizeForLookup(cwd)
			var cwdProject *manager.SessionizeProject
			for _, p := range m.projects {
				pNormalized, _ := pathutil.NormalizeForLookup(p.Path)
				if pNormalized == cwdNormalized || strings.HasPrefix(cwdNormalized, pNormalized+string(filepath.Separator)) {
					cwdProject = p
					break
				}
			}
			if cwdProject == nil {
				m.statusMessage = "CWD is not inside a known project"
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			}
			// Check if CWD project has a direct key mapping
			cleanPath := filepath.Clean(cwdProject.Path)
			normalizedPath, _ := pathutil.NormalizeForLookup(cleanPath)
			hasDirectMapping := false
			for path := range m.keyMap {
				normPath, _ := pathutil.NormalizeForLookup(path)
				if normPath == normalizedPath {
					hasDirectMapping = true
					break
				}
			}
			// If not directly mapped, prompt to add a mapping
			if !hasDirectMapping {
				return m, func() tea.Msg {
					return initiateMappingMsg{
						project:  cwdProject,
						returnTo: viewSessionize,
					}
				}
			}
			return m, m.goToMappingForPath(cwdProject.Path)

		case key.Matches(msg, m.keys.ToggleWorktrees):
			m.worktreesFolded = !m.worktreesFolded
			m.updateFiltered()
			_ = m.buildState().Save(m.configDir)
			return m, tea.Batch(m.enrichVisibleProjects(), updateDaemonFocusCmd(m.getVisiblePaths()))

		case key.Matches(msg, m.keys.EditKey):
			// Enter key editing mode by delegating to the manage view
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				return m, func() tea.Msg {
					return initiateMappingMsg{
						project:  project,
						returnTo: viewSessionize,
					}
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.ToggleKey):
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				cleanPath := filepath.Clean(project.Path)
				normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
				if err != nil {
					return m, nil
				}

				mappedKey := ""
				for path, k := range m.keyMap {
					normPath, err := pathutil.NormalizeForLookup(path)
					if err == nil && normPath == normalizedCleanPath {
						mappedKey = k
						break
					}
				}

				if mappedKey != "" {
					m.clearKeyMapping(project.Path)
					m.statusMessage = fmt.Sprintf("Unmapped key '%s'", mappedKey)
				} else {
					// Find first available key
					usedKeys := make(map[string]bool)
					for _, k := range m.keyMap {
						usedKeys[k] = true
					}
					for _, k := range m.availableKeys {
						if !usedKeys[k] {
							m.updateKeyMapping(project.Path, k)
							m.statusMessage = fmt.Sprintf("Mapped '%s' to key '%s'", project.Name, k)
							break
						}
					}
					if m.statusMessage == "" {
						m.statusMessage = "No available keys!"
					}
				}
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			}
			return m, nil

		case key.Matches(msg, m.keys.ClearKey):
			// Clear key mapping for the selected project
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				cleanPath := filepath.Clean(project.Path)
				normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
				if err != nil {
					return m, nil
				}

				// Find the mapped key
				mappedKey := ""
				for path, k := range m.keyMap {
					normPath, err := pathutil.NormalizeForLookup(path)
					if err == nil && normPath == normalizedCleanPath {
						mappedKey = k
						break
					}
				}

				if mappedKey != "" {
					m.clearKeyMapping(project.Path)
					m.statusMessage = fmt.Sprintf("Unmapped key '%s'", mappedKey)
					m.statusTimeout = time.Now().Add(2 * time.Second)
					return m, clearStatusCmd(2 * time.Second)
				} else {
					m.statusMessage = "No key mapped to this project"
					m.statusTimeout = time.Now().Add(2 * time.Second)
					return m, clearStatusCmd(2 * time.Second)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.CopyPath):
			// Yank (copy) the selected project path
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				if err := clipboard.WriteAll(project.Path); err != nil {
					m.statusMessage = fmt.Sprintf("Error copying path: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Copied: %s", project.Path)
				}
				m.statusTimeout = time.Now().Add(2 * time.Second)
				return m, clearStatusCmd(2 * time.Second)
			}
			return m, nil

		case key.Matches(msg, m.keys.Confirm):
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
				return m, updateDaemonFocusCmd(m.getVisiblePaths())
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
			return m, nil

		// Escape when not in filter mode cancels ecosystem picker
		case msg.Type == tea.KeyEsc:
			if m.ecosystemPickerMode {
				m.ecosystemPickerMode = false
				m.filterInput.SetValue("") // Clear any filter
				m.updateFiltered()
				m.cursor = 0
				m.moveCursorToFirstSelectable()
				return m, nil
			}

		case key.Matches(msg, m.keys.Quit):
			// Save cache before quitting to persist enrichment data
			projects := make([]manager.SessionizeProject, len(m.projects))
			for i, p := range m.projects {
				projects[i] = *p
			}
			_ = manager.SaveProjectCache(m.configDir, projects)
			return m, tea.Quit
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
	normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
	if err != nil {
		return // Cannot normalize path
	}
	for i, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			normalizedAbsPath, err := pathutil.NormalizeForLookup(filepath.Clean(absPath))
			if err == nil && normalizedAbsPath == normalizedCleanPath {
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
	normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
	if err != nil {
		return // Cannot normalize path
	}

	// Find if this project has a key mapping
	var targetSessionIndex = -1
	for i, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			normalizedAbsPath, err := pathutil.NormalizeForLookup(filepath.Clean(absPath))
			if err == nil && normalizedAbsPath == normalizedCleanPath {
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

// executeMapToGroup maps the selected project to the first available key in the target group
func (m *sessionizeModel) executeMapToGroup(targetGroup string) {
	// Find current key if it has one (to try preserving it in target group)
	currentKey := ""
	cleanPath := filepath.Clean(m.mapToGroupPath)
	normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
	if err == nil {
		for path, key := range m.keyMap {
			normPath, err := pathutil.NormalizeForLookup(path)
			if err == nil && normPath == normalizedCleanPath {
				currentKey = key
				break
			}
		}
	}

	// Save current group
	currentGroup := m.manager.GetActiveGroup()

	// Switch to target group
	m.manager.SetActiveGroup(targetGroup)
	targetSessions, _ := m.manager.GetSessions()

	// Try to use the same key first if it's available
	targetKey := ""
	if currentKey != "" {
		for _, ts := range targetSessions {
			if ts.Key == currentKey && ts.Path == "" {
				targetKey = ts.Key
				break
			}
		}
	}

	// Fallback to first available key
	if targetKey == "" {
		for _, ts := range targetSessions {
			if ts.Path == "" {
				targetKey = ts.Key
				break
			}
		}
	}

	if targetKey == "" {
		m.statusMessage = fmt.Sprintf("No empty slots in '%s'", targetGroup)
		m.statusTimeout = time.Now().Add(2 * time.Second)
		m.manager.SetActiveGroup(currentGroup) // Restore group
		return
	}

	// Map the project to the selected key
	for i := range targetSessions {
		if targetSessions[i].Key == targetKey {
			targetSessions[i].Path = m.mapToGroupPath
			targetSessions[i].Repository = filepath.Base(m.mapToGroupPath)
			break
		}
	}

	// Save target group
	if err := m.manager.UpdateSessions(targetSessions); err != nil {
		m.statusMessage = fmt.Sprintf("Error: %v", err)
		m.statusTimeout = time.Now().Add(2 * time.Second)
		m.manager.SetActiveGroup(currentGroup)
		return
	}

	// Regenerate bindings
	_ = m.manager.RegenerateBindings()
	_ = reloadTmuxConfig()

	// Switch to the target group
	_ = m.manager.SetLastAccessedGroup(targetGroup)
	// Reload sessions for new group
	m.sessions, _ = m.manager.GetSessions()
	m.keyMap = make(map[string]string)
	for _, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				m.keyMap[filepath.Clean(absPath)] = s.Key
			}
		}
	}

	if m.filterGroup {
		m.updateFiltered()
		m.cursor = 0
		m.moveCursorToFirstSelectable()
	}

	m.statusMessage = fmt.Sprintf("Mapped to '%s' (key %s)", targetGroup, targetKey)
	m.statusTimeout = time.Now().Add(2 * time.Second)
}

func (m *sessionizeModel) updateFiltered() {
	filter := strings.ToLower(m.filterInput.Value())

	if m.ecosystemPickerMode {
		m.filtered = []*manager.SessionizeProject{}
		m.contextOnlyPaths = make(map[string]bool) // Clear context-only paths so all ecosystems are selectable
		m.hasChildren = make(map[string]bool)

		// Separate into main ecosystems and worktrees
		mainEcosystemsMap := make(map[string]*manager.SessionizeProject)
		worktreesByParent := make(map[string][]*manager.SessionizeProject)
		ecoOrder := make(map[string]int)

		for i, p := range m.projects {
			// Track earliest occurrence to sort ecosystems by recent access
			if p.IsEcosystem() && !p.IsWorktree() {
				if _, exists := ecoOrder[p.Path]; !exists {
					ecoOrder[p.Path] = i
				}
			} else if p.RootEcosystemPath != "" {
				if _, exists := ecoOrder[p.RootEcosystemPath]; !exists {
					ecoOrder[p.RootEcosystemPath] = i
				}
			}

			if !p.IsEcosystem() {
				continue
			}

			matchesFilter := filter == "" ||
				strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Path), filter)

			if !matchesFilter {
				continue
			}

			if p.IsWorktree() && p.ParentProjectPath != "" {
				worktreesByParent[p.ParentProjectPath] = append(worktreesByParent[p.ParentProjectPath], p)
			} else {
				mainEcosystemsMap[p.Path] = p
			}
		}

		var mainEcosystems []*manager.SessionizeProject
		for _, eco := range mainEcosystemsMap {
			mainEcosystems = append(mainEcosystems, eco)
		}
		sort.Slice(mainEcosystems, func(i, j int) bool {
			if mainEcosystems[i].Name == "cx-repos" {
				return true
			}
			if mainEcosystems[j].Name == "cx-repos" {
				return false
			}
			idxI, okI := ecoOrder[mainEcosystems[i].Path]
			idxJ, okJ := ecoOrder[mainEcosystems[j].Path]
			if okI && okJ && idxI != idxJ {
				return idxI < idxJ
			}
			return strings.ToLower(mainEcosystems[i].Name) < strings.ToLower(mainEcosystems[j].Name)
		})

		for _, eco := range mainEcosystems {
			m.filtered = append(m.filtered, eco)

			if worktrees, hasWorktrees := worktreesByParent[eco.Path]; hasWorktrees {
				m.hasChildren[eco.Path] = true

				// Skip children if folded (unless text searching)
				if filter == "" && m.foldedPaths[eco.Path] {
					continue
				}

				sort.Slice(worktrees, func(i, j int) bool {
					iIsEcoWT := worktrees[i].Kind == workspace.KindEcosystemWorktree
					jIsEcoWT := worktrees[j].Kind == workspace.KindEcosystemWorktree

					if iIsEcoWT != jIsEcoWT {
						return iIsEcoWT
					}
					return strings.ToLower(worktrees[i].Name) < strings.ToLower(worktrees[j].Name)
				})
				m.filtered = append(m.filtered, worktrees...)
			}
		}
		return
	}

	// 1. Determine base set of projects (Focus vs Global)
	var baseProjects []*manager.SessionizeProject
	if m.focusedProject != nil {
		// Build a set of ecosystem worktree paths to filter out their subprojects
		ecoWorktreePaths := make(map[string]bool)
		for _, p := range m.projects {
			if p.Kind == workspace.KindEcosystemWorktree && p.RootEcosystemPath == m.focusedProject.Path {
				ecoWorktreePaths[p.Path] = true
			}
		}

		for _, p := range m.projects {
			// Skip subprojects that are inside ecosystem worktrees (show only the worktree parent itself)
			if p.ParentEcosystemPath != "" && ecoWorktreePaths[p.ParentEcosystemPath] {
				continue
			}
			if p.Path == m.focusedProject.Path || p.IsChildOf(m.focusedProject.Path) || p.RootEcosystemPath == m.focusedProject.Path || p.ParentEcosystemPath == m.focusedProject.Path {
				baseProjects = append(baseProjects, p)
			}
		}
	} else {
		baseProjects = m.projects
	}

	// 2. Apply text filter to find direct and implicitly related matches
	matchedPaths := make(map[string]bool)
	if filter != "" {
		directMatches := make(map[string]bool)
		for _, p := range baseProjects {
			if strings.Contains(strings.ToLower(p.Name), filter) ||
				strings.Contains(strings.ToLower(p.Path), filter) {
				directMatches[p.Path] = true
			}
		}
		// Include worktrees if their parent matched
		for _, p := range baseProjects {
			if p.IsWorktree() && directMatches[p.ParentProjectPath] {
				matchedPaths[p.Path] = true
			} else if directMatches[p.Path] {
				matchedPaths[p.Path] = true
			}
		}
	} else {
		for _, p := range baseProjects {
			matchedPaths[p.Path] = true
		}
	}

	// 3. Apply attribute filters (Group, Dirty, Hold, Folding)
	pathsToKeep := make(map[string]bool)

	hasKey := func(p *manager.SessionizeProject) bool {
		cleanPath := filepath.Clean(p.Path)
		if _, ok := m.keyMap[cleanPath]; ok {
			return true
		}
		normalized, err := pathutil.NormalizeForLookup(cleanPath)
		if err == nil {
			for keyPath := range m.keyMap {
				normKey, err := pathutil.NormalizeForLookup(keyPath)
				if err == nil && normKey == normalized {
					return true
				}
			}
		}
		return false
	}

	for _, p := range baseProjects {
		if filter != "" && !matchedPaths[p.Path] {
			continue
		}

		keep := true
		if m.filterGroup && !hasKey(p) {
			keep = false
		}
		if m.filterDirty && (p.GetGitStatus() == nil || !p.GetGitStatus().IsDirty) {
			keep = false
		}
		if !m.showOnHold && p.PlanStats != nil && p.PlanStats.PlanStatus == "hold" {
			keep = false
		}
		// Hide worktrees only when folded AND group filter is not active
		// (group filter should always show worktrees for context)
		if filter == "" && m.worktreesFolded && !m.filterGroup && p.IsWorktree() {
			keep = false
		}

		if keep {
			pathsToKeep[p.Path] = true
		}
	}

	// 4. Trace Ancestry to Build Context Tree
	projectByPath := make(map[string]*manager.SessionizeProject)
	for _, p := range baseProjects {
		projectByPath[p.Path] = p
	}

	m.contextOnlyPaths = make(map[string]bool)
	finalIncludedPaths := make(map[string]bool)

	for path := range pathsToKeep {
		finalIncludedPaths[path] = true

		currentPath := path
		for {
			p, exists := projectByPath[currentPath]
			if !exists {
				break
			}

			parentPath := p.GetHierarchicalParent()
			if parentPath == "" || projectByPath[parentPath] == nil {
				if p.ParentEcosystemPath != "" && projectByPath[p.ParentEcosystemPath] != nil {
					parentPath = p.ParentEcosystemPath
				}
			}

			if parentPath == "" || parentPath == currentPath {
				break
			}
			if m.focusedProject != nil && parentPath == m.focusedProject.GetHierarchicalParent() {
				break
			}

			finalIncludedPaths[parentPath] = true
			if !pathsToKeep[parentPath] {
				m.contextOnlyPaths[parentPath] = true // Context-only nodes
			}

			currentPath = parentPath
		}
	}

	if m.focusedProject != nil {
		finalIncludedPaths[m.focusedProject.Path] = true
		if !pathsToKeep[m.focusedProject.Path] {
			m.contextOnlyPaths[m.focusedProject.Path] = true
		}
	}

	// 5. Structure Hierarchical Roots & Children
	childrenByParent := make(map[string][]*manager.SessionizeProject)
	var roots []*manager.SessionizeProject
	m.hasChildren = make(map[string]bool)

	for path := range finalIncludedPaths {
		p := projectByPath[path]
		if p == nil {
			continue
		}

		parentPath := p.GetHierarchicalParent()
		if parentPath == "" || projectByPath[parentPath] == nil || !finalIncludedPaths[parentPath] {
			if p.ParentEcosystemPath != "" && projectByPath[p.ParentEcosystemPath] != nil && finalIncludedPaths[p.ParentEcosystemPath] {
				parentPath = p.ParentEcosystemPath
			} else {
				parentPath = ""
			}
		}

		if m.focusedProject != nil && p.Path == m.focusedProject.Path {
			parentPath = ""
		}

		if parentPath != "" {
			childrenByParent[parentPath] = append(childrenByParent[parentPath], p)
			m.hasChildren[parentPath] = true
		} else {
			roots = append(roots, p)
		}
	}

	// 6. Sort and Flatten
	sort.Slice(roots, func(i, j int) bool {
		var hasActive func(path string) bool
		hasActive = func(path string) bool {
			p := projectByPath[path]
			if p != nil && m.runningSessions[p.Identifier()] {
				return true
			}
			for _, child := range childrenByParent[path] {
				if hasActive(child.Path) {
					return true
				}
			}
			return false
		}

		activeI := hasActive(roots[i].Path)
		activeJ := hasActive(roots[j].Path)

		if activeI && !activeJ {
			return true
		}
		if !activeI && activeJ {
			return false
		}

		if roots[i].Name == "cx-repos" {
			return true
		}
		if roots[j].Name == "cx-repos" {
			return false
		}

		return strings.ToLower(roots[i].Name) < strings.ToLower(roots[j].Name)
	})

	m.filtered = []*manager.SessionizeProject{}
	var flatten func(p *manager.SessionizeProject)
	flatten = func(p *manager.SessionizeProject) {
		m.filtered = append(m.filtered, p)

		// Check for active folding (ignored if actively text searching)
		if filter == "" && m.foldedPaths[p.Path] && m.hasChildren[p.Path] {
			return
		}

		children := childrenByParent[p.Path]

		sort.Slice(children, func(i, j int) bool {
			iIsEcoWT := children[i].Kind == workspace.KindEcosystemWorktree
			jIsEcoWT := children[j].Kind == workspace.KindEcosystemWorktree
			if iIsEcoWT != jIsEcoWT {
				return iIsEcoWT
			}
			return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
		})

		for _, child := range children {
			flatten(child)
		}
	}

	for _, root := range roots {
		flatten(root)
	}
}
func (m sessionizeModel) View() string {
	// If help is visible, show it and return
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder

	// Render group tabs (always show so users can switch groups and set keys)
	groups := m.manager.GetGroups()
	if len(groups) > 0 {
		labelStyle := lipgloss.NewStyle().Faint(true).Italic(true)
		b.WriteString("  " + labelStyle.Render("Key group: "))

		activeGroup := m.manager.GetActiveGroup()
		var tabs []string
		for _, g := range groups {
			iconStr := ""
			if g == "default" {
				if defIcon := m.manager.GetDefaultIcon(); defIcon != "" {
					iconStr = resolveIcon(defIcon) + " "
				} else {
					iconStr = core_theme.IconHome + " "
				}
			} else {
				if cfg, ok := m.manager.GetGroupConfig(g); ok && cfg.Icon != "" {
					iconStr = resolveIcon(cfg.Icon) + " "
				} else {
					iconStr = core_theme.IconFolderStar + " "
				}
			}

			tabText := iconStr + g

			if g == activeGroup {
				arrow := core_theme.DefaultTheme.Highlight.Render(core_theme.IconArrowRightBold)
				tabs = append(tabs, arrow+" "+core_theme.DefaultTheme.Highlight.Render(tabText))
			} else {
				tabs = append(tabs, "  "+core_theme.DefaultTheme.Muted.Render(tabText))
			}
		}
		b.WriteString(strings.Join(tabs, core_theme.DefaultTheme.Muted.Render(" │ ")))
		b.WriteString("\n")
	}

	// Render projects using table view
	b.WriteString(m.renderTable())

	b.WriteString("\n")

	// Footer with status indicators and help
	helpStyle := core_theme.DefaultTheme.Muted
	violetStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultTheme.Colors.Violet)

	// Mode and status indicators
	if m.newGroupMode {
		b.WriteString("  " + core_theme.DefaultTheme.Info.Render(core_theme.IconFolderStar+" New Group Mode") + "\n")
		if m.newGroupStep == 0 {
			b.WriteString("  " + core_theme.DefaultTheme.Header.Render("Name: "+m.newGroupName+"█") + "\n")
		} else {
			b.WriteString("  " + core_theme.DefaultTheme.Muted.Render("Name: "+m.newGroupName) + "\n")
			b.WriteString("  " + core_theme.DefaultTheme.Header.Render("Prefix: "+m.newGroupPrefix+"█") + "\n")
		}
		b.WriteString("  " + helpStyle.Render("Enter to confirm • Esc to cancel") + "\n")
	} else if m.mapToGroupMode {
		b.WriteString("  " + core_theme.DefaultTheme.Info.Render(core_theme.IconFolderStar+" Map to Group") + "\n")
		for i, g := range m.mapToGroupOptions {
			prefix := "  "
			if i == m.mapToGroupCursor {
				prefix = core_theme.DefaultTheme.Highlight.Render("> ")
			}
			b.WriteString("  " + prefix + g + "\n")
		}
		b.WriteString("  " + helpStyle.Render("j/k to select • Enter to confirm • Esc to cancel") + "\n")
	} else if m.ecosystemPickerMode {
		b.WriteString("  " + core_theme.DefaultTheme.Info.Render(core_theme.IconEcosystem+" Select ecosystem to focus") + "\n")
	} else if m.focusedProject != nil {
		focusIndicator := core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s [%s]", core_theme.IconEcosystem, m.focusedProject.Name))
		b.WriteString("  " + focusIndicator + "\n")
	}

	// Status message
	if m.statusMessage != "" && time.Now().Before(m.statusTimeout) && !m.newGroupMode && !m.mapToGroupMode {
		b.WriteString("  " + core_theme.DefaultTheme.Success.Render(m.statusMessage) + "\n")
	}

	// Dirty filter indicator
	if m.filterDirty {
		b.WriteString("  " + core_theme.DefaultTheme.Warning.Render("[DIRTY]") + "\n")
	}

	// Build status indicators line
	var indicators []string
	if m.filterGroup {
		indicators = append(indicators, violetStyle.Render(core_theme.IconFilter+" Group Filter"))
	}
	// Show worktrees indicator only when not using group filter
	// (group filter always shows worktrees for context)
	if !m.worktreesFolded && !m.filterGroup {
		indicators = append(indicators, violetStyle.Render(core_theme.IconWorktree+" Show Worktrees"))
	}
	// Show folded indicator when any paths are folded
	if len(m.foldedPaths) > 0 {
		indicators = append(indicators, violetStyle.Render("⋯ Folded"))
	}

	if len(indicators) > 0 {
		b.WriteString("  " + strings.Join(indicators, helpStyle.Render("  •  ")) + "\n")
	}

	// Help line
	if m.ecosystemPickerMode {
		b.WriteString("  " + helpStyle.Render("Enter to select • Esc to cancel"))
	} else if m.focusedProject != nil {
		b.WriteString("  " + helpStyle.Render("? • help • 0 • clear focus • q • quit"))
	} else {
		b.WriteString("  " + helpStyle.Render("? • help • q • quit"))
	}

	return pageStyle.Render(b.String())
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

// hasVisibleContextData checks if any filtered projects have a context rule applied or cx stats.
func (m sessionizeModel) hasVisibleContextData() bool {
	for _, project := range m.filtered {
		if status, ok := m.rulesState[project.Path]; ok {
			if status == grovecontext.RuleHot || status == grovecontext.RuleCold || status == grovecontext.RuleExcluded {
				return true
			}
		}
		if project.CxStats != nil && project.CxStats.Tokens > 0 {
			return true
		}
	}
	return false
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

// getVisiblePaths returns the paths of currently filtered projects.
// This is used to tell the daemon which workspaces to prioritize for scanning.
// When focused, returns all filtered paths (the focused ecosystem's children).
// Otherwise returns just the visible range.
func (m *sessionizeModel) getVisiblePaths() []string {
	// If focused or height not yet set, use all filtered paths
	if m.focusedProject != nil || m.height == 0 {
		paths := make([]string, 0, len(m.filtered))
		for _, p := range m.filtered {
			paths = append(paths, p.Path)
		}
		return paths
	}

	// Otherwise use visible range
	start, end := m.getVisibleRange()
	paths := make([]string, 0, end-start)
	for i := start; i < end && i < len(m.filtered); i++ {
		paths = append(paths, m.filtered[i].Path)
	}
	return paths
}

// jumpToPath sets up the UI to focus on the ecosystem containing the target path
func (m *sessionizeModel) jumpToPath(targetPath string, applyGroupFilter bool) {
	m.filterInput.SetValue("")
	m.filterDirty = false
	m.ecosystemPickerMode = false

	targetNormalized, _ := pathutil.NormalizeForLookup(targetPath)

	var targetProj *manager.SessionizeProject
	for _, p := range m.projects {
		norm, _ := pathutil.NormalizeForLookup(p.Path)
		if norm == targetNormalized {
			targetProj = p
			break
		}
	}

	// If target is a worktree, ensure worktrees are shown
	if targetProj != nil && targetProj.IsWorktree() {
		m.worktreesFolded = false
	}

	if applyGroupFilter {
		// Apply group filter instead of ecosystem focus
		m.filterGroup = true
		m.focusedProject = nil
	} else if targetProj != nil {
		// Apply ecosystem focus for this project
		var targetEcosystem *manager.SessionizeProject
		var targetEcosystemIsWorktree bool

		for _, p := range m.projects {
			if !p.IsEcosystem() {
				continue
			}
			pNorm, _ := pathutil.NormalizeForLookup(p.Path)
			targetProjNorm, _ := pathutil.NormalizeForLookup(targetProj.Path)
			// Check if targetProj is inside this ecosystem
			if strings.HasPrefix(targetProjNorm, pNorm+string(filepath.Separator)) || targetProjNorm == pNorm {
				if p.IsWorktree() {
					targetEcosystem = p
					targetEcosystemIsWorktree = true
					break
				} else if targetEcosystem == nil || !targetEcosystemIsWorktree {
					targetEcosystem = p
					targetEcosystemIsWorktree = false
				}
			}
		}

		if targetEcosystem != nil {
			m.focusedProject = targetEcosystem
			m.filterGroup = false
		} else {
			m.focusedProject = nil
		}
	}

	// Clear all folds to ensure target is visible
	m.foldedPaths = make(map[string]bool)

	m.updateFiltered()

	// Find in filtered and set cursor
	for i, p := range m.filtered {
		pNorm, _ := pathutil.NormalizeForLookup(p.Path)
		if pNorm == targetNormalized {
			m.cursor = i
			break
		}
	}
}

// focusEcosystemForPath applies focus to the ecosystem containing the given path.
// If targetPath is empty, uses the current working directory.
func (m *sessionizeModel) focusEcosystemForPath(targetPath string) tea.Cmd {
	if targetPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil
		}
		targetPath = cwd
	}

	targetNormalized, err := pathutil.NormalizeForLookup(targetPath)
	if err != nil {
		targetNormalized = filepath.Clean(targetPath)
	}

	var targetEcosystem *manager.SessionizeProject
	var targetEcosystemIsWorktree bool

	for _, p := range m.projects {
		if !p.IsEcosystem() {
			continue
		}
		pNormalized, err := pathutil.NormalizeForLookup(p.Path)
		if err != nil {
			pNormalized = filepath.Clean(p.Path)
		}
		// Check if target path is inside this ecosystem
		if strings.HasPrefix(targetNormalized, pNormalized+string(filepath.Separator)) || targetNormalized == pNormalized {
			if p.IsWorktree() {
				targetEcosystem = p
				targetEcosystemIsWorktree = true
				break
			} else if targetEcosystem == nil || !targetEcosystemIsWorktree {
				targetEcosystem = p
				targetEcosystemIsWorktree = false
			}
		}
	}

	if targetEcosystem == nil {
		m.statusMessage = "Path not inside a known ecosystem"
		m.statusTimeout = time.Now().Add(2 * time.Second)
		return clearStatusCmd(2 * time.Second)
	}

	m.filterGroup = false
	m.filterDirty = false
	m.filterInput.SetValue("")
	m.ecosystemPickerMode = false
	m.focusedProject = targetEcosystem
	m.updateFiltered()

	// Find the exact workspace in the ecosystem and move cursor to it
	m.cursor = 0
	for i, p := range m.filtered {
		pNormalized, err := pathutil.NormalizeForLookup(p.Path)
		if err != nil {
			pNormalized = filepath.Clean(p.Path)
		}
		if pNormalized == targetNormalized {
			m.cursor = i
			break
		}
	}
	if m.cursor == 0 {
		m.moveCursorToFirstSelectable()
	}

	_ = m.buildState().Save(m.configDir)
	return updateDaemonFocusCmd(m.getVisiblePaths())
}

// goToMappingForPath switches to the group containing the path's mapping and applies group filter.
// Caller should verify the path is mapped before calling this function.
func (m *sessionizeModel) goToMappingForPath(targetPath string) tea.Cmd {
	targetGroup := m.manager.FindGroupForPath(targetPath)
	if targetGroup == "" {
		// Should not happen if caller verified, but handle gracefully
		return nil
	}

	m.manager.SetActiveGroup(targetGroup)
	_ = m.manager.SetLastAccessedGroup(targetGroup)

	// Reload sessions for the new group
	m.sessions, _ = m.manager.GetSessions()
	m.keyMap = make(map[string]string)
	for _, s := range m.sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				m.keyMap[filepath.Clean(absPath)] = s.Key
			}
		}
	}

	// Apply group filter to show only mapped projects
	m.filterGroup = true
	m.filterInput.SetValue("")
	m.filterDirty = false
	m.focusedProject = nil
	m.ecosystemPickerMode = false

	m.updateFiltered()

	// Position cursor on the project (or a parent if the path is deep inside)
	m.cursor = 0
	cleanTargetPath := filepath.Clean(targetPath)
	targetNormalized, _ := pathutil.NormalizeForLookup(cleanTargetPath)
	for i, p := range m.filtered {
		pNormalized, _ := pathutil.NormalizeForLookup(filepath.Clean(p.Path))
		// Exact match or target is inside this project
		if pNormalized == targetNormalized || strings.HasPrefix(targetNormalized, pNormalized+string(filepath.Separator)) {
			m.cursor = i
			break
		}
	}

	return nil
}
