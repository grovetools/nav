package sessionizer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/keymap"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/util/pathutil"
	grovecontext "github.com/grovetools/cx/pkg/context"
	"github.com/grovetools/nav/pkg/api"
)

// Features is re-exported for ergonomics — callers building a Config can
// reference sessionizer.Features rather than importing nav/pkg/api directly.
type Features = api.Features

// ProjectLoader fetches the full project list. The standalone nav binary
// supplies a loader that talks to *tmux.Manager and applies the cloned-repo
// virtual ecosystem grouping; terminal can supply its own implementation.
type ProjectLoader func() ([]*api.Project, error)

// Config collects every dependency the sessionizer TUI needs from its host.
// Store / SessionDriver / SessionStateProvider are the required interfaces.
// Everything else is plain data or optional callbacks.
type Config struct {
	Store                Store
	SessionDriver        SessionDriver
	SessionStateProvider SessionStateProvider

	ConfigDir     string
	SearchPaths   []string
	Features      Features
	CwdFocusPath  string
	UsedCache     bool
	CurrentSession string // name of the current session (empty if not inside one)

	// LoadProjects is called on manual refresh to reload the project list.
	// May be nil — manual refresh becomes a no-op in that case.
	LoadProjects ProjectLoader

	// ReloadConfig is an optional callback invoked after session mutations
	// (e.g. after updating keys) so the host can reload its multiplexer's
	// configuration. Nil means "no reload".
	ReloadConfig func() error

	// KeyMap lets the host override the default sessionizer keymap. Zero
	// value uses DefaultKeyMap().
	KeyMap KeyMap

	// DisableCx forces the CX column off regardless of persisted user
	// state. Hosts that embed the sessionizer in long-lived processes
	// (e.g. the terminal) use this to opt out of the cx.Manager rules
	// pipeline entirely — it recompiles the grove.toml JSONSchema
	// validator per project per tick and pegs CPU at 400%+.
	DisableCx bool
}

// jumpState captures the view state for the jump list (C-o/C-i navigation).
type jumpState struct {
	focusedPath     string
	cursor          int
	filterText      string
	filterGroup     bool
	filterDirty     bool
	worktreesFolded bool
	activeGroup     string
}

// Model is the interactive project picker. It implements tea.Model and is
// the exported replacement for the previous sessionizeModel defined inside
// package main.
type Model struct {
	cfg Config

	projects   []*api.Project
	filtered   []*api.Project
	selected   *api.Project
	projectMap map[string]*api.Project

	cursor       int
	filterInput  textinput.Model
	store        Store
	features     Features
	configDir    string

	keyMap          map[string]string
	runningSessions map[string]bool
	currentSession  string
	width           int
	height          int
	keys            KeyMap
	availableKeys   []string
	sessions        []models.TmuxSession
	help            help.Model

	ecosystemPickerMode bool
	focusedProject      *api.Project
	worktreesFolded     bool
	foldedPaths         map[string]bool
	hasChildren         map[string]bool
	sequence            *keymap.SequenceState

	showGitStatus   bool
	showBranch      bool
	showNoteCounts  bool
	showPlanStats   bool
	showOnHold      bool
	pathDisplayMode int
	showRelease     bool
	showBinary      bool
	showLink        bool
	showCx          bool

	filterDirty bool
	filterGroup bool

	contextOnlyPaths map[string]bool

	statusMessage string
	statusTimeout time.Time

	isLoading    bool
	usedCache    bool
	spinnerFrame int
	lastSpinTime time.Time

	enrichmentLoading map[string]bool

	rulesState map[string]grovecontext.RuleStatus

	newGroupMode   bool
	newGroupStep   int
	newGroupName   string
	newGroupPrefix string

	mapToGroupMode    bool
	mapToGroupOptions []string
	mapToGroupCursor  int
	mapToGroupPaths   []string

	selectedPaths map[string]bool

	jumpList []jumpState
	jumpIdx  int

	// panelFocused tracks whether this panel currently has embed focus.
	// When false, the periodic tick skips enrichment and daemon focus
	// updates to avoid unnecessary work while the panel is hidden.
	panelFocused bool

	// Daemon SSE stream — owned per-Model so multiple sessionizer instances
	// can be embedded without sharing state. Set when daemonStreamConnectedMsg
	// is delivered; cleared by Close().
	streamCh     <-chan daemon.StateUpdate
	streamCancel context.CancelFunc
	// streamWg tracks in-flight listenToDaemonCmd goroutines so Close() can
	// wait for them to exit before returning. Hosts that rapidly create and
	// destroy sessionizer instances must not leak SSE listener goroutines.
	streamWg sync.WaitGroup
}

