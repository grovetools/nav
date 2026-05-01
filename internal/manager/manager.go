package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	core_config "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/mux"
	"github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/nav/pkg/api"
	navbindings "github.com/grovetools/nav/pkg/bindings"
)

type Manager struct {
	configDir     string
	coreConfig    *core_config.Config
	tmuxConfig    *TmuxConfig
	sessions      map[string]TmuxSessionConfig
	lockedKeys    []string
	configPath    string
	navConfigPath string // Path to the file containing nav config (may differ from configPath for fragments)
	sessionsPath  string
	tmuxClient    *tmux.Client
	activeGroup   string
	sessionsFile  TmuxSessionsFile
	undoStack     [][]byte
	redoStack     [][]byte
	daemonClient  daemon.Client // Daemon client for persisting bindings (uses LocalClient fallback when daemon is not running)
}

// managerState captures the full state for undo/redo operations
type managerState struct {
	TmuxConfig   TmuxConfig
	SessionsFile TmuxSessionsFile
	ActiveGroup  string
	LockedKeys   []string
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// tmuxCommand creates an exec.Command for tmux that respects GROVE_TMUX_SOCKET.
// This ensures all tmux operations use the same socket as the tmux client.
func tmuxCommand(args ...string) *exec.Cmd {
	if socket := os.Getenv(mux.EnvGroveTmuxSocket); socket != "" {
		// Prepend -L <socket> to use the isolated tmux server
		args = append([]string{"-L", socket}, args...)
	}
	return exec.Command("tmux", args...)
}

// Legacy types removed as discovery is now handled by grove-core's DiscoveryService.
// SearchPathConfig, ExplicitProject, and ProjectSearchConfig are no longer needed.

// NewManager creates a new Manager instance
func NewManager(configDir string) (*Manager, error) {
	// Expand paths
	configDir = expandPath(configDir)

	// Load the layered grove.yml configuration
	layered, err := core_config.LoadLayered(configDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load grove config: %w", err)
	}

	var coreCfg *core_config.Config
	if layered != nil {
		coreCfg = layered.Final
	}
	// If config doesn't exist, proceed with a default config object
	if coreCfg == nil {
		coreCfg = &core_config.Config{}
	}

	// Unmarshal the 'nav' extension (with backwards compatibility for 'tmux')
	var navCfg TmuxConfig
	if err := coreCfg.UnmarshalExtension("nav", &navCfg); err != nil {
		return nil, fmt.Errorf("failed to parse 'nav' config section: %w", err)
	}
	// Backwards compatibility: if 'nav' section is empty, try 'tmux'
	if navCfg.AvailableKeys == nil {
		if err := coreCfg.UnmarshalExtension("tmux", &navCfg); err != nil {
			return nil, fmt.Errorf("failed to parse 'tmux' config section: %w", err)
		}
	}

	// Find the primary config file path for saving
	configPath, err := core_config.FindConfigFile(configDir)
	if err != nil {
		// If no file exists, default to the standard global path
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "grove", "grove.yml")
	}

	// Find which file contains the nav.groups section for inline persistence
	// Check fragment files first, then fall back to primary config
	navConfigPath := configPath
	if layered != nil {
		navConfigPath = findNavGroupsSource(layered, configPath)
	}

	// Load sessions via the daemon client (LocalClient falls back to direct file I/O when daemon not running).
	// Use zero-arg: when embedded in treemux we inherit GROVE_SCOPE so the
	// nav manager shares the same daemon as its host instead of spawning
	// a new one keyed to configDir (which isn't a workspace anyway).
	sessionsPath := filepath.Join(paths.StateDir(), "nav", "sessions.yml")
	daemonClient := daemon.NewWithAutoStart()
	sessions := make(map[string]TmuxSessionConfig)
	var lockedKeys []string
	var sessionsFile TmuxSessionsFile

	if loaded, err := daemonClient.GetNavBindings(context.Background()); err == nil && loaded != nil {
		sessionsFile = *loaded
		if sessionsFile.Sessions != nil {
			sessions = sessionsFile.Sessions
		}

		// Migrate local locked keys to global locked keys
		lockedMap := make(map[string]bool)
		lockedKeys = sessionsFile.LockedKeys
		for _, k := range lockedKeys {
			lockedMap[k] = true
		}
		for _, gState := range sessionsFile.Groups {
			for _, k := range gState.LockedKeys {
				if !lockedMap[k] {
					lockedKeys = append(lockedKeys, k)
					lockedMap[k] = true
				}
			}
		}
		sessionsFile.LockedKeys = lockedKeys
	}
	// If file doesn't exist or is empty, sessions will be an empty map
	if sessions == nil {
		sessions = make(map[string]TmuxSessionConfig)
	}
	if sessionsFile.Groups == nil {
		sessionsFile.Groups = make(map[string]GroupState)
	}
	sessionsFile.Sessions = sessions
	sessionsFile.LockedKeys = lockedKeys

	// Initialize tmux client
	tmuxClient, clientErr := tmux.NewClient()
	if clientErr != nil {
		// Log warning but don't fail - some operations may still work
		fmt.Fprintf(os.Stderr, "Warning: could not initialize tmux client: %v\n", clientErr)
	}

	return &Manager{
		configDir:     configDir,
		coreConfig:    coreCfg,
		tmuxConfig:    &navCfg,
		sessions:      sessions,
		lockedKeys:    lockedKeys,
		configPath:    configPath,
		navConfigPath: navConfigPath,
		sessionsPath:  sessionsPath,
		tmuxClient:    tmuxClient,
		activeGroup:   "default",
		sessionsFile:  sessionsFile,
		undoStack:     make([][]byte, 0),
		redoStack:     make([][]byte, 0),
		daemonClient:  daemonClient,
	}, nil
}

// findNavGroupsSource finds which config file should contain nav.groups definitions.
// Priority: file with nav.groups > file with nav config > default path
func findNavGroupsSource(layered *core_config.LayeredConfig, defaultPath string) string {
	// First pass: check for files that already have nav.groups
	for _, fragment := range layered.GlobalFragments {
		if hasNavGroups(fragment.Config) {
			return fragment.Path
		}
	}
	if layered.Global != nil && hasNavGroups(layered.Global) {
		if globalPath, ok := layered.FilePaths[core_config.SourceGlobal]; ok {
			return globalPath
		}
	}

	// Second pass: find a file with any nav config (for new groups)
	for _, fragment := range layered.GlobalFragments {
		if hasNavConfig(fragment.Config) {
			return fragment.Path
		}
	}
	if layered.Global != nil && hasNavConfig(layered.Global) {
		if globalPath, ok := layered.FilePaths[core_config.SourceGlobal]; ok {
			return globalPath
		}
	}

	return defaultPath
}

