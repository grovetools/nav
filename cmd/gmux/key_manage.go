package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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

// projectItem implements list.Item
type projectItem struct {
	path string
	name string
}

func (i projectItem) Title() string       { return i.name }
func (i projectItem) Description() string { return i.path }
func (i projectItem) FilterValue() string { return i.name + " " + i.path }

// Message for enriched data of mapped projects
type enrichedProjectsMsg struct {
	projects []*workspace.ProjectInfo
}

// Message for all discoverable projects for the fuzzy finder
type allProjectsMsg struct {
	projects []manager.DiscoveredProject
}

var keyManageCmd = &cobra.Command{
	Use:     "manage",
	Aliases: []string{"m"},
	Short:   "Interactively manage tmux session key mappings",
	Long:    `Open an interactive table to map/unmap sessions to keys. Use arrow keys to navigate, e/enter to map with fuzzy search, and space to unmap. Changes are auto-saved on exit.`,
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

		// Create the interactive model
		m := newManageModel(sessions, mgr)

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
	// Fuzzy search state
	searching       bool
	searchInput     textinput.Model
	projectList     list.Model
	projects        []manager.DiscoveredProject
	filteredProj    []manager.DiscoveredProject
	selectedKey     string
	projectsLoading bool
	// Enriched data
	enrichedProjects map[string]*workspace.ProjectInfo // Caches enriched data by path
	allProjects      []manager.DiscoveredProject       // Caches all discoverable projects
}

// Key bindings
type keyMap struct {
	keymap.Base
	Up     key.Binding
	Down   key.Binding
	Toggle key.Binding
	Edit   key.Binding
	Open   key.Binding
	Delete key.Binding
	Save   key.Binding
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
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "Switch to session")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Edit,
			k.Toggle,
			k.Delete,
			k.Save,
			k.Help,
			k.Quit,
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
		key.WithHelp("e", "edit/map with fuzzy search"),
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
}

func newManageModel(sessions []models.TmuxSession, mgr *tmux.Manager) manageModel {
	// Initialize search input
	ti := textinput.New()
	ti.Placeholder = "Type to filter projects..."
	ti.CharLimit = 100
	ti.Width = 50

	// Create list for projects with custom delegate
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(0)

	items := make([]list.Item, 0)
	l := list.New(items, delegate, 60, 15)
	l.Title = "Select a project"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // We'll do our own filtering
	l.Styles.Title = titleStyle

	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Session Key Manager - Help").
		Build()

	return manageModel{
		cursor:           0,
		sessions:         sessions,
		manager:          mgr,
		keys:             keys,
		help:             helpModel,
		searchInput:      ti,
		projectList:      l,
		projects:         []manager.DiscoveredProject{},
		projectsLoading:  true,
		enrichedProjects: make(map[string]*workspace.ProjectInfo),
		allProjects:      []manager.DiscoveredProject{},
	}
}