// Close releases resources owned by the Model. Today this only tears down
// the daemon SSE stream subscription, but hosts that embed the sessionizer
// (e.g. grove terminal) should call it on shutdown so background goroutines
// don't leak between Model lifetimes.
//
// Close cancels the SSE stream context (which makes the daemon close the
// channel) and then waits for any in-flight listener goroutine to actually
// exit before returning. Without the wait, an embedded host that rapidly
// creates and destroys sessionizer instances could briefly leak a listener
// goroutine per instance.
func (m *Model) Close() error {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
		m.streamCh = nil
	}
	m.streamWg.Wait()
	return nil
}

// listenToDaemon wraps listenToDaemonCmd so the Model's WaitGroup tracks
// the in-flight listener goroutine. Add() runs synchronously when the cmd
// is constructed (in Update); Done() runs when bubbletea executes the cmd
// and the goroutine exits. This guarantees Close() can wait for the
// listener to actually return.
func (m *Model) listenToDaemon() tea.Cmd {
	if m.streamCh == nil {
		return nil
	}
	m.streamWg.Add(1)
	inner := listenToDaemonCmd(m.streamCh)
	return func() tea.Msg {
		defer m.streamWg.Done()
		return inner()
	}
}

// Selected returns the project the user chose with the Confirm key, or nil
// if the user quit without selecting anything.
func (m *Model) Selected() *api.Project {
	return m.selected
}

// IsTextInputFocused reports whether the sessionizer's filter text input is
// currently focused. Hosts use this to decide whether a global key stroke
// should trigger a view switch or be forwarded as text.
func (m *Model) IsTextInputFocused() bool {
	return m.filterInput.Focused()
}

// JumpToPath is the public wrapper around the internal jumpToPath helper.
// It positions the cursor on the project with the given path and optionally
// applies the group filter.
func (m *Model) JumpToPath(targetPath string, applyGroupFilter bool) {
	m.jumpToPath(targetPath, applyGroupFilter)
}

// FocusEcosystemForPath is the public wrapper around focusEcosystemForPath.
func (m *Model) FocusEcosystemForPath(targetPath string) tea.Cmd {
	return m.focusEcosystemForPath(targetPath)
}

