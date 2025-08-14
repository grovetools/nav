package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattsolo1/grove-core/pkg/models"
	"gopkg.in/yaml.v3"
)

type Manager struct {
	configDir       string
	sessionsFile    string
	searchPathsFile string
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

type SearchPathConfig struct {
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
	Enabled     bool   `yaml:"enabled"`
}

type ProjectSearchConfig struct {
	SearchPaths map[string]SearchPathConfig `yaml:"search_paths"`
	Discovery   struct {
		MaxDepth        int      `yaml:"max_depth"`
		MinDepth        int      `yaml:"min_depth"`
		FileTypes       []string `yaml:"file_types"`
		ExcludePatterns []string `yaml:"exclude_patterns"`
	} `yaml:"discovery"`
}

// NewManager creates a new Manager instance
func NewManager(configDir string, sessionsFile string) *Manager {
	// Expand paths
	configDir = expandPath(configDir)

	// Use provided sessions file or default
	if sessionsFile == "" {
		sessionsFile = filepath.Join(configDir, "tmux-sessions.yaml")
	} else if !filepath.IsAbs(sessionsFile) {
		// If relative path, make it relative to configDir
		sessionsFile = filepath.Join(configDir, sessionsFile)
	} else {
		// Expand absolute path too
		sessionsFile = expandPath(sessionsFile)
	}

	// Try multiple locations for search paths file
	searchPathsFile := ""
	possiblePaths := []string{
		filepath.Join(configDir, "project-search-paths.yaml"),
		expandPath("~/.config/tmux/project-search-paths.yaml"),
		expandPath("~/.config/grove/project-search-paths.yaml"),
	}
	
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			searchPathsFile = path
			break
		}
	}
	
	// Default to grove config dir if none found
	if searchPathsFile == "" {
		searchPathsFile = filepath.Join(configDir, "project-search-paths.yaml")
	}

	return &Manager{
		configDir:       configDir,
		sessionsFile:    sessionsFile,
		searchPathsFile: searchPathsFile,
	}
}