// hasNavGroups checks if a config has nav.groups defined
func hasNavGroups(cfg *core_config.Config) bool {
	if cfg == nil || cfg.Extensions == nil {
		return false
	}
	nav, ok := cfg.Extensions["nav"]
	if !ok {
		return false
	}
	navMap, ok := nav.(map[string]interface{})
	if !ok {
		return false
	}
	_, hasGroups := navMap["groups"]
	return hasGroups
}

// hasNavConfig checks if a config has any nav configuration defined
func hasNavConfig(cfg *core_config.Config) bool {
	if cfg == nil || cfg.Extensions == nil {
		return false
	}
	_, ok := cfg.Extensions["nav"]
	return ok
}

// TakeSnapshot captures the current state for undo/redo operations.
// Call this before making any destructive changes to mappings or groups.
func (m *Manager) TakeSnapshot() {
	state := managerState{
		TmuxConfig:   *m.tmuxConfig,
		SessionsFile: m.sessionsFile,
		ActiveGroup:  m.activeGroup,
		LockedKeys:   m.lockedKeys,
	}
	data, err := json.Marshal(state)
	if err == nil {
		m.undoStack = append(m.undoStack, data)
		m.redoStack = m.redoStack[:0] // Clear redo stack on new action
	}
}

// Undo reverts the last data change.
func (m *Manager) Undo() error {
	if len(m.undoStack) == 0 {
		return fmt.Errorf("nothing to undo")
	}
	// Save current to redo stack
	currState := managerState{
		TmuxConfig:   *m.tmuxConfig,
		SessionsFile: m.sessionsFile,
		ActiveGroup:  m.activeGroup,
		LockedKeys:   m.lockedKeys,
	}
	currData, _ := json.Marshal(currState)
	m.redoStack = append(m.redoStack, currData)

	// Pop undo
	data := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	return m.restoreState(data)
}

// Redo re-applies a previously undone change.
func (m *Manager) Redo() error {
	if len(m.redoStack) == 0 {
		return fmt.Errorf("nothing to redo")
	}
	// Save current to undo stack
	currState := managerState{
		TmuxConfig:   *m.tmuxConfig,
		SessionsFile: m.sessionsFile,
		ActiveGroup:  m.activeGroup,
		LockedKeys:   m.lockedKeys,
	}
	currData, _ := json.Marshal(currState)
	m.undoStack = append(m.undoStack, currData)

	// Pop redo
	data := m.redoStack[len(m.redoStack)-1]
	m.redoStack = m.redoStack[:len(m.redoStack)-1]
	return m.restoreState(data)
}

// restoreState restores the manager state from a JSON snapshot.
func (m *Manager) restoreState(data []byte) error {
	var state managerState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	*m.tmuxConfig = state.TmuxConfig
	m.sessionsFile = state.SessionsFile
	m.lockedKeys = state.LockedKeys
	m.SetActiveGroup(state.ActiveGroup) // Will also sync m.sessions
	if err := m.saveStaticConfigFull(); err != nil {
		return err
	}
	if err := m.saveSessions(); err != nil {
		return err
	}
	return m.RegenerateBindingsGo()
}

// loadSessionsFromFile loads sessions from an external TOML or YAML file.
func (m *Manager) loadSessionsFromFile(path string) map[string]TmuxSessionConfig {
	var file struct {
		Nav struct {
			Sessions map[string]TmuxSessionConfig `yaml:"sessions" toml:"sessions"`
		} `yaml:"nav" toml:"nav"`
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if strings.HasSuffix(path, ".toml") {
			_, _ = toml.Decode(string(data), &file) // best-effort config parse
		} else {
			_ = yaml.Unmarshal(data, &file) // best-effort config parse
		}
		if file.Nav.Sessions != nil {
			return file.Nav.Sessions
		}
	}
	return make(map[string]TmuxSessionConfig)
}

// saveSessionsToFile saves sessions to an external TOML or YAML file.
// For TOML, writes simple "key = path" format when no extra metadata exists.
func (m *Manager) saveSessionsToFile(path string, sessions map[string]TmuxSessionConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// For TOML, use simpler format: key = "path" when no extra metadata
	if strings.HasSuffix(path, ".toml") {
		simpleSessions := make(map[string]string)
		for k, v := range sessions {
			// Only include path if Repository and Description are empty
			simpleSessions[k] = v.Path
		}
		wrapper := map[string]interface{}{
			"nav": map[string]interface{}{
				"sessions": simpleSessions,
			},
		}
		var buf strings.Builder
		enc := toml.NewEncoder(&buf)
		if err := enc.Encode(wrapper); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(buf.String()), 0o600)
	}

	// For YAML, use full struct
	wrapper := map[string]interface{}{
		"nav": map[string]interface{}{
			"sessions": sessions,
		},
	}
	data, err := yaml.Marshal(wrapper)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// SetActiveGroup switches the manager to operate on a different workspace group.
// When switching groups, sessions are swapped to the group's state.
// Locked keys are global and shared across all groups.
func (m *Manager) SetActiveGroup(group string) {
	if group == "" {
		group = "default"
	}
	m.activeGroup = group

	// lockedKeys is global and remains attached to m.lockedKeys

	if group == "default" {
		m.sessions = m.sessionsFile.Sessions
	} else {
		groupCfg, exists := m.tmuxConfig.Groups[group]
		isPersisted := false

		if exists && groupCfg.Persist != nil && groupCfg.Persist != false && groupCfg.Persist != "false" && groupCfg.Persist != "" {
			isPersisted = true
			if persistStr, ok := groupCfg.Persist.(string); ok && persistStr != "true" {
				targetPath := persistStr
				if !filepath.IsAbs(targetPath) {
					targetPath = filepath.Join(filepath.Dir(m.configPath), targetPath)
				}
				m.sessions = m.loadSessionsFromFile(targetPath)
			} else {
				if groupCfg.Sessions == nil {
					m.sessions = make(map[string]TmuxSessionConfig)
				} else {
					m.sessions = groupCfg.Sessions
				}
			}
		}

		if !isPersisted {
			if m.sessionsFile.Groups == nil {
				m.sessionsFile.Groups = make(map[string]GroupState)
			}
			state, exists := m.sessionsFile.Groups[group]
			if !exists {
				state = GroupState{Sessions: make(map[string]TmuxSessionConfig)}
				m.sessionsFile.Groups[group] = state
			}
			if state.Sessions == nil {
				state.Sessions = make(map[string]TmuxSessionConfig)
				m.sessionsFile.Groups[group] = state
			}
			m.sessions = state.Sessions
		}
	}
}

// GetGroupConfig returns the configuration for a specific group.
func (m *Manager) GetGroupConfig(group string) (GroupRef, bool) {
	if m.tmuxConfig == nil || m.tmuxConfig.Groups == nil {
		return GroupRef{}, false
	}
	cfg, ok := m.tmuxConfig.Groups[group]
	return cfg, ok
}

