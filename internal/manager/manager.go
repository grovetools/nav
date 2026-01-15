package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	core_config "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type Manager struct {
	configDir    string
	coreConfig   *core_config.Config
	tmuxConfig   *TmuxConfig
	sessions     map[string]TmuxSessionConfig
	lockedKeys   []string
	configPath   string
	sessionsPath string
	tmuxClient   *tmux.Client
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

// Legacy types removed as discovery is now handled by grove-core's DiscoveryService.
// SearchPathConfig, ExplicitProject, and ProjectSearchConfig are no longer needed.

// NewManager creates a new Manager instance
func NewManager(configDir string) (*Manager, error) {
	// Expand paths
	configDir = expandPath(configDir)

	// Load the layered grove.yml configuration
	coreCfg, err := core_config.LoadFrom(configDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load grove config: %w", err)
	}
	// If config doesn't exist, proceed with a default config object
	if coreCfg == nil {
		coreCfg = &core_config.Config{}
	}

	// Unmarshal the 'tmux' extension (static config only)
	var tmuxCfg TmuxConfig
	if err := coreCfg.UnmarshalExtension("tmux", &tmuxCfg); err != nil {
		return nil, fmt.Errorf("failed to parse 'tmux' config section: %w", err)
	}

	// Find the primary config file path for saving
	configPath, err := core_config.FindConfigFile(configDir)
	if err != nil {
		// If no file exists, default to the standard global path
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "grove", "grove.yml")
	}

	// Load sessions from separate file in gmux directory
	sessionsPath := filepath.Join(configDir, "gmux", "sessions.yml")
	sessions := make(map[string]TmuxSessionConfig)
	var lockedKeys []string

	if data, err := os.ReadFile(sessionsPath); err == nil {
		var sessionsFile TmuxSessionsFile
		if err := yaml.Unmarshal(data, &sessionsFile); err == nil {
			// Ensure sessions map is not nil before assigning
			if sessionsFile.Sessions != nil {
				sessions = sessionsFile.Sessions
			}
			lockedKeys = sessionsFile.LockedKeys
		}
	}
	// If file doesn't exist or is empty, sessions will be an empty map

	// Initialize tmux client
	tmuxClient, clientErr := tmux.NewClient()
	if clientErr != nil {
		// Log warning but don't fail - some operations may still work
		fmt.Fprintf(os.Stderr, "Warning: could not initialize tmux client: %v\n", clientErr)
	}

	return &Manager{
		configDir:    configDir,
		coreConfig:   coreCfg,
		tmuxConfig:   &tmuxCfg,
		sessions:     sessions,
		lockedKeys:   lockedKeys,
		configPath:   configPath,
		sessionsPath: sessionsPath,
		tmuxClient:   tmuxClient,
	}, nil
}

func (m *Manager) GetLockedKeys() []string {
	return m.lockedKeys
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
				Key:         key,
				Path:        expandPath(sessionData.Path),
				Repository:  sessionData.Repository,
				Description: sessionData.Description,
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
// - Static config (search paths, discovery) to grove.yml
// - Dynamic state (session mappings) to gmux/sessions.yml
func (m *Manager) Save() error {
	// Save static config to grove.yml
	if err := m.saveStaticConfig(); err != nil {
		return err
	}

	// Save sessions to separate file
	return m.saveSessions()
}

// saveStaticConfig saves the static tmux configuration to grove.yml
func (m *Manager) saveStaticConfig() error {
	// Ensure the config directory exists
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Read the existing file into a generic map to preserve other contents
	var fullConfig map[string]interface{}
	data, err := os.ReadFile(m.configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing config file: %w", err)
	}
	// If file exists, unmarshal it
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &fullConfig); err != nil {
			return fmt.Errorf("failed to parse existing config file: %w", err)
		}
	} else {
		fullConfig = make(map[string]interface{})
	}

	// Update the 'tmux' section (static config only, no sessions)
	fullConfig["tmux"] = m.tmuxConfig

	// Marshal the full config back to YAML
	newData, err := yaml.Marshal(fullConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	return os.WriteFile(m.configPath, newData, 0o644)
}

