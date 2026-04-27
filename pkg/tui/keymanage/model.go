package keymanage

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/nav/pkg/api"
)

// pageStyle is the default lipgloss style for the keymanage view.
var (
	pageStyle = lipgloss.NewStyle()
	dimStyle  = core_theme.DefaultTheme.Muted
)

// Features re-exports the nav feature-flag struct for ergonomics.
type Features = api.Features

// Config collects every dependency the keymanage TUI needs from its host.
// Store and SessionDriver are the required interfaces; everything else
// is plain data or optional callbacks.
type Config struct {
	// Store is the mutation surface the TUI drives (read sessions,
	// update/lock keys, groups, undo/redo, etc.).
	Store Store

	// SessionDriver handles the tmux launch/switch surface. May be nil
	// if the host is not running inside tmux (the TUI shows "Not in a
	// tmux session" messages when it needs to jump to a session).
	SessionDriver SessionDriver

	// ConfigDir is the nav config directory (used for cache save/load).
	ConfigDir string

	// CwdPath is the current working directory — used to enrich the
	// CWD project for the "map CWD to key" flow.
	CwdPath string

	// Features is the resolved nav feature-flag set. Controls which
	// group-related key bindings are enabled.
	Features Features

	// EnrichedProjects is an optional seed map of cached enriched
	// projects from a previous session. May be nil.
	EnrichedProjects map[string]*api.Project

	// UsedCache indicates that EnrichedProjects came from a stale cache.
	// When true, the TUI begins in a loading state and refreshes on Init.
	UsedCache bool

	// ReloadConfig is an optional callback invoked after session-binding
	// mutations so the host can reload its multiplexer config (e.g. tmux
	// source-file). Nil means "no reload".
	ReloadConfig func() error

	// KeyMap lets the host override the default manage keymap. Zero
	// value uses DefaultKeyMap().
	KeyMap KeyMap
}

// Model is the interactive session key manager. It implements tea.Model.
type Model struct {
	cfg Config
	// EmbedMode suppresses the header in View() when the model is
	// hosted inside a pager that renders its own title row.
	EmbedMode bool
	store     Store
	driver    SessionDriver
	keys      KeyMap
	help      help.Model
	width     int
	height    int
	cwdPath   string

	cursor   int
	sessions []models.TmuxSession

	features Features

	quitting     bool
	message      string
	selectedPath string

	// CWD state
	cwdProject *api.Project

	// Enriched data
	enrichedProjects  map[string]*api.Project
	enrichmentLoading map[string]bool

	// Navigation modes
	jumpMode   bool
	setKeyMode bool

	// Move mode state
	moveMode   bool
	lockedKeys map[string]bool

	// Loading state
	isLoading    bool
	usedCache    bool
	spinnerFrame int

	// View toggles
	pathDisplayMode int
	commandOnExit   *exec.Cmd

	// Change tracking
	changesMade bool

	// Save to group state
	saveToGroupMode    bool
	saveToGroupOptions []string
	saveToGroupCursor  int
	saveToGroupNewMode bool
	saveToGroupInput   string

	// Move to group state
	moveToGroupMode    bool
	moveToGroupOptions []string
	moveToGroupCursor  int
	moveToGroupKeys    []string
	selectedKeys       map[string]bool

	// Confirmation state
	confirmMode   string
	confirmSource string

	// Load from group state
	loadFromGroupMode    bool
	loadFromGroupOptions []string
	loadFromGroupCursor  int

	// Group creation state
	newGroupMode   bool
	newGroupStep   int
	newGroupName   string
	newGroupPrefix string

	// Default locked sessions (shared across all groups)
	defaultLockedSessions map[string]models.TmuxSession

	// Handoff hint for hosts that poll NextCommand().
	nextCommand string

	// Pending mapping state (from sessionize view)
	pendingMapProject *api.Project

	// Just-mapped highlighting state
	justMappedKeys map[string]bool
}