// GetGroupIcon returns the icon configured for a group, or "" if the group
// has no icon (or doesn't exist). This is the narrow accessor that the
// extracted sessionizer TUI uses instead of pulling the whole GroupRef
// across the package boundary.
func (m *Manager) GetGroupIcon(group string) string {
	cfg, ok := m.GetGroupConfig(group)
	if !ok {
		return ""
	}
	return cfg.Icon
}

// IsGroupExplicitlyInactive reports whether the group has its 'active' flag
// explicitly set to false. Returns false for groups that don't exist, that
// have no active flag set, or that are explicitly active. Used by the
// extracted groups TUI to render an "(Inactive)" status without having to
// import the manager.GroupRef struct.
func (m *Manager) IsGroupExplicitlyInactive(group string) bool {
	cfg, ok := m.GetGroupConfig(group)
	if !ok {
		return false
	}
	return cfg.Active != nil && !*cfg.Active
}

// ConfirmKeyUpdates returns whether to show confirmation prompts for bulk key update operations.
// Defaults to true if not configured.
func (m *Manager) ConfirmKeyUpdates() bool {
	if m.tmuxConfig == nil || m.tmuxConfig.ConfirmKeyUpdates == nil {
		return true // Default to showing confirmations
	}
	return *m.tmuxConfig.ConfirmKeyUpdates
}

// GetActiveGroup returns the name of the currently active workspace group.
func (m *Manager) GetActiveGroup() string {
	return m.activeGroup
}

// GetGroups returns a list of all available workspace groups (active only).
// Always includes "default" as the first group, followed by other groups sorted by Order, then alphabetically.
func (m *Manager) GetGroups() []string {
	groups := []string{"default"}
	if m.tmuxConfig != nil && m.tmuxConfig.Groups != nil {
		// Collect group names and sort for deterministic ordering
		var otherGroups []string
		for g, ref := range m.tmuxConfig.Groups {
			// Only include active groups (nil or true = active, false = deactivated)
			if ref.Active == nil || *ref.Active {
				otherGroups = append(otherGroups, g)
			}
		}
		// Sort by Order field, then alphabetically for ties
		sort.Slice(otherGroups, func(i, j int) bool {
			groupI := m.tmuxConfig.Groups[otherGroups[i]]
			groupJ := m.tmuxConfig.Groups[otherGroups[j]]
			if groupI.Order != groupJ.Order {
				return groupI.Order < groupJ.Order
			}
			return otherGroups[i] < otherGroups[j]
		})
		groups = append(groups, otherGroups...)
	}
	return groups
}

// GetAllGroups returns a list of all available workspace groups, including deactivated ones.
// Sorted by Order field, then alphabetically for ties.
func (m *Manager) GetAllGroups() []string {
	groups := []string{"default"}
	if m.tmuxConfig != nil && m.tmuxConfig.Groups != nil {
		var otherGroups []string
		for g := range m.tmuxConfig.Groups {
			otherGroups = append(otherGroups, g)
		}
		// Sort by Order field, then alphabetically for ties
		sort.Slice(otherGroups, func(i, j int) bool {
			groupI := m.tmuxConfig.Groups[otherGroups[i]]
			groupJ := m.tmuxConfig.Groups[otherGroups[j]]
			if groupI.Order != groupJ.Order {
				return groupI.Order < groupJ.Order
			}
			return otherGroups[i] < otherGroups[j]
		})
		groups = append(groups, otherGroups...)
	}
	return groups
}

// CreateGroup creates a new group.
func (m *Manager) CreateGroup(name, prefix string) error {
	if name == "" || name == "default" {
		return fmt.Errorf("invalid group name")
	}
	if m.tmuxConfig.Groups == nil {
		m.tmuxConfig.Groups = make(map[string]GroupRef)
	}
	if _, exists := m.tmuxConfig.Groups[name]; exists {
		return fmt.Errorf("group already exists")
	}
	active := true
	m.tmuxConfig.Groups[name] = GroupRef{
		Prefix: prefix,
		Active: &active,
		Order:  len(m.tmuxConfig.Groups), // Set default order to end of list
	}
	return m.saveStaticConfigFull()
}

// DeleteGroup removes a group from config and state.
func (m *Manager) DeleteGroup(name string) error {
	if name == "default" {
		return fmt.Errorf("cannot delete default group")
	}
	// If we're deleting the active group, switch to default first
	// to prevent saveSessions() from re-adding the deleted group's state
	if m.activeGroup == name {
		m.SetActiveGroup("default")
	}
	if m.sessionsFile.LastAccessedGroup == name {
		m.sessionsFile.LastAccessedGroup = "default"
	}
	if m.tmuxConfig.Groups != nil {
		delete(m.tmuxConfig.Groups, name)
	}
	if m.sessionsFile.Groups != nil {
		delete(m.sessionsFile.Groups, name)
	}
	if err := m.saveStaticConfigFull(); err != nil {
		return err
	}
	return m.saveSessions()
}

// SetGroupActive sets the active state of a group.
func (m *Manager) SetGroupActive(name string, active bool) error {
	if name == "default" {
		return fmt.Errorf("cannot modify default group active state")
	}
	if ref, exists := m.tmuxConfig.Groups[name]; exists {
		ref.Active = &active
		m.tmuxConfig.Groups[name] = ref
		return m.saveStaticConfigFull()
	}
	return fmt.Errorf("group not found")
}

// RenameGroup renames a group, updating both static config and state.
func (m *Manager) RenameGroup(oldName, newName string) error {
	if oldName == "default" || newName == "default" {
		return fmt.Errorf("cannot rename default group")
	}
	if oldName == newName {
		return nil
	}
	if newName == "" {
		return fmt.Errorf("new group name cannot be empty")
	}
	if _, exists := m.tmuxConfig.Groups[newName]; exists {
		return fmt.Errorf("group %s already exists", newName)
	}

	// Swap in static config
	if cfg, exists := m.tmuxConfig.Groups[oldName]; exists {
		m.tmuxConfig.Groups[newName] = cfg
		delete(m.tmuxConfig.Groups, oldName)
	} else {
		return fmt.Errorf("group %s not found", oldName)
	}

	// Swap in state/sessions file
	if state, exists := m.sessionsFile.Groups[oldName]; exists {
		m.sessionsFile.Groups[newName] = state
		delete(m.sessionsFile.Groups, oldName)
	}

	// Update active string references
	if m.activeGroup == oldName {
		m.activeGroup = newName
	}
	if m.sessionsFile.LastAccessedGroup == oldName {
		m.sessionsFile.LastAccessedGroup = newName
	}

	if err := m.saveStaticConfigFull(); err != nil {
		return err
	}
	return m.saveSessions()
}

// SetGroupOrder sets the display order for a group.
func (m *Manager) SetGroupOrder(name string, order int) error {
	if name == "default" {
		return nil // default group is always first
	}
	if ref, exists := m.tmuxConfig.Groups[name]; exists {
		ref.Order = order
		m.tmuxConfig.Groups[name] = ref
		return m.saveStaticConfigFull()
	}
	return fmt.Errorf("group not found")
}