func (m manageModel) Init() tea.Cmd {
	return tea.Batch(
		enrichMappedProjectsCmd(m.sessions),
		fetchAllProjectsCmd(m.manager),
	)
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

// fetchAllProjectsCmd fetches all discoverable projects without enrichment
func fetchAllProjectsCmd(mgr *tmux.Manager) tea.Cmd {
	return func() tea.Msg {
		enrichOpts := buildEnrichmentOptions(false, false)
		projects, _ := mgr.GetAvailableProjectsWithOptions(enrichOpts)

		// Sort by access history
		if history, err := mgr.GetAccessHistory(); err == nil {
			projects = history.SortProjectsByAccess(projects)
		}

		return allProjectsMsg{projects: projects}
	}
}

func (m manageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle search mode
	if m.searching {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEsc, tea.KeyCtrlC:
				m.searching = false
				m.searchInput.SetValue("")
				return m, nil
			case tea.KeyEnter:
				// Select the project
				if i, ok := m.projectList.SelectedItem().(projectItem); ok {
					// Update the session with the selected project
					if m.cursor < len(m.sessions) {
						m.sessions[m.cursor].Path = i.path
						m.sessions[m.cursor].Repository = ""
						m.sessions[m.cursor].Description = ""
						m.message = fmt.Sprintf("Mapped %s to %s", m.selectedKey, i.name)
					}
				}
				m.searching = false
				m.searchInput.SetValue("")
				return m, nil
			}
		}

		// Update search input
		var inputCmd tea.Cmd
		m.searchInput, inputCmd = m.searchInput.Update(msg)
		cmds = append(cmds, inputCmd)

		// Filter projects based on search
		searchTerm := strings.ToLower(m.searchInput.Value())
		items := make([]list.Item, 0)

		// Get existing paths to exclude
		existingPaths := make(map[string]bool)
		for _, s := range m.sessions {
			if s.Path != "" {
				absPath, _ := filepath.Abs(expandPath(s.Path))
				existingPaths[absPath] = true
			}
		}

		for _, p := range m.allProjects {
			absPath, _ := filepath.Abs(expandPath(p.Path))
			if existingPaths[absPath] {
				continue // Skip already mapped projects
			}

			if searchTerm == "" ||
				strings.Contains(strings.ToLower(p.Name), searchTerm) ||
				strings.Contains(strings.ToLower(p.Path), searchTerm) {
				items = append(items, projectItem{path: p.Path, name: p.Name})
			}
		}

		m.projectList.SetItems(items)

		// Update list size and items
		m.projectList.SetWidth(min(80, m.help.Width-4))
		m.projectList.SetHeight(min(20, len(items)+2))

		var listCmd tea.Cmd
		m.projectList, listCmd = m.projectList.Update(msg)
		cmds = append(cmds, listCmd)

		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case enrichedProjectsMsg:
		// Populate enriched projects map
		for _, proj := range msg.projects {
			cleanPath := filepath.Clean(proj.Path)
			m.enrichedProjects[cleanPath] = proj
		}
		return m, nil

	case allProjectsMsg:
		m.allProjects = msg.projects
		m.projectsLoading = false
		return m, nil

	case projectsUpdateMsg:
		m.projects = msg.projects
		m.projectsLoading = false
		return m, nil

	case tea.WindowSizeMsg:
		m.help.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		// If help is visible, it consumes all key presses
		if m.help.ShowAll {
			m.help.Toggle() // Any key closes help
			return m, nil
		}

		switch {
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

		case key.Matches(msg, m.keys.Edit):
			if m.projectsLoading {
				m.message = "Projects are still loading, please wait..."
				return m, nil
			}
			// Enter fuzzy search mode
			if m.cursor < len(m.sessions) {
				m.selectedKey = m.sessions[m.cursor].Key
				m.searching = true
				m.searchInput.Focus()

				// Initialize project list using pre-fetched allProjects
				items := make([]list.Item, 0)
				existingPaths := make(map[string]bool)
				for _, s := range m.sessions {
					if s.Path != "" {
						absPath, _ := filepath.Abs(expandPath(s.Path))
						existingPaths[absPath] = true
					}
				}

				for _, p := range m.allProjects {
					absPath, _ := filepath.Abs(expandPath(p.Path))
					if existingPaths[absPath] {
						continue
					}
					items = append(items, projectItem{path: p.Path, name: p.Name})
				}
				m.projectList.SetItems(items)
				m.projectList.SetWidth(60)
				m.projectList.SetHeight(min(15, len(items)+2))

				return m, textinput.Blink
			}

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
			}

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
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

	// Show fuzzy search interface if searching
	if m.searching {
		b.WriteString(titleStyle.Render(fmt.Sprintf("Select Project for Key '%s'", m.selectedKey)) + "\n\n")
		b.WriteString("Search: " + m.searchInput.View() + "\n\n")

		// Show item count
		itemCount := len(m.projectList.Items())
		if itemCount == 0 {
			b.WriteString(dimStyle.Render("No matching projects found") + "\n")
		} else {
			b.WriteString(fmt.Sprintf("Found %d projects:\n\n", itemCount))
			b.WriteString(m.projectList.View())
		}

		b.WriteString("\n" + dimStyle.Render("↑↓ Navigate • Enter to select • Esc to cancel") + "\n")
		return b.String()
	}

	// Normal table view
	b.WriteString(titleStyle.Render("Manage Session Keys") + "\n\n")

	// Build table data
	headers := []string{"Key", "Ecosystem", "Repository", "Worktree", "Git", "Claude"}
	var rows [][]string

	for _, s := range m.sessions {
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

				// Format Claude status (with colors)
				claudeStatus = formatClaudeStatus(projInfo.ClaudeSession)
			} else {
				// Fallback if no enriched data
				repository = filepath.Base(s.Path)
			}
		}

		rows = append(rows, []string{
			s.Key,
			ecosystem,
			repository,
			worktree,
			gitStatus,
			claudeStatus,
		})
	}

	// Render table with selection and Repository column highlighted (column index 2)
	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{
		HighlightColumn: 2, // Repository column
	})
	b.WriteString(tableStr)
	b.WriteString("\n\n")

	if m.message != "" {
		b.WriteString(dimStyle.Render(m.message) + "\n\n")
	}

	b.WriteString(helpStyle.Render("Press ? for help  •  * = part of ecosystem worktree"))

	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatClaudeStatus formats the Claude session status into a styled string
func formatClaudeStatus(session *workspace.ClaudeSessionInfo) string {
	if session == nil {
		return ""
	}

	statusSymbol := ""
	var statusColor lipgloss.Color
	switch session.Status {
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

	if session.Duration != "" {
		return statusStyled + " " + session.Duration
	}
	return statusStyled
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