// New constructs a Model from the given Config. The caller owns the
// Store and SessionDriver lifetimes.
func New(cfg Config) *Model {
	if cfg.KeyMap.Quit.Keys() == nil {
		cfg.KeyMap = DefaultKeyMap()
	}

	helpModel := help.NewBuilder().
		WithKeys(cfg.KeyMap).
		WithTitle("Session Key Manager - Help").
		Build()

	enriched := cfg.EnrichedProjects
	if enriched == nil {
		enriched = make(map[string]*api.Project)
	}

	// Load locked keys from the store.
	lockedKeysSlice := cfg.Store.GetLockedKeys()
	lockedKeysMap := make(map[string]bool)
	for _, k := range lockedKeysSlice {
		lockedKeysMap[k] = true
	}

	// Load default's sessions for locked keys (shared across all groups).
	// Temporarily flip the active group to "default" to read its bindings.
	currentGroup := cfg.Store.GetActiveGroup()
	cfg.Store.SetActiveGroup("default")
	defaultSessions, _ := cfg.Store.GetSessions()
	defaultLockedSessions := make(map[string]models.TmuxSession)
	for _, s := range defaultSessions {
		if lockedKeysMap[s.Key] {
			defaultLockedSessions[s.Key] = s
		}
	}
	cfg.Store.SetActiveGroup(currentGroup)

	sessions, _ := cfg.Store.GetSessions()

	m := &Model{
		cfg:                   cfg,
		store:                 cfg.Store,
		driver:                cfg.SessionDriver,
		keys:                  cfg.KeyMap,
		help:                  helpModel,
		cwdPath:               cfg.CwdPath,
		sessions:              sessions,
		features:              cfg.Features,
		enrichedProjects:      enriched,
		lockedKeys:            lockedKeysMap,
		usedCache:             cfg.UsedCache,
		isLoading:             cfg.UsedCache,
		enrichmentLoading:     make(map[string]bool),
		pathDisplayMode:       0,
		defaultLockedSessions: defaultLockedSessions,
		selectedKeys:          make(map[string]bool),
		justMappedKeys:        make(map[string]bool),
	}

	// Prune key bindings based on feature flags (also removes them from
	// the help menu).
	if !m.features.Groups {
		m.keys.NextGroup.SetEnabled(false)
		m.keys.PrevGroup.SetEnabled(false)
		m.keys.LoadDefault.SetEnabled(false)
		m.keys.UnloadDefault.SetEnabled(false)
		m.keys.SaveToGroup.SetEnabled(false)
		m.keys.MoveToGroup.SetEnabled(false)
		m.keys.NewGroup.SetEnabled(false)
		m.keys.DeleteGroup.SetEnabled(false)
		m.keys.Groups.SetEnabled(false)
	}

	return m
}

// Close releases resources owned by the Model. Currently a no-op, kept
// for symmetry with the other extracted TUIs.
func (m *Model) Close() error { return nil }

// SelectedPath returns the path of the session the user jumped to via
// openSessionForPath, or "" if no jump occurred. Used by the terminal
// host's wrapInnerCmd to translate a keymanage tea.Quit into a
// SwitchWorkspaceMsg.
func (m *Model) SelectedPath() string { return m.selectedPath }

// ResetJumpState clears the quit/jump state left behind by
// openSessionForPath so the model renders the full session list
// again. Called on tab re-entry (OnReenterKeymanage) because the
// navapp keeps sub-models alive across blurs.
func (m *Model) ResetJumpState() {
	m.quitting = false
	m.selectedPath = ""
	m.message = ""
}

// Init implements tea.Model. Fires the initial enrichment commands.
func (m *Model) Init() tea.Cmd {
	m.ResetJumpState()
	m.rebuildSessionsOrder()

	cmds := []tea.Cmd{
		enrichInitialProjectsCmd(m.sessions, m.enrichedProjects),
		enrichCwdProjectCmd(m.cwdPath),
	}
	if m.isLoading {
		cmds = append(cmds, spinnerTickCmd())
	}
	return tea.Batch(cmds...)
}

// ----- Public accessors for host routers -----------------------------------

// NextCommand returns the host-handoff hint (currently only "groups"
// when the user presses the groups key). Hosts should poll this after
// routing key messages and switch views accordingly.
func (m *Model) NextCommand() string { return m.nextCommand }

// ClearNextCommand resets the handoff hint after the host has acted.
func (m *Model) ClearNextCommand() { m.nextCommand = "" }

// PendingMapProject reports whether the model is currently waiting for
// the user to pick a slot for a pending map operation. Hosts read this
// to decide whether to clear pending state on view switches.
func (m *Model) PendingMapProject() *api.Project { return m.pendingMapProject }