// SetGroupPrefix sets the prefix key for a group.
func (m *Manager) SetGroupPrefix(name, prefix string) error {
	if name == "default" {
		return fmt.Errorf("use main prefix setting for default group")
	}
	if ref, exists := m.tmuxConfig.Groups[name]; exists {
		ref.Prefix = prefix
		m.tmuxConfig.Groups[name] = ref
		return m.saveStaticConfigFull()
	}
	return fmt.Errorf("group not found")
}

// GetGroupSessionCount returns the number of sessions in a group.
func (m *Manager) GetGroupSessionCount(name string) int {
	if name == "default" {
		return len(m.sessionsFile.Sessions)
	}
	// Check static config first (for inline persistence)
	if ref, exists := m.tmuxConfig.Groups[name]; exists && ref.Sessions != nil {
		if len(ref.Sessions) > 0 {
			return len(ref.Sessions)
		}
	}
	// Check state file
	if state, exists := m.sessionsFile.Groups[name]; exists && state.Sessions != nil {
		return len(state.Sessions)
	}
	return 0
}

// FindGroupForPath finds which group contains a session with the given path.
// It matches if the target path equals a mapped path OR is inside a mapped path.
// Prioritizes "default" group if it has a match, otherwise returns the most specific match.
// Uses pathutil.NormalizeForLookup to handle symlinks and case sensitivity on macOS.
func (m *Manager) FindGroupForPath(targetPath string) string {
	// Normalize the target path to resolve symlinks and handle case sensitivity
	targetNormalized, err := pathutil.NormalizeForLookup(expandPath(targetPath))
	if err != nil {
		// Fallback to Abs if normalization fails
		targetNormalized, err = filepath.Abs(expandPath(targetPath))
		if err != nil {
			return ""
		}
	}

	originalGroup := m.activeGroup
	defer m.SetActiveGroup(originalGroup)

	bestMatch := ""
	bestMatchLen := 0
	defaultMatchLen := 0

	for _, g := range m.GetAllGroups() {
		m.SetActiveGroup(g)
		sessions, _ := m.GetSessions()
		for _, s := range sessions {
			if s.Path != "" {
				// Normalize the session path to resolve symlinks
				pNormalized, err := pathutil.NormalizeForLookup(expandPath(s.Path))
				if err != nil {
					// Fallback to Abs if normalization fails
					pNormalized, err = filepath.Abs(expandPath(s.Path))
					if err != nil {
						continue
					}
				}
				// Check if target path equals or is inside the mapped path
				if targetNormalized == pNormalized || strings.HasPrefix(targetNormalized, pNormalized+string(filepath.Separator)) {
					// Track default group matches separately for prioritization
					if g == "default" && len(pNormalized) > defaultMatchLen {
						defaultMatchLen = len(pNormalized)
					}
					// Keep track of the most specific (longest) match
					if len(pNormalized) > bestMatchLen {
						bestMatch = g
						bestMatchLen = len(pNormalized)
					} else if len(pNormalized) == bestMatchLen && g == "default" {
						// Prefer default group when match lengths are equal
						bestMatch = g
					}
				}
			}
		}
	}

	// Prioritize default group if it has any match
	if defaultMatchLen > 0 {
		return "default"
	}
	return bestMatch
}

// SetLastAccessedGroup updates the access history.
func (m *Manager) SetLastAccessedGroup(group string) error {
	m.sessionsFile.LastAccessedGroup = group
	return m.saveSessions()
}

// GetLastAccessedGroup returns the most recently accessed group.
func (m *Manager) GetLastAccessedGroup() string {
	return m.sessionsFile.LastAccessedGroup
}

// GetPrefixForGroup returns the configured prefix for a specific group.
func (m *Manager) GetPrefixForGroup(group string) string {
	if group == "default" || group == "" {
		if m.tmuxConfig == nil || m.tmuxConfig.Prefix == "" {
			return "<prefix>"
		}
		return m.tmuxConfig.Prefix
	}
	if m.tmuxConfig != nil && m.tmuxConfig.Groups != nil {
		if g, ok := m.tmuxConfig.Groups[group]; ok {
			// Return the group's prefix (empty string means no keybindings)
			return g.Prefix
		}
	}
	// Group not found, return empty (no keybindings)
	return ""
}

func (m *Manager) GetLockedKeys() []string {
	return m.lockedKeys
}

// GetPrefix returns the configured prefix for the active group's nav bindings.
// Defaults to "<prefix>" (native tmux prefix table) for backwards compatibility.
func (m *Manager) GetPrefix() string {
	return m.GetPrefixForGroup(m.activeGroup)
}

// GetDefaultIcon returns the configured icon for the default group.
func (m *Manager) GetDefaultIcon() string {
	if m.tmuxConfig == nil {
		return ""
	}
	return m.tmuxConfig.DefaultIcon
}

func (m *Manager) GetSessions() ([]models.TmuxSession, error) {
	if m.tmuxConfig == nil {
		return []models.TmuxSession{}, nil
	}

	// Create sessions for all available keys
	sessions := make([]models.TmuxSession, 0, len(m.tmuxConfig.AvailableKeys))

	// Add all available keys as sessions (empty if not configured)
	for _, key := range m.tmuxConfig.AvailableKeys {
		if sessionData, exists := m.sessions[key]; exists {
			// Key has a configured session
			sessions = append(sessions, models.TmuxSession{
				Key:  key,
				Path: expandPath(sessionData.Path),
			})
		} else {
			// Key exists but has no session configured
			sessions = append(sessions, models.TmuxSession{
				Key:         key,
				Path:        "",
				Repository:  "",
				Description: "",
			})
		}
	}

	return sessions, nil
}

// Save persists the tmux configuration:
// - Dynamic state (session mappings) to nav/sessions.yml or persist file
// - Static config is only saved for inline persistence (persist=true)
func (m *Manager) Save() error {
	// Handle inline persistence (persist=true) - saves sessions directly in config file
	if m.activeGroup != "default" && m.activeGroup != "" {
		if groupCfg, exists := m.tmuxConfig.Groups[m.activeGroup]; exists {
			if groupCfg.Persist != nil && groupCfg.Persist != false && groupCfg.Persist != "false" && groupCfg.Persist != "" {
				if persistStr, ok := groupCfg.Persist.(string); !ok || persistStr == "true" {
					// Inline persistence - save sessions to the nav config file (e.g., keys.toml)
					groupCfg.Sessions = m.sessions
					m.tmuxConfig.Groups[m.activeGroup] = groupCfg
					if err := m.saveStaticConfig(); err != nil {
						return err
					}
				}
			}
		}
	}

	// Save sessions to separate file (persist="filename.toml") or state file
	return m.saveSessions()
}

