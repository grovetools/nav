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
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
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
	table    table.Model
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
	// Build table columns
	columns := []table.Column{
		{Title: "Key", Width: 5},
		{Title: "Repository", Width: 25},
		{Title: "Path", Width: 60},
	}

	// Build table rows
	var rows []table.Row
	for _, s := range sessions {
		repo := ""
		path := ""

		if s.Path != "" {
			repo = filepath.Base(s.Path)
			path = s.Path
		}

		rows = append(rows, table.Row{
			s.Key,
			repo,
			path,
		})
	}

	// Create table
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(rows)),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(core_theme.DefaultColors.Border).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(core_theme.DefaultColors.LightText).
		Background(core_theme.DefaultColors.SelectedBackground).
		Bold(false)
	t.SetStyles(s)

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
		table:           t,
		sessions:        sessions,
		manager:         mgr,
		keys:            keys,
		help:            helpModel,
		searchInput:     ti,
		projectList:     l,
		projects:        []manager.DiscoveredProject{},
		projectsLoading: true,
	}
}

func (m manageModel) Init() tea.Cmd {
	// Fetch projects without enrichment for speed.
	return fetchProjectsCmd(m.manager, false, false)
}

func (m manageModel) rebuildTable() table.Model {
	// Build table rows from current sessions
	var rows []table.Row
	for _, s := range m.sessions {
		repo := ""
		path := ""

		if s.Path != "" {
			repo = filepath.Base(s.Path)
			path = s.Path
		}

		rows = append(rows, table.Row{
			s.Key,
			repo,
			path,
		})
	}

	// Update the table rows
	newTable := m.table
	newTable.SetRows(rows)
	return newTable
}

func (m manageModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
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
					cursor := m.table.Cursor()
					if cursor < len(m.sessions) {
						m.sessions[cursor].Path = i.path
						m.sessions[cursor].Repository = ""
						m.sessions[cursor].Description = ""
						m.table = m.rebuildTable()
						m.table.SetCursor(cursor)
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

		for _, p := range m.projects {
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
			cursor := m.table.Cursor()
			if cursor < len(m.sessions) {
				m.selectedKey = m.sessions[cursor].Key
				m.searching = true
				m.searchInput.Focus()

				// Initialize project list
				items := make([]list.Item, 0)
				existingPaths := make(map[string]bool)
				for _, s := range m.sessions {
					if s.Path != "" {
						absPath, _ := filepath.Abs(expandPath(s.Path))
						existingPaths[absPath] = true
					}
				}

				for _, p := range m.projects {
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
			cursor := m.table.Cursor()
			if cursor < len(m.sessions) {
				session := m.sessions[cursor]
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
			cursor := m.table.Cursor()
			if cursor < len(m.sessions) {
				session := &m.sessions[cursor]
				if session.Path != "" {
					// Clear the session
					session.Path = ""
					session.Repository = ""
					session.Description = ""
					m.table = m.rebuildTable()
					m.table.SetCursor(cursor)
					m.message = fmt.Sprintf("Unmapped key %s", session.Key)
				} else {
					m.message = "Press 'e' or Enter to map this key"
				}
			}

		case key.Matches(msg, m.keys.Delete):
			// Clear the mapping for selected session
			cursor := m.table.Cursor()
			if cursor < len(m.sessions) {
				session := &m.sessions[cursor]
				if session.Path != "" {
					// Clear the session
					session.Path = ""
					session.Repository = ""
					session.Description = ""

					// Rebuild table with updated data
					m.table = m.rebuildTable()
					m.table.SetCursor(cursor)

					m.message = fmt.Sprintf("Unmapped key %s", session.Key)
				}
			}

		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
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
	b.WriteString(m.table.View() + "\n\n")

	if m.message != "" {
		b.WriteString(dimStyle.Render(m.message) + "\n\n")
	}

	b.WriteString(helpStyle.Render("Press ? for help"))

	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