// ClearPendingMapProject clears the pending-map state. Hosts call this
// when switching away from the manage view without completing the map.
func (m *Model) ClearPendingMapProject() { m.pendingMapProject = nil }

// IsTextInputFocused reports whether the model is in a sub-mode that
// captures text input (so global key bindings should be suppressed).
func (m *Model) IsTextInputFocused() bool {
	return m.setKeyMode || m.saveToGroupNewMode || m.newGroupMode
}

// Sessions returns the currently-displayed sessions slice. Hosts use
// this to forward state to the store on exit (e.g. to persist unsaved
// changes).
func (m *Model) Sessions() []models.TmuxSession { return m.sessions }

// LockedKeysSlice returns the locked-keys state as a slice. Hosts use
// this together with Sessions() to persist state on exit.
func (m *Model) LockedKeysSlice() []string { return m.getLockedKeysSlice() }

// ChangesMade reports whether the model has unsaved changes. Hosts use
// this on exit to decide whether to call UpdateSessionsAndLocks.
func (m *Model) ChangesMade() bool { return m.changesMade }

// CommandOnExit returns an exec.Cmd the host should run after the TUI
// exits (e.g. a popup-close command). Nil if nothing to run.
func (m *Model) CommandOnExit() *exec.Cmd { return m.commandOnExit }

// RefreshAfterGroupSwitch is called by the host after a cross-TUI group
// switch (e.g. after returning from the groups view) to re-read sessions
// and reset the cursor.
func (m *Model) RefreshAfterGroupSwitch() {
	m.sessions, _ = m.store.GetSessions()
	m.rebuildSessionsOrder()
	m.message = fmt.Sprintf("Switched to group: %s", m.store.GetActiveGroup())
}

// ApplyBulkMapping applies a batch of newly-mapped keys (from the
// sessionizer's bulk-map flow) by refreshing sessions and highlighting.
func (m *Model) ApplyBulkMapping(mappedKeys []string) {
	m.sessions, _ = m.store.GetSessions()
	m.rebuildSessionsOrder()
	m.justMappedKeys = make(map[string]bool)
	for _, k := range mappedKeys {
		m.justMappedKeys[k] = true
	}
	m.message = fmt.Sprintf("Mapped %d projects to keys", len(mappedKeys))
}

// ----- Internal helpers (previously private methods on manageModel) -------

// rebuildSessionsOrder ensures locked keys are always at the bottom.
// Locked keys use default's mappings (shared across all groups).
func (m *Model) rebuildSessionsOrder() {
	var unlocked, locked []models.TmuxSession
	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
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

// cycleGroup switches to the next or previous workspace group.
func (m *Model) cycleGroup(dir int) {
	if m.changesMade {
		_ = m.store.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice())
	}

	groups := m.store.GetGroups()
	if len(groups) <= 1 {
		m.message = "No other groups configured"
		return
	}

	currentIdx := 0
	for i, g := range groups {
		if g == m.store.GetActiveGroup() {
			currentIdx = i
			break
		}
	}

	nextIdx := (currentIdx + dir) % len(groups)
	if nextIdx < 0 {
		nextIdx = len(groups) - 1
	}

	newGroup := groups[nextIdx]
	m.store.SetActiveGroup(newGroup)
	_ = m.store.SetLastAccessedGroup(newGroup)

	m.sessions, _ = m.store.GetSessions()
	lockedKeysSlice := m.store.GetLockedKeys()
	m.lockedKeys = make(map[string]bool)
	for _, k := range lockedKeysSlice {
		m.lockedKeys[k] = true
	}
	m.cursor = 0
	m.rebuildSessionsOrder()
	m.changesMade = false
	m.justMappedKeys = make(map[string]bool)
	m.message = fmt.Sprintf("Switched to group: %s", newGroup)
}

// getLockedKeysSlice converts the locked keys map to a slice.
func (m *Model) getLockedKeysSlice() []string {
	out := make([]string, 0, len(m.lockedKeys))
	for k := range m.lockedKeys {
		out = append(out, k)
	}
	return out
}