// saveStaticConfig saves the static tmux configuration to the nav config file.
// For inline persistence (persist=true), it uses navConfigPath which points to the
// fragment file containing nav.groups (e.g., keys.toml), not the primary config.
// Uses surgical text editing to preserve file structure, comments, and formatting.
func (m *Manager) saveStaticConfig() error {
	targetPath := m.navConfigPath

	// Ensure the config directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if m.activeGroup == "" || m.activeGroup == "default" {
		return nil
	}

	groupCfg, exists := m.tmuxConfig.Groups[m.activeGroup]
	if !exists {
		return nil
	}

	// Read the existing file
	data, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	content := string(data)
	isTOML := strings.HasSuffix(targetPath, ".toml")

	if isTOML {
		content = m.updateTOMLSessions(content, m.activeGroup, groupCfg.Sessions)
	} else {
		// For YAML, fall back to full rewrite (less common case)
		return m.saveStaticConfigFull()
	}

	return os.WriteFile(targetPath, []byte(content), 0o600)
}

// updateTOMLSessions performs a surgical update of the sessions section in a TOML file.
// It preserves the rest of the file structure, comments, and formatting.
func (m *Manager) updateTOMLSessions(content, group string, sessions map[string]TmuxSessionConfig) string {
	sessionsHeader := fmt.Sprintf("[nav.groups.%s.sessions]", group)
	groupHeader := fmt.Sprintf("[nav.groups.%s]", group)

	// Build new sessions content
	var sessionLines []string
	// Sort keys for consistent output
	keys := make([]string, 0, len(sessions))
	for k := range sessions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sessionLines = append(sessionLines, fmt.Sprintf("%s = %q", k, sessions[k].Path))
	}
	newSessionsContent := strings.Join(sessionLines, "\n")

	// Check if sessions section already exists
	if idx := strings.Index(content, sessionsHeader); idx != -1 {
		// Find the end of the sessions section (next [...] header or EOF)
		startOfValues := idx + len(sessionsHeader)
		endIdx := len(content)

		// Find next section header
		remaining := content[startOfValues:]
		if nextBracket := strings.Index(remaining, "\n["); nextBracket != -1 {
			endIdx = startOfValues + nextBracket
		}

		// Replace the sessions section
		newContent := content[:idx] + sessionsHeader + "\n" + newSessionsContent + "\n" + content[endIdx:]
		return newContent
	}

	// Sessions section doesn't exist, need to add it after group header
	if idx := strings.Index(content, groupHeader); idx != -1 {
		// Find where this group's content ends (next [...] at same or higher level, or EOF)
		startOfGroup := idx + len(groupHeader)
		insertIdx := len(content)

		// Find next section that's not a child of this group
		remaining := content[startOfGroup:]
		lines := strings.Split(remaining, "\n")
		offset := startOfGroup
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[nav.groups."+group+".") {
				insertIdx = offset
				break
			}
			offset += len(line) + 1 // +1 for newline
		}

		// Insert sessions section before the next section
		insertion := "\n" + sessionsHeader + "\n" + newSessionsContent + "\n"
		newContent := content[:insertIdx] + insertion + content[insertIdx:]
		return newContent
	}

	// Group doesn't exist at all - append at end of nav section or file
	// This is a fallback; normally the group should exist
	return content + "\n" + sessionsHeader + "\n" + newSessionsContent + "\n"
}

// saveStaticConfigFull is the fallback that rewrites the entire config file.
// Used for YAML files or when surgical editing isn't possible.
func (m *Manager) saveStaticConfigFull() error {
	targetPath := m.navConfigPath
	isTOML := strings.HasSuffix(targetPath, ".toml")

	var fullConfig map[string]interface{}
	data, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing config file: %w", err)
	}

	if len(data) > 0 {
		if isTOML {
			if _, err := toml.Decode(string(data), &fullConfig); err != nil {
				return fmt.Errorf("failed to parse existing config file: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, &fullConfig); err != nil {
				return fmt.Errorf("failed to parse existing config file: %w", err)
			}
		}
	} else {
		fullConfig = make(map[string]interface{})
	}

	navSection, ok := fullConfig["nav"].(map[string]interface{})
	if !ok {
		navSection = make(map[string]interface{})
	}

	// Start fresh - only save groups that exist in tmuxConfig
	// (this ensures deleted groups are removed from the file)
	groups := make(map[string]interface{})

	// Save ALL groups from tmuxConfig
	for groupName, groupCfg := range m.tmuxConfig.Groups {
		groupMap, ok := groups[groupName].(map[string]interface{})
		if !ok {
			groupMap = make(map[string]interface{})
		}
		// Save prefix if set
		if groupCfg.Prefix != "" {
			groupMap["prefix"] = groupCfg.Prefix
		}
		// Save active state if explicitly set to false
		if groupCfg.Active != nil && !*groupCfg.Active {
			groupMap["active"] = false
		}
		// Save icon if set
		if groupCfg.Icon != "" {
			groupMap["icon"] = groupCfg.Icon
		}
		// Save order if set
		if groupCfg.Order != 0 {
			groupMap["order"] = groupCfg.Order
		}
		// Save sessions if any
		if len(groupCfg.Sessions) > 0 {
			sessionsMap := make(map[string]string)
			for key, sess := range groupCfg.Sessions {
				sessionsMap[key] = sess.Path
			}
			groupMap["sessions"] = sessionsMap
		}
		groups[groupName] = groupMap
	}

	navSection["groups"] = groups
	fullConfig["nav"] = navSection

	var newData []byte
	if isTOML {
		var buf strings.Builder
		enc := toml.NewEncoder(&buf)
		if err := enc.Encode(fullConfig); err != nil {
			return fmt.Errorf("failed to marshal updated config: %w", err)
		}
		newData = []byte(buf.String())
	} else {
		newData, err = yaml.Marshal(fullConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal updated config: %w", err)
		}
	}

	return os.WriteFile(targetPath, newData, 0o600)
}