func (m *Manager) GetSessions() ([]models.TmuxSession, error) {
	data, err := os.ReadFile(m.sessionsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.TmuxSession{}, nil
		}
		return nil, err
	}

	// Parse the sessions map format with available keys
	var config struct {
		AvailableKeys []string `yaml:"available_keys"`
		Sessions      map[string]struct {
			Path        string `yaml:"path"`
			Repo        string `yaml:"repo"`
			Description string `yaml:"description"`
		} `yaml:"sessions"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Create sessions for all available keys
	sessions := make([]models.TmuxSession, 0, len(config.AvailableKeys))

	// First, add all available keys as sessions (empty if not configured)
	for _, key := range config.AvailableKeys {
		if sessionData, exists := config.Sessions[key]; exists {
			// Key has a configured session
			sessions = append(sessions, models.TmuxSession{
				Key:         key,
				Path:        expandPath(sessionData.Path),
				Repository:  sessionData.Repo,
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

func (m *Manager) UpdateSessions(sessions []models.TmuxSession) error {
	// First, read the current config to preserve available_keys
	currentData, err := os.ReadFile(m.sessionsFile)
	if err != nil {
		return err
	}

	var currentConfig struct {
		AvailableKeys []string `yaml:"available_keys"`
		Sessions      map[string]struct {
			Path        string `yaml:"path"`
			Repo        string `yaml:"repo"`
			Description string `yaml:"description"`
		} `yaml:"sessions"`
	}
	if err := yaml.Unmarshal(currentData, &currentConfig); err != nil {
		return err
	}

	// Convert slice back to map format, only including non-empty sessions
	sessionsMap := make(map[string]struct {
		Path        string `yaml:"path"`
		Repo        string `yaml:"repo"`
		Description string `yaml:"description"`
	})

	for _, session := range sessions {
		// Only save sessions that have actual data
		if session.Path != "" || session.Repository != "" {
			sessionsMap[session.Key] = struct {
				Path        string `yaml:"path"`
				Repo        string `yaml:"repo"`
				Description string `yaml:"description"`
			}{
				Path:        session.Path,
				Repo:        session.Repository,
				Description: session.Description,
			}
		}
	}

	config := struct {
		AvailableKeys []string `yaml:"available_keys"`
		Sessions      map[string]struct {
			Path        string `yaml:"path"`
			Repo        string `yaml:"repo"`
			Description string `yaml:"description"`
		} `yaml:"sessions"`
	}{
		AvailableKeys: currentConfig.AvailableKeys,
		Sessions:      sessionsMap,
	}

	data, err := yaml.Marshal(&config)
	if err != nil {
		return err
	}

	return os.WriteFile(m.sessionsFile, data, 0644)
}

func (m *Manager) UpdateSingleSession(key string, session models.TmuxSession) error {
	// Get all current sessions
	sessions, err := m.GetSessions()
	if err != nil {
		return err
	}

	// Update the specific session
	found := false
	for i, s := range sessions {
		if s.Key == key {
			sessions[i] = session
			sessions[i].Key = key // Ensure key remains the same
			found = true
			break
		}
	}

	// If session doesn't exist, add it
	if !found {
		session.Key = key
		sessions = append(sessions, session)
	}

	// Update all sessions
	return m.UpdateSessions(sessions)
}

func (m *Manager) GetAvailableProjects() ([]string, error) {
	searchConfig, err := m.getSearchPaths()
	if err != nil {
		return nil, err
	}

	projects := []string{}
	seen := make(map[string]bool)

	for _, sp := range searchConfig.SearchPaths {
		if !sp.Enabled {
			continue
		}

		expandedPath := expandPath(sp.Path)
		entries, err := os.ReadDir(expandedPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				fullPath := filepath.Join(expandedPath, entry.Name())
				if !seen[fullPath] {
					projects = append(projects, fullPath)
					seen[fullPath] = true
				}
			}
		}
	}

	return projects, nil
}

func (m *Manager) getSearchPaths() (*ProjectSearchConfig, error) {
	data, err := os.ReadFile(m.searchPathsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &ProjectSearchConfig{SearchPaths: make(map[string]SearchPathConfig)}, nil
		}
		return nil, err
	}

	var config ProjectSearchConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
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
	upstream := m.runGitCommand(path, "for-each-ref", "--format=%(upstream:short)", "@{u}")
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

	// If we're not on main/master and it exists, show ahead/behind
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
	sessionName := filepath.Base(expandedPath)
	// Replace dots with underscores for valid tmux session names
	sessionName = strings.ReplaceAll(sessionName, ".", "_")

	// Check if tmux is running
	tmuxRunning := m.isTmuxRunning()
	inTmux := os.Getenv("TMUX") != ""

	// If tmux is not running and we're not in tmux, start new session
	if !tmuxRunning && !inTmux {
		cmd := exec.Command("tmux", "new-session", "-s", sessionName, "-c", expandedPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Check if session already exists
	if !m.hasSession(sessionName) {
		// Create new detached session
		cmd := exec.Command("tmux", "new-session", "-ds", sessionName, "-c", expandedPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Switch to the session if we're in tmux
	if inTmux {
		cmd := exec.Command("tmux", "switch-client", "-t", sessionName)
		return cmd.Run()
	}

	// Attach to the session if we're outside tmux
	cmd := exec.Command("tmux", "attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *Manager) isTmuxRunning() bool {
	cmd := exec.Command("pgrep", "tmux")
	err := cmd.Run()
	return err == nil
}

func (m *Manager) hasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	err := cmd.Run()
	return err == nil
}

// RegenerateBindingsGo generates tmux key bindings in Go (replacing Python script)
func (m *Manager) RegenerateBindingsGo() error {
	sessions, err := m.GetSessions()
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	var bindings strings.Builder
	bindings.WriteString("# Auto-generated tmux key bindings from tmux-sessions.yaml\n")
	bindings.WriteString("# Generated by tmux-claude-hud\n\n")

	sessionizerPath := "~/.local/bin/scripts/tmux-sessionizer"

	// Check if sessionizer script path is configured
	data, err := os.ReadFile(m.sessionsFile)
	if err == nil {
		var config struct {
			TmuxSessionizer struct {
				ScriptPath string `yaml:"script_path"`
			} `yaml:"tmux_sessionizer"`
		}
		if err := yaml.Unmarshal(data, &config); err == nil && config.TmuxSessionizer.ScriptPath != "" {
			sessionizerPath = config.TmuxSessionizer.ScriptPath
		}
	}

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
		bindings.WriteString(fmt.Sprintf("bind-key -r %s run-shell \"%s %s\"\n\n",
			session.Key, sessionizerPath, session.Path))
	}

	bindingsFile := filepath.Join(m.configDir, "generated-bindings.conf")
	return os.WriteFile(bindingsFile, []byte(bindings.String()), 0644)
}

// DetectTmuxKeyForPath detects the tmux session key for a given working directory
func (m *Manager) DetectTmuxKeyForPath(workingDir string) string {
	// Check if we're in a tmux session
	if tmuxEnv := os.Getenv("TMUX"); tmuxEnv == "" {
		return ""
	}

	// Try to read the tmux sessions config to find matching path
	data, err := os.ReadFile(m.sessionsFile)
	if err != nil {
		return ""
	}

	// Parse YAML to find matching session
	var config struct {
		Sessions map[string]struct {
			Path string `yaml:"path"`
		} `yaml:"sessions"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return ""
	}

	// Normalize working directory
	absWorkingDir, _ := filepath.Abs(workingDir)

	// Find matching session by path
	for key, session := range config.Sessions {
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
	data, err := os.ReadFile(m.sessionsFile)
	if err != nil {
		return []string{}
	}

	var config struct {
		AvailableKeys []string `yaml:"available_keys"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return []string{}
	}

	return config.AvailableKeys
}

// UpdateSessionKey updates the key for a specific session
func (m *Manager) UpdateSessionKey(oldKey, newKey string) error {
	if oldKey == newKey {
		return nil // No change needed
	}

	// Read current configuration
	data, err := os.ReadFile(m.sessionsFile)
	if err != nil {
		return fmt.Errorf("failed to read sessions file: %w", err)
	}

	var config struct {
		AvailableKeys []string `yaml:"available_keys"`
		Sessions      map[string]struct {
			Path        string `yaml:"path"`
			Repo        string `yaml:"repo"`
			Description string `yaml:"description"`
		} `yaml:"sessions"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse sessions file: %w", err)
	}

	// Check if old key exists
	session, exists := config.Sessions[oldKey]
	if !exists {
		return fmt.Errorf("session with key '%s' not found", oldKey)
	}

	// Check if new key is valid
	validKey := false
	for _, k := range config.AvailableKeys {
		if k == newKey {
			validKey = true
			break
		}
	}
	if !validKey {
		return fmt.Errorf("'%s' is not a valid key", newKey)
	}

	// Check if new key is already in use (unless it's the same session)
	if _, exists := config.Sessions[newKey]; exists && newKey != oldKey {
		return fmt.Errorf("key '%s' is already in use", newKey)
	}

	// Update the session key
	if oldKey != newKey {
		config.Sessions[newKey] = session
		delete(config.Sessions, oldKey)
	}

	// Write back to file
	newData, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal sessions: %w", err)
	}

	return os.WriteFile(m.sessionsFile, newData, 0644)
}