// refreshStateAfterUndoRedo re-reads the model state after undo/redo.
func (m *Model) refreshStateAfterUndoRedo() {
	m.sessions, _ = m.store.GetSessions()
	lockedKeysSlice := m.store.GetLockedKeys()
	m.lockedKeys = make(map[string]bool)
	for _, k := range lockedKeysSlice {
		m.lockedKeys[k] = true
	}
	m.rebuildSessionsOrder()
	if m.cursor >= len(m.sessions) {
		m.cursor = len(m.sessions) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	if m.cfg.ReloadConfig != nil {
		_ = m.cfg.ReloadConfig()
	}
}

// saveChanges persists the current session state immediately.
func (m *Model) saveChanges() {
	if err := m.store.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error saving: %v", err)
		return
	}
	_ = m.store.RegenerateBindings()
	if m.cfg.ReloadConfig != nil {
		_ = m.cfg.ReloadConfig()
	}
	m.changesMade = false
}

// mapKeyToCwd maps the CWD to the target key index.
func (m *Model) mapKeyToCwd(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(m.sessions) || m.cwdProject == nil {
		return
	}

	targetSession := &m.sessions[targetIndex]
	cwdNormalizedPath, err := pathutil.NormalizeForLookup(m.cwdProject.Path)
	if err != nil {
		m.message = "Failed to normalize CWD path"
		m.setKeyMode = false
		return
	}

	// Clear any pre-existing mapping for the CWD path.
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

	targetSession.Path = m.cwdProject.Path
	targetSession.Repository = ""
	targetSession.Description = ""

	m.enrichedProjects[filepath.Clean(m.cwdProject.Path)] = m.cwdProject

	m.setKeyMode = false
	m.message = fmt.Sprintf("Mapped key '%s' to '%s'", targetSession.Key, m.cwdProject.Name)
	m.saveChanges()
}

// mapSelectedSlot maps the pending project or CWD to the currently
// selected slot. Returns the model and a follow-up tea.Cmd.
func (m *Model) mapSelectedSlot() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.sessions) {
		return m, nil
	}

	session := &m.sessions[m.cursor]

	if session.Path != "" {
		m.message = fmt.Sprintf("Key '%s' is already mapped. Clear it first with 'd' or space.", session.Key)
		return m, nil
	}

	var projectToMap *api.Project
	if m.pendingMapProject != nil {
		projectToMap = m.pendingMapProject
	} else {
		projectToMap = m.cwdProject
	}
	if projectToMap == nil {
		m.message = "Current directory is not a valid workspace/project"
		return m, nil
	}

	cwdNormalizedPath, err := pathutil.NormalizeForLookup(projectToMap.Path)
	if err != nil {
		m.message = "Failed to normalize project path"
		return m, nil
	}
	for _, s := range m.sessions {
		if s.Path == "" {
			continue
		}
		sNormalizedPath, err := pathutil.NormalizeForLookup(s.Path)
		if err != nil {
			continue
		}
		if sNormalizedPath == cwdNormalizedPath {
			m.message = fmt.Sprintf("Project is already mapped to key '%s'", s.Key)
			return m, nil
		}
	}

	m.store.TakeSnapshot()
	session.Path = projectToMap.Path
	session.Repository = ""
	session.Description = ""

	m.enrichedProjects[filepath.Clean(projectToMap.Path)] = projectToMap

	m.message = fmt.Sprintf("Mapped key '%s' to '%s'", session.Key, projectToMap.Name)
	m.justMappedKeys = map[string]bool{session.Key: true}
	m.saveChanges()

	if m.pendingMapProject != nil {
		m.pendingMapProject = nil
		return m, clearHighlightCmd()
	}
	return m, nil
}