// saveSessions saves the session mappings to gmux/sessions.yml
func (m *Manager) saveSessions() error {
	// Ensure the gmux directory exists
	gmuxDir := filepath.Dir(m.sessionsPath)
	if err := os.MkdirAll(gmuxDir, 0o755); err != nil {
		return fmt.Errorf("failed to create gmux directory: %w", err)
	}

	// Create the sessions file structure
	sessionsFile := TmuxSessionsFile{
		Sessions:   m.sessions,
		LockedKeys: m.lockedKeys,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(sessionsFile)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	return os.WriteFile(m.sessionsPath, data, 0o644)
}

func (m *Manager) UpdateSessions(sessions []models.TmuxSession) error {
	return m.UpdateSessionsAndLocks(sessions, m.lockedKeys)
}

func (m *Manager) UpdateSessionsAndLocks(sessions []models.TmuxSession, lockedKeys []string) error {
	// Convert slice back to map format, only including non-empty sessions
	sessionsMap := make(map[string]TmuxSessionConfig)

	for _, session := range sessions {
		// Only save sessions that have actual data
		if session.Path != "" || session.Repository != "" {
			sessionsMap[session.Key] = TmuxSessionConfig{
				Path:        session.Path,
				Repository:  session.Repository,
				Description: session.Description,
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
		Path:        session.Path,
		Repository:  session.Repository,
		Description: session.Description,
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

// isGitRepository checks if a directory contains a .git folder or file
func isGitRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

// GetAvailableProjects now uses the DiscoveryService from grove-core to find all projects.
// Enrichment is no longer handled here; it is done asynchronously in the TUI.
func (m *Manager) GetAvailableProjects() ([]DiscoveredProject, error) {
	// Initialize the DiscoveryService
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.WarnLevel)

	// Step 1: Run discovery and transform to WorkspaceNodes
	// GetProjects is the new, consolidated function in grove-core.
	workspaceNodes, err := workspace.GetProjects(logger)
	if err != nil {
		// Return an empty list if discovery fails - sessionize will handle the empty case
		// This allows first-run setup to trigger
		return []DiscoveredProject{}, fmt.Errorf("failed to run discovery service: %w", err)
	}

	// Step 2: Transform []*workspace.WorkspaceNode into []DiscoveredProject (SessionizeProject)
	projects := make([]DiscoveredProject, len(workspaceNodes))
	for i, node := range workspaceNodes {
		projects[i] = SessionizeProject{
			WorkspaceNode: node,
			// Enrichment fields are initialized to nil and populated later.
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
		if session.Path == "" || session.Repository == "" {
			continue
		}

		status := m.GetGitStatus(session.Path, session.Repository)
		statuses[session.Repository] = status
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
		HasChanges: status != "✓",
		IsClean:    status == "✓",
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
		return "✓"
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
	sessionName := projInfo.Identifier()

	ctx := context.Background()

	// Check if tmux is running
	tmuxRunning := m.isTmuxRunning()
	inTmux := os.Getenv("TMUX") != ""

	// If tmux is not running and we're not in tmux, start new session
	if !tmuxRunning && !inTmux {
		// Need to use exec.Command directly for interactive session
		cmd := exec.Command("tmux", "new-session", "-s", sessionName, "-c", expandedPath)
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
	cmd := exec.Command("tmux", "attach-session", "-t", sessionName)
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

func (m *Manager) hasSession(name string) bool {
	if m.tmuxClient == nil {
		return false
	}
	exists, err := m.tmuxClient.SessionExists(context.Background(), name)
	return err == nil && exists
}

// RegenerateBindingsGo generates tmux key bindings in Go (replacing Python script)
func (m *Manager) RegenerateBindingsGo() error {
	sessions, err := m.GetSessions()
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	var bindings strings.Builder
	bindings.WriteString("# Auto-generated tmux key bindings from grove.yml\n")
	bindings.WriteString("# Generated by grove-tmux\n\n")

	// Use gmux sessionize as the command path
	sessionizerPath := "gmux sessionize"

	// Sort sessions by key for consistent output
	sortedSessions := make([]models.TmuxSession, len(sessions))
	copy(sortedSessions, sessions)
	// Sort by key
	for i := 0; i < len(sortedSessions)-1; i++ {
		for j := i + 1; j < len(sortedSessions); j++ {
			if sortedSessions[i].Key > sortedSessions[j].Key {
				sortedSessions[i], sortedSessions[j] = sortedSessions[j], sortedSessions[i]
			}
		}
	}

	for _, session := range sortedSessions {
		if session.Path == "" {
			continue
		}

		comment := fmt.Sprintf("# %s: %s", session.Key, session.Repository)
		if session.Description != "" {
			comment = fmt.Sprintf("# %s: %s - %s", session.Key, session.Repository, session.Description)
		}

		bindings.WriteString(comment + "\n")
		// Use quotes around the path to handle spaces
		bindings.WriteString(fmt.Sprintf("bind-key -r %s run-shell \"%s '%s'\"\n\n",
			session.Key, sessionizerPath, session.Path))
	}

	bindingsFile := filepath.Join(m.configDir, "gmux", "generated-bindings.conf")
	return os.WriteFile(bindingsFile, []byte(bindings.String()), 0o644)
}

// DetectTmuxKeyForPath detects the tmux session key for a given working directory
func (m *Manager) DetectTmuxKeyForPath(workingDir string) string {
	// Check if we're in a tmux session
	if tmuxEnv := os.Getenv("TMUX"); tmuxEnv == "" {
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

// GetConfigPath returns the path to the config file
func (m *Manager) GetConfigPath() string {
	return m.configPath
}