// New builds a sessionizer Model for the given project list and config.
func New(cfg Config, projects []*api.Project) *Model {
	if cfg.KeyMap.Quit.Keys() == nil {
		cfg.KeyMap = DefaultKeyMap()
	}

	ti := textinput.New()
	ti.Placeholder = ""
	ti.Prompt = core_theme.DefaultTheme.Muted.Render("󰍉 ")
	ti.CharLimit = 256
	ti.Width = 50

	store := cfg.Store

	initialGroup := store.GetActiveGroup()
	autoEnableGroupFilter := false
	clearFocus := false

	cwd, err := os.Getwd()
	if err == nil && cwd != "" {
		cwdGroup := store.FindGroupForPath(cwd)
		if cwdGroup != "" && cwdGroup != "default" {
			autoEnableGroupFilter = true
		} else if _, err := workspace.GetProjectByPath(cwd); err == nil {
			clearFocus = true
		}
	}

	store.SetActiveGroup(initialGroup)
	_ = store.SetLastAccessedGroup(initialGroup)

	keyMap := make(map[string]string)
	sessions, err := store.GetSessions()
	if err != nil {
		sessions = []models.TmuxSession{}
	}
	for _, s := range sessions {
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err == nil {
				keyMap[filepath.Clean(absPath)] = s.Key
			}
		}
	}

	// Available keys aren't on the Store interface — the sessionizer uses
	// them purely for auto-assignment when mapping projects. We derive them
	// from sessions that have an empty Path (i.e. free slots).
	availableKeys := []string{}
	for _, s := range sessions {
		if s.Path == "" {
			availableKeys = append(availableKeys, s.Key)
		}
	}

	runningSessions := make(map[string]bool)
	currentSession := cfg.CurrentSession
	if cfg.SessionStateProvider != nil {
		ctx := context.Background()
		if names, err := cfg.SessionStateProvider.ListActive(ctx); err == nil {
			for _, name := range names {
				runningSessions[name] = true
			}
		}
	}

	helpModel := help.NewBuilder().
		WithKeys(cfg.KeyMap).
		WithTitle("Project Sessionizer - Help").
		WithLegend("Icons: " + core_theme.IconBullet + " current • " + core_theme.IconBullet + " active • " + core_theme.IconEcosystem + " ecosystem • " + core_theme.IconRepo + " repo • " + core_theme.IconWorktree + " worktree • " + core_theme.IconGitBranch + " branch").
		Build()

	projectMap := make(map[string]*api.Project, len(projects))
	for _, p := range projects {
		p.EnrichmentStatus = make(map[string]string)
		if p.GitStatus != nil {
			p.EnrichmentStatus["git"] = "done"
		}
		projectMap[p.Path] = p
	}

	features := cfg.Features

	var focusedProject *api.Project
	var worktreesFolded bool
	foldedPaths := make(map[string]bool)
	showGitStatus := true
	showBranch := true
	showNoteCounts := true
	showPlanStats := true
	pathDisplayMode := 1
	showRelease := false
	showBinary := false
	showLink := false
	showCx := true

	if !features.Integrations {
		showGitStatus = false
		showBranch = false
		showNoteCounts = false
		showPlanStats = false
		showRelease = false
		showBinary = false
		showLink = false
		showCx = false
	}
	if !features.Worktrees {
		worktreesFolded = false
	}

	if state, err := api.LoadState(cfg.ConfigDir); err == nil {
		if clearFocus {
			state.FocusedEcosystemPath = ""
		} else if cfg.CwdFocusPath != "" {
			state.FocusedEcosystemPath = cfg.CwdFocusPath
		}
		if state.FocusedEcosystemPath != "" {
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

	// Host opt-out wins over persisted state: embedders that set
	// DisableCx in Config never get the cx rules pipeline, no matter
	// what the user's nav state file says.
	if cfg.DisableCx {
		showCx = false
	}

	m := &Model{
		cfg:              cfg,
		rulesState:       make(map[string]grovecontext.RuleStatus),
		projects:         projects,
		filtered:         projects,
		projectMap:       projectMap,
		filterInput:      ti,
		store:            store,
		features:         features,
		configDir:        cfg.ConfigDir,
		keyMap:           keyMap,
		runningSessions:  runningSessions,
		currentSession:   currentSession,
		keys:             cfg.KeyMap,
		availableKeys:    availableKeys,
		sessions:         sessions,
		help:             helpModel,
		worktreesFolded:  worktreesFolded,
		showGitStatus:    showGitStatus,
		showBranch:       showBranch,
		showNoteCounts:   showNoteCounts,
		showPlanStats:    showPlanStats,
		showOnHold:       false,
		pathDisplayMode:  pathDisplayMode,
		showRelease:      showRelease,
		showBinary:       showBinary,
		showLink:         showLink,
		showCx:           showCx,
		filterGroup:      autoEnableGroupFilter,
		focusedProject: func() *api.Project {
			if autoEnableGroupFilter {
				return nil
			}
			return focusedProject
		}(),
		contextOnlyPaths:  make(map[string]bool),
		usedCache:         cfg.UsedCache,
		isLoading:         cfg.UsedCache,
		enrichmentLoading: make(map[string]bool),
		foldedPaths:       foldedPaths,
		hasChildren:       make(map[string]bool),
		sequence:          keymap.NewSequenceState(),
		selectedPaths:     make(map[string]bool),
		jumpList:          make([]jumpState, 0),
		jumpIdx:           0,
		panelFocused:      true, // assume focused until BlurMsg says otherwise
	}

	if !features.Groups {
		m.keys.NextGroup.SetEnabled(false)
		m.keys.PrevGroup.SetEnabled(false)
		m.keys.FilterGroup.SetEnabled(false)
		m.keys.ManageGroups.SetEnabled(false)
		m.keys.NewGroup.SetEnabled(false)
		m.keys.MapToGroup.SetEnabled(false)
		m.keys.GoToMappingCursor.SetEnabled(false)
		m.keys.GoToMappingCwd.SetEnabled(false)
	}
	if !features.Ecosystems {
		m.keys.FocusEcosystem.SetEnabled(false)
		m.keys.OpenEcosystem.SetEnabled(false)
		m.keys.FocusEcosystemCursor.SetEnabled(false)
		m.keys.FocusEcosystemCwd.SetEnabled(false)
		m.keys.ClearFocus.SetEnabled(false)
	}
	if !features.Integrations {
		m.keys.ToggleCx.SetEnabled(false)
		m.keys.ToggleNoteCounts.SetEnabled(false)
		m.keys.TogglePlanStats.SetEnabled(false)
		m.keys.ToggleHotContext.SetEnabled(false)
		m.keys.ToggleHold.SetEnabled(false)
		m.keys.ToggleRelease.SetEnabled(false)
		m.keys.ToggleBinary.SetEnabled(false)
		m.keys.ToggleLink.SetEnabled(false)
	}
	if !features.Worktrees {
		m.keys.ToggleWorktrees.SetEnabled(false)
	}

	m.updateFiltered()
	m.moveCursorToFirstSelectable()

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

// buildState captures the current UI state for persistence.
func (m *Model) buildState() *api.SessionizerState {
	state := &api.SessionizerState{
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

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		fetchRunningSessionsCmd(m.cfg.SessionStateProvider),
		fetchKeyMapCmd(m.store),
		tickCmd(),
		subscribeToDaemonCmd(),
		updateDaemonFocusCmd(m.getVisiblePaths()),
	}

	if !m.usedCache {
		cmds = append(cmds, fetchProjectsCmd(m.cfg.LoadProjects))
	} else if m.showCx {
		cmds = append(cmds, fetchRulesStateCmd(m.projects))
	}

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
	if m.showCx {
		m.enrichmentLoading["cxstats"] = true
		cmds = append(cmds, fetchCxPerLineStatsCmd(m.projects))
	}

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