// saveSessions persists the in-memory session state via the daemon client.
// The daemon (or LocalClient fallback) owns the sessions.yml file and broadcasts SSE updates.
// External persistence modes (persist="filename") still write directly because they touch
// user-owned config files outside the daemon's state directory.
func (m *Manager) saveSessions() error {
	ctx := context.Background()

	// Global locked keys
	m.sessionsFile.LockedKeys = m.lockedKeys

	// Update the sessions file structure based on active group
	if m.activeGroup == "default" || m.activeGroup == "" {
		m.sessionsFile.Sessions = m.sessions
	} else {
		// Check persist setting from in-memory config
		var currentPersist interface{}
		if groupCfg, exists := m.tmuxConfig.Groups[m.activeGroup]; exists {
			currentPersist = groupCfg.Persist
		}

		isPersisted := false
		if currentPersist != nil && currentPersist != false && currentPersist != "false" && currentPersist != "" {
			isPersisted = true
			if persistStr, ok := currentPersist.(string); ok && persistStr != "true" {
				targetPath := persistStr
				if !filepath.IsAbs(targetPath) {
					targetPath = filepath.Join(filepath.Dir(m.configPath), targetPath)
				}
				// Filter out locked keys - they're stored only in default
				filteredSessions := make(map[string]TmuxSessionConfig)
				for key, sess := range m.sessions {
					isLocked := false
					for _, lk := range m.lockedKeys {
						if lk == key {
							isLocked = true
							break
						}
					}
					if !isLocked {
						filteredSessions[key] = sess
					}
				}
				// External persist mode: write to user-owned file directly (not daemon-managed).
				if err := m.saveSessionsToFile(targetPath, filteredSessions); err != nil {
					return err
				}
			}
		}

		if !isPersisted {
			if m.sessionsFile.Groups == nil {
				m.sessionsFile.Groups = make(map[string]GroupState)
			}
			// Filter out locked keys - they're stored only in default
			filteredSessions := make(map[string]TmuxSessionConfig)
			for key, sess := range m.sessions {
				isLocked := false
				for _, lk := range m.lockedKeys {
					if lk == key {
						isLocked = true
						break
					}
				}
				if !isLocked {
					filteredSessions[key] = sess
				}
			}
			m.sessionsFile.Groups[m.activeGroup] = GroupState{
				Sessions: filteredSessions,
			}
		}
	}

	// Push the active group to the daemon. The daemon persists sessions.yml and
	// broadcasts an SSE update; LocalClient fallback writes the file directly.
	if m.daemonClient != nil {
		groupName := m.activeGroup
		if groupName == "" {
			groupName = "default"
		}

		var groupState GroupState
		if groupName == "default" {
			groupState = GroupState{Sessions: m.sessionsFile.Sessions}
		} else if existing, ok := m.sessionsFile.Groups[groupName]; ok {
			groupState = existing
		}

		if err := m.daemonClient.UpdateNavGroup(ctx, groupName, groupState); err != nil {
			return fmt.Errorf("failed to update nav group via daemon: %w", err)
		}

		if err := m.daemonClient.UpdateNavLockedKeys(ctx, m.lockedKeys); err != nil {
			return fmt.Errorf("failed to update nav locked keys via daemon: %w", err)
		}

		if m.sessionsFile.LastAccessedGroup != "" {
			if err := m.daemonClient.SetNavLastAccessedGroup(ctx, m.sessionsFile.LastAccessedGroup); err != nil {
				return fmt.Errorf("failed to update nav last-accessed group via daemon: %w", err)
			}
		}
		return nil
	}

	// Fallback: no daemon client (should not happen — daemon.New() always returns one).
	data, err := yaml.Marshal(m.sessionsFile)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(m.sessionsPath), 0o755); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}
	return os.WriteFile(m.sessionsPath, data, 0o600)
}

func (m *Manager) UpdateSessions(sessions []models.TmuxSession) error {
	return m.UpdateSessionsAndLocks(sessions, m.lockedKeys)
}

func (m *Manager) UpdateSessionsAndLocks(sessions []models.TmuxSession, lockedKeys []string) error {
	// Convert slice back to map format, only including non-empty sessions
	sessionsMap := make(map[string]TmuxSessionConfig)

	for _, session := range sessions {
		// Only save sessions that have actual data
		if session.Path != "" {
			sessionsMap[session.Key] = TmuxSessionConfig{
				Path: session.Path,
			}
		}
	}

	m.sessions = sessionsMap
	m.lockedKeys = lockedKeys

	return m.Save()
}

func (m *Manager) UpdateSingleSession(key string, session models.TmuxSession) error {
	if m.sessions == nil {
		m.sessions = make(map[string]TmuxSessionConfig)
	}

	m.sessions[key] = TmuxSessionConfig{
		Path: session.Path,
	}

	// Add key to available_keys if it's not there
	keyExists := false
	for _, k := range m.tmuxConfig.AvailableKeys {
		if k == key {
			keyExists = true
			break
		}
	}
	if !keyExists {
		m.tmuxConfig.AvailableKeys = append(m.tmuxConfig.AvailableKeys, key)
	}

	return m.Save()
}

// GetAvailableProjects uses the daemon client to fetch enriched workspaces.
// If the daemon is running, it uses cached/pre-computed data including git status.
// If not, it falls back to direct discovery via LocalClient.
func (m *Manager) GetAvailableProjects() ([]api.Project, error) {
	// Create daemon client — inherit GROVE_SCOPE from parent (e.g. treemux)
	// so embedded navs share the host's daemon instead of spawning one
	// keyed to configDir (which isn't a workspace).
	client := daemon.NewWithAutoStart()
	defer client.Close()

	// Fetch enriched workspaces via the client interface
	// This includes pre-computed git status when the daemon is running
	enrichedWorkspaces, err := client.GetEnrichedWorkspaces(context.Background(), nil)
	if err != nil {
		// Return an empty list if discovery fails - sessionize will handle the empty case
		// This allows first-run setup to trigger
		return []api.Project{}, fmt.Errorf("failed to get workspaces: %w", err)
	}

	// Transform []*models.EnrichedWorkspace into []api.Project. Both types now
	// use core/pkg/models enrichment types directly, so no field conversion needed.
	projects := make([]api.Project, len(enrichedWorkspaces))
	for i, ew := range enrichedWorkspaces {
		projects[i] = api.Project{
			WorkspaceNode: ew.WorkspaceNode,
			GitStatus:     ew.GitStatus,
			GitRemoteURL:  ew.GitRemoteURL,
			NoteCounts:    ew.NoteCounts,
			PlanStats:     ew.PlanStats,
			ReleaseInfo:   ew.ReleaseInfo,
			ActiveBinary:  ew.ActiveBinary,
			CxStats:       ew.CxStats,
		}
	}

	return projects, nil
}

// GetAvailableProjectsWithOptions is now a convenience wrapper around GetAvailableProjects.
// The enrichment options are no longer used here as enrichment is handled by the caller (TUI).
func (m *Manager) GetAvailableProjectsWithOptions(enrichOpts interface{}) ([]DiscoveredProject, error) {
	return m.GetAvailableProjects()
}

func (m *Manager) GetAvailableProjectsSorted() ([]DiscoveredProject, error) {
	projects, err := m.GetAvailableProjects()
	if err != nil {
		return nil, err
	}

	// Load access history and sort projects
	history, err := workspace.LoadAccessHistory(m.configDir)
	if err != nil {
		// If we can't load history, just return unsorted
		return projects, nil
	}

	// Use local SortProjectsByAccess which understands SessionizeProject
	return SortProjectsByAccess(history, projects), nil
}

func (m *Manager) RecordProjectAccess(path string) error {
	history, err := workspace.LoadAccessHistory(m.configDir)
	if err != nil {
		return err
	}

	history.RecordAccess(path)
	return history.Save(m.configDir)
}

