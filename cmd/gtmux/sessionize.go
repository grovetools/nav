package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

var sessionizeCmd = &cobra.Command{
	Use:     "sessionize",
	Aliases: []string{"sz"},
	Short:   "Quickly create or switch to tmux sessions from project directories",
	Long:    `Discover projects from configured search paths and quickly create or switch to tmux sessions. Similar to tmux-sessionizer but with a beautiful interface.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If a path is provided as argument, use it directly
		if len(args) > 0 {
			// Record access for direct path usage too
			mgr := tmux.NewManager(configDir, sessionsFile)
			mgr.RecordProjectAccess(args[0])
			return sessionizeProject(args[0])
		}

		// Otherwise, show the interactive project picker
		mgr := tmux.NewManager(configDir, sessionsFile)
		projects, err := mgr.GetAvailableProjectsSorted()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found in search paths!")
			fmt.Println("\nMake sure your search paths are configured in one of:")
			fmt.Println("  ~/.config/tmux/project-search-paths.yaml")
			fmt.Println("  ~/.config/grove/project-search-paths.yaml")
			return nil
		}

		// Get search paths for display
		searchPaths, err := mgr.GetEnabledSearchPaths()
		if err != nil {
			// Don't fail if we can't get search paths, just continue without them
			searchPaths = []string{}
		}

		// Create the interactive model
		m := newSessionizeModel(projects, searchPaths, mgr)
		
		// Run the interactive program
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		// Check if a project was selected
		if sm, ok := finalModel.(sessionizeModel); ok && sm.selected.Path != "" {
			// Record the access before switching
			mgr.RecordProjectAccess(sm.selected.Path)
			// If it's a worktree, also record access for the parent
			if sm.selected.IsWorktree && sm.selected.ParentPath != "" {
				mgr.RecordProjectAccess(sm.selected.ParentPath)
			}
			return sessionizeProject(sm.selected.Path)
		}

		return nil
	},
}

// sessionizeModel is the model for the interactive project picker
type sessionizeModel struct {
	projects     []manager.DiscoveredProject
	filtered     []manager.DiscoveredProject
	selected     manager.DiscoveredProject
	cursor       int
	filterInput  textinput.Model
	searchPaths  []string
	manager      *tmux.Manager
	keyMap       map[string]string // path -> key mapping
	sessionMap   map[string]bool   // path -> session exists mapping
	currentSession string          // name of the current tmux session
	width        int
	height       int
	// Key editing mode
	editingKeys  bool
	keyCursor    int
	availableKeys []string
	sessions     []models.TmuxSession
}

func newSessionizeModel(projects []manager.DiscoveredProject, searchPaths []string, mgr *tmux.Manager) sessionizeModel {
	// Create text input for filtering
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
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

	// Create session existence map
	sessionMap := make(map[string]bool)
	// We'll populate this lazily as needed to avoid too many tmux calls at startup

	// Get current session name if we're in tmux
	currentSession := ""
	if os.Getenv("TMUX") != "" {
		client, err := tmux.NewClient()
		if err == nil {
			ctx := context.Background()
			if current, err := client.GetCurrentSession(ctx); err == nil {
				currentSession = current
			}
		}
	}

	return sessionizeModel{
		projects:      projects,
		filtered:      projects,
		filterInput:   ti,
		searchPaths:   searchPaths,
		manager:       mgr,
		keyMap:        keyMap,
		sessionMap:    sessionMap,
		currentSession: currentSession,
		cursor:        0,
		editingKeys:   false,
		keyCursor:     0,
		availableKeys: availableKeys,
		sessions:      sessions,
	}
}

func (m sessionizeModel) Init() tea.Cmd {
	return textinput.Blink
}

// getSessionExists checks if a tmux session exists for the given project path
func (m *sessionizeModel) getSessionExists(projectPath string) bool {
	// Check cache first
	if exists, found := m.sessionMap[projectPath]; found {
		return exists
	}

	// Create session name from path
	sessionName := filepath.Base(projectPath)
	sessionName = strings.ReplaceAll(sessionName, ".", "_")

	// Check if session exists using tmux client
	client, err := tmux.NewClient()
	if err != nil {
		// If we can't create client, assume no session
		m.sessionMap[projectPath] = false
		return false
	}

	ctx := context.Background()
	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		// On error, assume no session
		m.sessionMap[projectPath] = false
		return false
	}

	// Cache the result
	m.sessionMap[projectPath] = exists
	return exists
}

func (m sessionizeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
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
			}
			return m, nil
		}
		
		// Normal mode
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown, tea.KeyCtrlN:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case tea.KeyCtrlE:
			// Enter key editing mode
			if m.cursor < len(m.filtered) {
				m.editingKeys = true
				m.keyCursor = 0
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
					cmd.Run()
				}
			}
		case tea.KeyCtrlD:
			// Close the selected session
			if m.cursor < len(m.filtered) {
				project := m.filtered[m.cursor]
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")
				
				// Check if session exists before trying to close it
				client, err := tmux.NewClient()
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
									client.SwitchClient(ctx, targetSession)
								}
							}
						}
						
						// Kill the session
						if err := client.KillSession(ctx, sessionName); err == nil {
							// Clear the cached session status
							delete(m.sessionMap, project.Path)
						}
					}
				}
			}
		case tea.KeyEnter:
			if m.cursor < len(m.filtered) {
				m.selected = m.filtered[m.cursor]
				return m, tea.Quit
			}
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		default:
			// Update filter input
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

	return m, nil
}

func (m *sessionizeModel) updateKeyMapping(projectPath, newKey string) {
	// Find if there's already a session with this key
	var existingSessionIndex = -1
	var targetSessionIndex = -1
	
	cleanPath := filepath.Clean(projectPath)
	
	for i, s := range m.sessions {
		if s.Key == newKey && s.Path != "" {
			existingSessionIndex = i
		}
		// Check for existing mapping of this project
		if s.Path != "" {
			expandedPath := expandPath(s.Path)
			absPath, _ := filepath.Abs(expandedPath)
			if strings.EqualFold(filepath.Clean(absPath), cleanPath) {
				targetSessionIndex = i
			}
		}
	}
	
	// If the key is already used, swap or clear it
	if existingSessionIndex >= 0 && existingSessionIndex != targetSessionIndex {
		// Clear the existing mapping
		m.sessions[existingSessionIndex].Path = ""
		m.sessions[existingSessionIndex].Repository = ""
	}
	
	// Update or create the session
	if targetSessionIndex >= 0 {
		// Update existing session with new key
		oldKey := m.sessions[targetSessionIndex].Key
		m.sessions[targetSessionIndex].Key = newKey
		
		// If we're swapping, give the old key to the other session
		if existingSessionIndex >= 0 && oldKey != "" {
			m.sessions[existingSessionIndex].Key = oldKey
		}
	} else {
		// Create new session
		newSession := models.TmuxSession{
			Key:        newKey,
			Path:       projectPath,
			Repository: filepath.Base(projectPath),
		}
		
		// Find the session with this key and update it
		updated := false
		for i, s := range m.sessions {
			if s.Key == newKey {
				m.sessions[i] = newSession
				updated = true
				break
			}
		}
		
		if !updated {
			m.sessions = append(m.sessions, newSession)
		}
	}
	
	// Save the updated sessions
	m.manager.UpdateSessions(m.sessions)
	m.manager.RegenerateBindings()
	
	// Update our key map
	m.keyMap[cleanPath] = newKey
	
	// Reload tmux config
	reloadTmuxConfig()
}

func (m *sessionizeModel) updateFiltered() {
	filter := strings.ToLower(m.filterInput.Value())
	m.filtered = []manager.DiscoveredProject{}
	
	if filter == "" {
		// No filter, return all projects
		m.filtered = m.projects
		return
	}
	
	// Separate matches into priority groups
	var exactNameMatches []manager.DiscoveredProject   // Exact match on project name
	var prefixNameMatches []manager.DiscoveredProject  // Prefix match on project name
	var containsNameMatches []manager.DiscoveredProject // Contains in project name
	var pathMatches []manager.DiscoveredProject        // Matches elsewhere in the path
	
	for _, p := range m.projects {
		lowerName := strings.ToLower(p.Name)
		lowerPath := strings.ToLower(p.Path)
		
		if lowerName == filter {
			// Exact match on name - highest priority
			exactNameMatches = append(exactNameMatches, p)
		} else if strings.HasPrefix(lowerName, filter) {
			// Prefix match on name - high priority
			prefixNameMatches = append(prefixNameMatches, p)
		} else if strings.Contains(lowerName, filter) {
			// Contains in name - medium priority
			containsNameMatches = append(containsNameMatches, p)
		} else if strings.Contains(lowerPath, filter) {
			// Match elsewhere in the path - lower priority
			pathMatches = append(pathMatches, p)
		}
	}
	
	// Combine results in priority order
	m.filtered = []manager.DiscoveredProject{}
	m.filtered = append(m.filtered, exactNameMatches...)
	m.filtered = append(m.filtered, prefixNameMatches...)
	m.filtered = append(m.filtered, containsNameMatches...)
	m.filtered = append(m.filtered, pathMatches...)
}

func (m sessionizeModel) View() string {
	var b strings.Builder

	// Show key editing mode if active
	if m.editingKeys {
		return m.viewKeyEditor()
	}

	// Header with filter input (always at top)
	b.WriteString(m.filterInput.View())
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
		} else if m.cursor >= len(m.filtered) - visibleHeight/2 {
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
		sessionExists := m.getSessionExists(project.Path)
		
		// Prepare prefix for worktrees
		prefix := ""
		displayName := project.Name
		if project.IsWorktree {
			prefix = "  └─ "
		}
		
		if i == m.cursor {
			// Highlight selected line
			indicator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true).
				Render("▶ ")
			
			nameStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true)
			pathStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#95e1d3"))
			
			keyIndicator := "  " // Default: 2 spaces
			if keyMapping != "" {
				keyIndicator = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffaa00")).
					Bold(true).
					Render(fmt.Sprintf("%s ", keyMapping))
			}
			
			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")
				
				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00aaff")).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00ff00")).
						Render("●")
				}
			}
			
			b.WriteString(fmt.Sprintf("%s%s%s %s%s  %s", 
				indicator,
				keyIndicator,
				sessionIndicator,
				prefix,
				nameStyle.Render(fmt.Sprintf("%-26s", displayName)),
				pathStyle.Render(project.Path)))
		} else {
			// Normal line with colored name
			nameStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4ecdc4")).
				Bold(true)
			pathStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#808080"))
			
			// Always reserve space for key indicator
			keyIndicator := "  "
			if keyMapping != "" {
				keyIndicator = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffaa00")).
					Render(fmt.Sprintf("%s ", keyMapping))
			}
			
			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				sessionName := filepath.Base(project.Path)
				sessionName = strings.ReplaceAll(sessionName, ".", "_")
				
				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00aaff")).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#00ff00")).
						Render("●")
				}
			}
			
			b.WriteString(fmt.Sprintf("  %s%s %s%s  %s", 
				keyIndicator,
				sessionIndicator,
				prefix,
				nameStyle.Render(fmt.Sprintf("%-26s", displayName)),
				pathStyle.Render(project.Path)))
		}
		b.WriteString("\n")
	}

	// Show scroll indicators if needed
	if start > 0 || end < len(m.filtered) {
		scrollInfo := fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.filtered))
		b.WriteString(dimStyle.Render(scrollInfo))
	}
	
	// Help text at bottom
	if len(m.filtered) == 0 {
		b.WriteString("\n" + dimStyle.Render("No matching projects"))
	}
	
	b.WriteString("\n" + helpStyle.Render("↑/↓: navigate • enter: select • ctrl+e: edit key • ctrl+y: copy path • ctrl+d: close • esc: quit"))
	
	// Display search paths at the very bottom
	if len(m.searchPaths) > 0 {
		b.WriteString("\n" + dimStyle.Render("Search paths: "))
		// Truncate search paths if too long
		pathsDisplay := strings.Join(m.searchPaths, " • ")
		if len(pathsDisplay) > m.width-15 && m.width > 50 {
			pathsDisplay = pathsDisplay[:m.width-18] + "..."
		}
		b.WriteString(dimStyle.Render(pathsDisplay))
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
	
	b.WriteString(titleStyle.Render(fmt.Sprintf("Select key for: %s", selectedProject)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(selectedPath))
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
		} else if m.keyCursor >= len(displays) - visibleHeight/2 {
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
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00ff00")).
				Bold(true).
				Render("▶ "))
		} else {
			b.WriteString("  ")
		}
		
		// Key
		keyStyle := lipgloss.NewStyle().Bold(true)
		if d.isCurrent {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#ffaa00"))
		} else if d.repository != "" {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#808080"))
		} else {
			keyStyle = keyStyle.Foreground(lipgloss.Color("#00ff00"))
		}
		b.WriteString(keyStyle.Render(fmt.Sprintf("%s ", d.key)))
		
		// Repository and path
		if d.repository != "" {
			repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ecdc4"))
			pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
			
			b.WriteString(repoStyle.Render(fmt.Sprintf("%-20s", d.repository)))
			b.WriteString(" ")
			b.WriteString(pathStyle.Render(d.path))
		} else {
			b.WriteString(dimStyle.Render("(available)"))
		}
		
		// Mark current
		if d.isCurrent {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffaa00")).
				Render(" ← current"))
		}
		
		b.WriteString("\n")
	}
	
	// Scroll indicator
	if start > 0 || end < len(displays) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n(%d-%d of %d)", start+1, end, len(displays))))
	}
	
	b.WriteString("\n" + helpStyle.Render("↑/↓: navigate • enter: assign key • esc: cancel"))
	
	return b.String()
}

// sessionizeProject creates or switches to a tmux session for the given project path
func sessionizeProject(projectPath string) error {
	// Expand path if needed
	if strings.HasPrefix(projectPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			projectPath = filepath.Join(home, projectPath[2:])
		}
	}

	// Get absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if directory exists
	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		return fmt.Errorf("directory does not exist: %s", absPath)
	}

	// Create session name from path
	sessionName := filepath.Base(absPath)
	sessionName = strings.ReplaceAll(sessionName, ".", "_")

	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		// Not in tmux, create new session
		cmd := exec.Command("tmux", "new-session", "-s", sessionName, "-c", absPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// We're in tmux, use the tmux client
	client, err := tmux.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create tmux client: %w", err)
	}

	ctx := context.Background()

	// Check if session exists
	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("failed to check session: %w", err)
	}

	if !exists {
		// Create new session
		opts := tmux.LaunchOptions{
			SessionName:      sessionName,
			WorkingDirectory: absPath,
		}
		if err := client.Launch(ctx, opts); err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Switch to the session
	if err := client.SwitchClient(ctx, sessionName); err != nil {
		return fmt.Errorf("failed to switch to session: %w", err)
	}

	return nil
}