// executeLoadIntoDefault performs the actual load operation after confirmation.
func (m *Model) executeLoadIntoDefault(sourceGroup string) {
	m.store.SetActiveGroup(sourceGroup)
	sourceSessions, _ := m.store.GetSessions()

	m.store.SetActiveGroup("default")
	m.sessions, _ = m.store.GetSessions()

	lockedKeys := m.store.GetLockedKeys()
	m.lockedKeys = make(map[string]bool)
	for _, k := range lockedKeys {
		m.lockedKeys[k] = true
	}

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

// executeClearGroup performs the actual clear operation after confirmation.
func (m *Model) executeClearGroup() {
	count := 0
	for i := range m.sessions {
		if m.sessions[i].Path != "" && !m.lockedKeys[m.sessions[i].Key] {
			m.sessions[i].Path = ""
			m.sessions[i].Repository = ""
			m.sessions[i].Description = ""
			count++
		}
	}
	groupName := m.store.GetActiveGroup()
	m.message = fmt.Sprintf("Cleared %d mappings from '%s'", count, groupName)
	m.rebuildSessionsOrder()
	m.saveChanges()
}

// saveDefaultToGroup saves current mappings to a target group.
func (m *Model) saveDefaultToGroup(targetGroup string) {
	sourceSessions := make([]models.TmuxSession, len(m.sessions))
	copy(sourceSessions, m.sessions)
	sourceGroup := m.store.GetActiveGroup()

	existingGroups := m.store.GetAllGroups()
	groupExists := false
	for _, g := range existingGroups {
		if g == targetGroup {
			groupExists = true
			break
		}
	}
	if !groupExists {
		if err := m.store.CreateGroup(targetGroup, ""); err != nil {
			m.message = fmt.Sprintf("Error creating group: %v", err)
			return
		}
	}

	m.store.SetActiveGroup(targetGroup)
	targetSessions, _ := m.store.GetSessions()

	count := 0
	for i := range targetSessions {
		if m.lockedKeys[targetSessions[i].Key] {
			continue
		}
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

	if err := m.store.UpdateSessionsAndLocks(targetSessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error saving to group: %v", err)
		m.store.SetActiveGroup(sourceGroup)
		return
	}
	if err := m.store.RegenerateBindings(); err != nil {
		m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
	} else if m.cfg.ReloadConfig != nil {
		_ = m.cfg.ReloadConfig()
	}

	m.sessions = targetSessions
	m.cursor = 0
	m.rebuildSessionsOrder()
	m.message = fmt.Sprintf("Saved %d mappings to '%s'", count, targetGroup)
}

// executeMoveToGroup moves the selected session(s) to the target group.
func (m *Model) executeMoveToGroup(targetGroup string) {
	if len(m.moveToGroupKeys) == 0 {
		return
	}

	sourceGroup := m.store.GetActiveGroup()

	type keyPath struct {
		key  string
		path string
	}
	itemsToMove := make([]keyPath, 0, len(m.moveToGroupKeys))
	for _, k := range m.moveToGroupKeys {
		for _, s := range m.sessions {
			if s.Key == k && s.Path != "" {
				itemsToMove = append(itemsToMove, keyPath{key: k, path: s.Path})
				break
			}
		}
	}
	if len(itemsToMove) == 0 {
		m.message = "No paths to move"
		return
	}

	m.store.SetActiveGroup(targetGroup)
	targetSessions, _ := m.store.GetSessions()

	type assignment struct {
		sourceKey string
		targetKey string
		path      string
	}
	assignments := make([]assignment, 0, len(itemsToMove))
	usedTargetKeys := make(map[string]bool)

	// First pass: preserve same keys where possible.
	for _, item := range itemsToMove {
		for _, ts := range targetSessions {
			if ts.Key == item.key && ts.Path == "" && !m.lockedKeys[ts.Key] && !usedTargetKeys[ts.Key] {
				assignments = append(assignments, assignment{sourceKey: item.key, targetKey: ts.Key, path: item.path})
				usedTargetKeys[ts.Key] = true
				break
			}
		}
	}

	// Second pass: any available slot.
	for _, item := range itemsToMove {
		assigned := false
		for _, a := range assignments {
			if a.sourceKey == item.key {
				assigned = true
				break
			}
		}
		if assigned {
			continue
		}
		for _, ts := range targetSessions {
			if ts.Path == "" && !m.lockedKeys[ts.Key] && !usedTargetKeys[ts.Key] {
				assignments = append(assignments, assignment{sourceKey: item.key, targetKey: ts.Key, path: item.path})
				usedTargetKeys[ts.Key] = true
				break
			}
		}
	}

	if len(assignments) < len(itemsToMove) {
		m.message = fmt.Sprintf("Not enough empty slots in '%s' (need %d, have %d)", targetGroup, len(itemsToMove), len(assignments))
		m.store.SetActiveGroup(sourceGroup)
		return
	}

	for _, a := range assignments {
		for i := range targetSessions {
			if targetSessions[i].Key == a.targetKey {
				targetSessions[i].Path = a.path
				targetSessions[i].Repository = filepath.Base(a.path)
				break
			}
		}
	}

	if err := m.store.UpdateSessionsAndLocks(targetSessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error saving to target group: %v", err)
		m.store.SetActiveGroup(sourceGroup)
		return
	}

	m.store.SetActiveGroup(sourceGroup)
	for _, a := range assignments {
		for i := range m.sessions {
			if m.sessions[i].Key == a.sourceKey {
				m.sessions[i].Path = ""
				m.sessions[i].Repository = ""
				m.sessions[i].Description = ""
				break
			}
		}
	}

	if err := m.store.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice()); err != nil {
		m.message = fmt.Sprintf("Error clearing source: %v", err)
		return
	}
	if err := m.store.RegenerateBindings(); err != nil {
		m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
	} else if m.cfg.ReloadConfig != nil {
		_ = m.cfg.ReloadConfig()
	}

	m.selectedKeys = make(map[string]bool)
	m.changesMade = true

	m.store.SetActiveGroup(targetGroup)
	_ = m.store.SetLastAccessedGroup(targetGroup)
	m.sessions, _ = m.store.GetSessions()
	m.rebuildSessionsOrder()

	if len(assignments) > 0 {
		m.justMappedKeys = make(map[string]bool)
		for _, a := range assignments {
			m.justMappedKeys[a.targetKey] = true
		}
		for i, s := range m.sessions {
			if s.Key == assignments[0].targetKey {
				m.cursor = i
				break
			}
		}
	}

	m.message = fmt.Sprintf("Moved %d items to '%s'", len(assignments), targetGroup)
}

// JumpToPath finds the group and row containing the given path and jumps
// to it. Returns true if the path was found. Exported so hosts can
// trigger the jump after receiving a cross-TUI jump message.
func (m *Model) JumpToPath(targetPath string) bool {
	normalizedTarget, err := pathutil.NormalizeForLookup(filepath.Clean(targetPath))
	if err != nil {
		return false
	}

	groups := m.store.GetGroups()
	for _, g := range groups {
		m.store.SetActiveGroup(g)
		sessions, _ := m.store.GetSessions()

		for i, s := range sessions {
			if s.Path == "" {
				continue
			}
			normalizedSessionPath, err := pathutil.NormalizeForLookup(filepath.Clean(s.Path))
			if err != nil {
				continue
			}
			if normalizedSessionPath == normalizedTarget || strings.HasPrefix(normalizedTarget, normalizedSessionPath+string(filepath.Separator)) {
				_ = m.store.SetLastAccessedGroup(g)
				m.sessions = sessions
				lockedKeysSlice := m.store.GetLockedKeys()
				m.lockedKeys = make(map[string]bool)
				for _, k := range lockedKeysSlice {
					m.lockedKeys[k] = true
				}
				m.rebuildSessionsOrder()
				m.cursor = i
				m.changesMade = false
				m.message = fmt.Sprintf("Found in group '%s' (key %s)", g, s.Key)
				return true
			}
		}
	}
	return false
}

// openSessionForPath launches/switches to a tmux session for the given
// path. It consults the SessionDriver from Config; if no driver is set,
// the message is set to "Not in a tmux session" and the model stays put.
// Returns a tea.Cmd that may include tea.Quit if the driver succeeds.
func (m *Model) openSessionForPath(ctx context.Context, path string) tea.Cmd {
	if m.driver == nil {
		m.message = "Not in a tmux session"
		return nil
	}

	projInfo, err := workspace.GetProjectByPath(path)
	if err != nil {
		m.message = fmt.Sprintf("Failed to get project info: %v", err)
		return nil
	}
	sessionName := projInfo.Identifier("_")

	exists, err := m.driver.Exists(ctx, sessionName)
	if err != nil {
		m.message = fmt.Sprintf("Failed to check session: %v", err)
		return nil
	}
	if !exists {
		if err := m.driver.Launch(ctx, sessionName, path); err != nil {
			m.message = fmt.Sprintf("Failed to create session: %v", err)
			return nil
		}
	}
	if err := m.driver.SwitchTo(ctx, sessionName); err != nil {
		m.message = fmt.Sprintf("Failed to switch to session: %v", err)
		return nil
	}
	_ = m.store.RecordProjectAccess(path)
	m.message = fmt.Sprintf("Switching to %s...", sessionName)
	m.selectedPath = path
	m.quitting = true
	return tea.Quit
}