func (m *Manager) GetAccessHistory() (*workspace.AccessHistory, error) {
	return workspace.LoadAccessHistory(m.configDir)
}

// GetEnabledSearchPaths is deprecated as search paths are now managed
// via grove-core's DiscoveryService using the global grove.yml 'groves' configuration.
func (m *Manager) GetEnabledSearchPaths() ([]string, error) {
	// This method is kept for backward compatibility but no longer used.
	// Discovery is now handled by DiscoveryService.
	return []string{}, nil
}

func (m *Manager) RegenerateBindings() error {
	// Use Go implementation instead of Python script
	return m.RegenerateBindingsGo()
}

func (m *Manager) GetGitStatuses() (map[string]models.GitStatus, error) {
	sessions, err := m.GetSessions()
	if err != nil {
		return nil, err
	}

	statuses := make(map[string]models.GitStatus)

	for _, session := range sessions {
		if session.Path == "" {
			continue
		}

		// Derive repo name from path
		repoName := filepath.Base(session.Path)
		status := m.GetGitStatus(session.Path, repoName)
		statuses[repoName] = status
	}

	return statuses, nil
}

func (m *Manager) GetGitStatus(path, repo string) models.GitStatus {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(os.Getenv("HOME"), path[2:])
	}

	// Check if directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return models.GitStatus{
			Repository: repo,
			Status:     "path not found",
			HasChanges: false,
			IsClean:    false,
		}
	}

	// Check if it's a git repository
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return models.GitStatus{
			Repository: repo,
			Status:     "not a git repo",
			HasChanges: false,
			IsClean:    false,
		}
	}

	// Get git status using git commands
	status := m.buildGitStatus(path)

	return models.GitStatus{
		Repository: repo,
		Status:     status,
		HasChanges: status != "*",
		IsClean:    status == "*",
	}
}

func (m *Manager) buildGitStatus(path string) string {
	var statusParts []string

	// Get untracked files
	untracked := m.runGitCommand(path, "ls-files", "--others", "--exclude-standard")
	untrackedCount := len(strings.Split(strings.TrimSpace(untracked), "\n"))
	if untracked != "" && untrackedCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("?%d", untrackedCount))
	}

	// Get modified files
	modified := m.runGitCommand(path, "diff", "--name-only")
	modifiedCount := len(strings.Split(strings.TrimSpace(modified), "\n"))
	if modified != "" && modifiedCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("M%d", modifiedCount))
	}

	// Get staged files
	staged := m.runGitCommand(path, "diff", "--cached", "--name-only")
	stagedCount := len(strings.Split(strings.TrimSpace(staged), "\n"))
	if staged != "" && stagedCount > 0 {
		statusParts = append(statusParts, fmt.Sprintf("●%d", stagedCount))
	}

	// Get upstream tracking status (ahead/behind remote)
	upstream := m.runGitCommand(path, "rev-parse", "--abbrev-ref", "@{u}")
	if upstream != "" {
		counts := m.runGitCommand(path, "rev-list", "--left-right", "--count", "HEAD...@{u}")
		if counts != "" {
			parts := strings.Fields(counts)
			if len(parts) == 2 {
				ahead, _ := strconv.Atoi(parts[0])
				behind, _ := strconv.Atoi(parts[1])
				if ahead > 0 {
					statusParts = append(statusParts, fmt.Sprintf("↑%d", ahead))
				}
				if behind > 0 {
					statusParts = append(statusParts, fmt.Sprintf("↓%d", behind))
				}
			}
		}
	}

	// Get main/master branch tracking status
	currentBranch := m.runGitCommand(path, "branch", "--show-current")
	var mainBranch string

	// Check for main or master branch
	if m.runGitCommand(path, "show-ref", "--verify", "--quiet", "refs/heads/main") == "" {
		mainBranch = "main"
	} else if m.runGitCommand(path, "show-ref", "--verify", "--quiet", "refs/heads/master") == "" {
		mainBranch = "master"
	}

	// If we're on main/master and no upstream is set, compare against origin/main or origin/master
	if mainBranch != "" && currentBranch == mainBranch && upstream == "" {
		// Check if origin/main or origin/master exists
		remoteRef := fmt.Sprintf("origin/%s", mainBranch)
		if m.runGitCommand(path, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/remotes/%s", remoteRef)) == "" {
			counts := m.runGitCommand(path, "rev-list", "--left-right", "--count", fmt.Sprintf("HEAD...%s", remoteRef))
			if counts != "" {
				parts := strings.Fields(counts)
				if len(parts) == 2 {
					ahead, _ := strconv.Atoi(parts[0])
					behind, _ := strconv.Atoi(parts[1])
					if ahead > 0 {
						statusParts = append(statusParts, fmt.Sprintf("⇡%d", ahead))
					}
					if behind > 0 {
						statusParts = append(statusParts, fmt.Sprintf("⇣%d", behind))
					}
				}
			}
		}
	}

	// If we're not on main/master and it exists, show ahead/behind main
	if mainBranch != "" && currentBranch != mainBranch && currentBranch != "" {
		counts := m.runGitCommand(path, "rev-list", "--left-right", "--count", fmt.Sprintf("HEAD...%s", mainBranch))
		if counts != "" {
			parts := strings.Fields(counts)
			if len(parts) == 2 {
				ahead, _ := strconv.Atoi(parts[0])
				behind, _ := strconv.Atoi(parts[1])
				if ahead > 0 {
					statusParts = append(statusParts, fmt.Sprintf("⇡%d", ahead))
				}
				if behind > 0 {
					statusParts = append(statusParts, fmt.Sprintf("⇣%d", behind))
				}
			}
		}
	}

	// Get line change stats
	stats := m.runGitCommand(path, "diff", "--numstat")
	stagedStats := m.runGitCommand(path, "diff", "--cached", "--numstat")

	additions, deletions := m.parseNumStats(stats + "\n" + stagedStats)
	if additions > 0 || deletions > 0 {
		statusParts = append(statusParts, fmt.Sprintf("+%d", additions))
		statusParts = append(statusParts, fmt.Sprintf("-%d", deletions))
	}

	if len(statusParts) == 0 {
		return "*"
	}

	return strings.Join(statusParts, " ")
}

func (m *Manager) runGitCommand(path string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (m *Manager) parseNumStats(stats string) (int, int) {
	var totalAdd, totalDel int

	lines := strings.Split(strings.TrimSpace(stats), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			if add, err := strconv.Atoi(parts[0]); err == nil && parts[0] != "-" {
				totalAdd += add
			}
			if del, err := strconv.Atoi(parts[1]); err == nil && parts[1] != "-" {
				totalDel += del
			}
		}
	}

	return totalAdd, totalDel
}

// Sessionize creates or switches to a tmux session for the given path
func (m *Manager) Sessionize(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	expandedPath := expandPath(path)

	// Get project info to generate proper session name
	projInfo, err := workspace.GetProjectByPath(expandedPath)
	if err != nil {
		return fmt.Errorf("failed to get project info: %w", err)
	}
	sessionName := projInfo.Identifier("_")

	ctx := context.Background()

	// Check if tmux is running
	tmuxRunning := m.isTmuxRunning()
	inTmux := mux.ActiveMux() != mux.MuxNone

	// If tmux is not running and we're not in tmux, start new session
	if !tmuxRunning && !inTmux {
		// Need to use exec.Command directly for interactive session
		// Use tmuxCommand to respect GROVE_TMUX_SOCKET
		cmd := tmuxCommand("new-session", "-s", sessionName, "-c", expandedPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Check if tmux client is available
	if m.tmuxClient == nil {
		return fmt.Errorf("tmux client not initialized")
	}

	// Check if session already exists
	exists, err := m.tmuxClient.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}

	if !exists {
		// Create new detached session using the tmux client
		opts := tmux.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: expandedPath,
		}
		if err := m.tmuxClient.Launch(ctx, opts); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Switch to the session if we're in tmux
	if inTmux {
		return m.tmuxClient.SwitchClientToSession(ctx, sessionName)
	}

	// Attach to the session if we're outside tmux
	// Need to use exec.Command directly for interactive attach
	// Use tmuxCommand to respect GROVE_TMUX_SOCKET
	cmd := tmuxCommand("attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *Manager) isTmuxRunning() bool {
	if m.tmuxClient == nil {
		return false
	}
	// Check if tmux server is running by trying to list sessions
	_, err := m.tmuxClient.ListSessions(context.Background())
	return err == nil || !strings.Contains(err.Error(), "no server")
}

// RegenerateBindingsGo generates tmux key bindings in Go (replacing Python script).
// It processes all configured workspace groups and generates separate binding files.
// Delegates to nav/pkg/bindings.GenerateTmuxConf for the actual generation.
func (m *Manager) RegenerateBindingsGo() error {
	// Preserve current active group
	originalGroup := m.activeGroup
	defer m.SetActiveGroup(originalGroup)

	groupNames := m.GetGroups()
	groupBindings := make([]navbindings.GroupBinding, 0, len(groupNames))

	for _, group := range groupNames {
		m.SetActiveGroup(group)
		prefix := m.GetPrefix()

		// Build session map from the Manager's internal state
		sessionMap := make(map[string]models.NavSessionConfig)
		for key, sess := range m.sessions {
			sessionMap[key] = sess
		}

		groupBindings = append(groupBindings, navbindings.GroupBinding{
			Name:     group,
			Prefix:   prefix,
			Sessions: sessionMap,
		})
	}

	return navbindings.GenerateTmuxConf(groupBindings, paths.BinDir(), paths.CacheDir())
}

// DetectTmuxKeyForPath detects the tmux session key for a given working directory
func (m *Manager) DetectTmuxKeyForPath(workingDir string) string {
	// Check if we're in a tmux session
	if mux.ActiveMux() == mux.MuxNone {
		return ""
	}

	if m.sessions == nil {
		return ""
	}

	// Normalize working directory
	absWorkingDir, _ := filepath.Abs(workingDir)

	// Find matching session by path
	for key, session := range m.sessions {
		sessionPath := expandPath(session.Path)
		absSessionPath, _ := filepath.Abs(sessionPath)

		// Check if paths match
		if absWorkingDir == absSessionPath {
			return key
		}
	}

	return ""
}

// GetAvailableKeys returns all available keys from configuration
func (m *Manager) GetAvailableKeys() []string {
	if m.tmuxConfig == nil {
		return []string{}
	}
	return m.tmuxConfig.AvailableKeys
}

// UpdateSessionKey updates the key for a specific session
func (m *Manager) UpdateSessionKey(oldKey, newKey string) error {
	if oldKey == newKey {
		return nil // No change needed
	}

	if m.sessions == nil {
		return fmt.Errorf("sessions not loaded")
	}

	// Check if old key exists
	session, exists := m.sessions[oldKey]
	if !exists {
		return fmt.Errorf("session with key '%s' not found", oldKey)
	}

	// Check if new key is valid
	validKey := false
	for _, k := range m.tmuxConfig.AvailableKeys {
		if k == newKey {
			validKey = true
			break
		}
	}
	if !validKey {
		return fmt.Errorf("'%s' is not a valid key", newKey)
	}

	// Check if new key is already in use
	if _, exists := m.sessions[newKey]; exists {
		return fmt.Errorf("key '%s' is already in use", newKey)
	}

	// Update the session key
	m.sessions[newKey] = session
	delete(m.sessions, oldKey)

	return m.Save()
}

// SetTmuxConfig sets the tmux configuration (used by first-run setup)
func (m *Manager) SetTmuxConfig(cfg *TmuxConfig) {
	m.tmuxConfig = cfg
}

// GetConfigPath returns the path to the primary config file
func (m *Manager) GetConfigPath() string {
	return m.configPath
}

// GetNavConfigPath returns the path to the file containing nav config (may be a fragment file like keys.toml)
func (m *Manager) GetNavConfigPath() string {
	return m.navConfigPath
}

// GetResolvedFeatures evaluates the mode preset and applies any granular feature overrides.
// Mode presets:
//   - "bare": Pure tmux-sessionizer replacement. No groups, ecosystems, integrations, or worktrees.
//   - "advanced": Adds multi-group key management and worktree support, but no ecosystems/integrations.
//   - "grove" (default): All features enabled.
//
// Granular overrides in [nav.features] take precedence over the mode preset.
func (m *Manager) GetResolvedFeatures() ResolvedFeatures {
	mode := "grove"
	if m.tmuxConfig != nil && m.tmuxConfig.Mode != "" {
		mode = m.tmuxConfig.Mode
	}

	// Start with mode baseline
	var feat ResolvedFeatures
	switch mode {
	case "bare":
		feat = ResolvedFeatures{Groups: false, Ecosystems: false, Integrations: false, Worktrees: false}
	case "advanced":
		feat = ResolvedFeatures{Groups: true, Ecosystems: false, Integrations: false, Worktrees: true}
	case "grove":
		fallthrough
	default:
		feat = ResolvedFeatures{Groups: true, Ecosystems: true, Integrations: true, Worktrees: true}
	}

	// Apply granular overrides if provided
	if m.tmuxConfig != nil && m.tmuxConfig.Features != nil {
		if m.tmuxConfig.Features.Groups != nil {
			feat.Groups = *m.tmuxConfig.Features.Groups
		}
		if m.tmuxConfig.Features.Ecosystems != nil {
			feat.Ecosystems = *m.tmuxConfig.Features.Ecosystems
		}
		if m.tmuxConfig.Features.Integrations != nil {
			feat.Integrations = *m.tmuxConfig.Features.Integrations
		}
		if m.tmuxConfig.Features.Worktrees != nil {
			feat.Worktrees = *m.tmuxConfig.Features.Worktrees
		}
	}

	return feat
}
