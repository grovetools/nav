package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/spf13/cobra"
)

// resolvePrefixDisplay converts prefix placeholders to actual keys for display
func resolvePrefixDisplay(prefix string) string {
	switch prefix {
	case "<prefix>":
		return "C-b"
	case "<grove>":
		return "C-g"
	case "":
		return "(none)"
	default:
		if strings.HasPrefix(prefix, "<prefix> ") {
			return "C-b " + strings.TrimPrefix(prefix, "<prefix> ")
		}
		if strings.HasPrefix(prefix, "<grove> ") {
			return "C-g " + strings.TrimPrefix(prefix, "<grove> ")
		}
		return prefix
	}
}

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "Interactively manage workspace groups",
	Long:  `Open an interactive table to manage workspace groups. Create, rename, reorder, and delete groups.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNavTUIWithView(viewGroups)
	},
}

// groupsModel is the model for the groups management TUI.
type groupsModel struct {
	manager *tmux.Manager
	groups  []string
	cursor  int
	keys    groupsKeyMap
	help    help.Model

	// Mode state
	moveMode    bool
	inputMode   string          // "new_name", "new_prefix", "rename", "edit_prefix"
	input       textinput.Model
	pendingName string // Holds name between name/prefix prompts for creation

	// Confirmation
	confirmMode bool
	message     string

	// Navigation
	nextCommand string // For handoff back to km

	// Dimensions
	width, height int
}

func newGroupsModel(mgr *tmux.Manager) groupsModel {
	helpModel := help.NewBuilder().
		WithKeys(groupsKeys).
		WithTitle("Group Management - Help").
		Build()

	ti := textinput.New()
	ti.CharLimit = 50
	ti.Width = 40

	return groupsModel{
		manager: mgr,
		groups:  mgr.GetAllGroups(),
		cursor:  0,
		keys:    groupsKeys,
		help:    helpModel,
		input:   ti,
	}
}

func (m *groupsModel) Init() tea.Cmd {
	return nil
}

func (m *groupsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// If help is visible, any key closes it
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
		}

		// Handle confirmation mode
		if m.confirmMode {
			switch {
			case msg.Type == tea.KeyEsc, msg.String() == "n", msg.String() == "N":
				m.confirmMode = false
				m.message = "Cancelled"
				return m, nil

			case msg.String() == "y", msg.String() == "Y":
				// Execute delete
				groupToDelete := m.groups[m.cursor]
				if err := m.manager.DeleteGroup(groupToDelete); err != nil {
					m.message = fmt.Sprintf("Error: %v", err)
				} else {
					m.message = fmt.Sprintf("Deleted group '%s'", groupToDelete)
					m.groups = m.manager.GetAllGroups()
					if m.cursor >= len(m.groups) {
						m.cursor = len(m.groups) - 1
					}
				}
				m.confirmMode = false
				return m, nil
			}
			return m, nil
		}

		// Handle text input modes
		if m.inputMode != "" {
			switch msg.Type {
			case tea.KeyEsc:
				m.inputMode = ""
				m.pendingName = ""
				m.input.SetValue("")
				m.message = "Cancelled"
				return m, nil

			case tea.KeyEnter:
				value := strings.TrimSpace(m.input.Value())

				switch m.inputMode {
				case "new_name":
					if value == "" {
						m.message = "Name cannot be empty"
						return m, nil
					}
					if value == "default" {
						m.message = "Cannot use 'default' as group name"
						return m, nil
					}
					// Check if group already exists
					for _, g := range m.groups {
						if g == value {
							m.message = fmt.Sprintf("Group '%s' already exists", value)
							return m, nil
						}
					}
					m.pendingName = value
					m.inputMode = "new_prefix"
					m.input.SetValue("")
					m.input.Placeholder = "e.g. <grove> g"
					m.message = "Enter prefix key (optional):"
					return m, nil

				case "new_prefix":
					// Create the group
					if err := m.manager.CreateGroup(m.pendingName, value); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					} else {
						m.message = fmt.Sprintf("Created group '%s'", m.pendingName)
						m.groups = m.manager.GetAllGroups()
					}
					m.inputMode = ""
					m.pendingName = ""
					m.input.SetValue("")
					return m, nil

				case "rename":
					if value == "" {
						m.message = "Name cannot be empty"
						return m, nil
					}
					if value == "default" {
						m.message = "Cannot rename to 'default'"
						return m, nil
					}
					oldName := m.groups[m.cursor]
					if err := m.manager.RenameGroup(oldName, value); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					} else {
						m.message = fmt.Sprintf("Renamed '%s' to '%s'", oldName, value)
						m.groups = m.manager.GetAllGroups()
					}
					m.inputMode = ""
					m.input.SetValue("")
					return m, nil

				case "edit_prefix":
					groupName := m.groups[m.cursor]
					if err := m.manager.SetGroupPrefix(groupName, value); err != nil {
						m.message = fmt.Sprintf("Error: %v", err)
					} else {
						m.message = fmt.Sprintf("Updated prefix for '%s'", groupName)
					}
					m.inputMode = ""
					m.input.SetValue("")
					return m, nil
				}
				return m, nil

			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}

		// Handle move mode
		if m.moveMode {
			switch {
			case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyEsc, key.Matches(msg, m.keys.MoveMode):
				m.moveMode = false
				m.message = "Exited move mode"
				return m, nil

			case key.Matches(msg, m.keys.MoveUp):
				if m.cursor > 1 { // Can't move before default (index 0)
					currentGroup := m.groups[m.cursor]
					prevGroup := m.groups[m.cursor-1]

					// Use position-based ordering: current goes to prev's position, prev goes to current's
					// Position in list (excluding default at 0) determines order
					_ = m.manager.SetGroupOrder(currentGroup, m.cursor-2) // Move up
					_ = m.manager.SetGroupOrder(prevGroup, m.cursor-1)    // Move down

					// Refresh list
					m.groups = m.manager.GetAllGroups()
					m.cursor--
					m.message = "Moved up"
				}
				return m, nil

			case key.Matches(msg, m.keys.MoveDown):
				if m.cursor < len(m.groups)-1 && m.cursor > 0 { // Can't move default
					currentGroup := m.groups[m.cursor]
					nextGroup := m.groups[m.cursor+1]

					// Use position-based ordering
					_ = m.manager.SetGroupOrder(currentGroup, m.cursor) // Move down
					_ = m.manager.SetGroupOrder(nextGroup, m.cursor-1)  // Move up

					// Refresh list
					m.groups = m.manager.GetAllGroups()
					m.cursor++
					m.message = "Moved down"
				}
				return m, nil

			case msg.Type == tea.KeyEnter:
				m.moveMode = false
				m.message = "Order saved"
				return m, nil
			}
			return m, nil
		}

		// Normal mode key handlers
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.Toggle()
			return m, nil

		case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyEsc:
			// Hand back to km
			m.nextCommand = "km"
			return m, nil // Don't quit - parent will handle view switch

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.groups)-1 {
				m.cursor++
			}
			return m, nil

		case key.Matches(msg, m.keys.PageUp):
			m.cursor -= 5
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case key.Matches(msg, m.keys.PageDown):
			m.cursor += 5
			if m.cursor >= len(m.groups) {
				m.cursor = len(m.groups) - 1
			}
			return m, nil

		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
			return m, nil

		case key.Matches(msg, m.keys.Bottom):
			m.cursor = len(m.groups) - 1
			return m, nil

		case key.Matches(msg, m.keys.New):
			m.inputMode = "new_name"
			m.input.SetValue("")
			m.input.Placeholder = "group-name"
			m.input.Focus()
			m.message = "Enter new group name:"
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Delete):
			if m.cursor == 0 {
				m.message = "Cannot delete default group"
				return m, nil
			}
			m.confirmMode = true
			m.message = fmt.Sprintf("Delete '%s'? All mappings will be lost. [y/N]", m.groups[m.cursor])
			return m, nil

		case key.Matches(msg, m.keys.Rename):
			if m.cursor == 0 {
				m.message = "Cannot rename default group"
				return m, nil
			}
			m.inputMode = "rename"
			m.input.SetValue(m.groups[m.cursor])
			m.input.Placeholder = "new-name"
			m.input.Focus()
			m.message = "Enter new name:"
			return m, textinput.Blink

		case key.Matches(msg, m.keys.EditPrefix):
			if m.cursor == 0 {
				m.message = "Use main prefix setting for default group"
				return m, nil
			}
			cfg, _ := m.manager.GetGroupConfig(m.groups[m.cursor])
			m.inputMode = "edit_prefix"
			m.input.SetValue(cfg.Prefix)
			m.input.Placeholder = "<grove> g"
			m.input.Focus()
			m.message = "Enter new prefix:"
			return m, textinput.Blink

		case key.Matches(msg, m.keys.MoveMode):
			if m.cursor == 0 {
				m.message = "Cannot move default group"
				return m, nil
			}
			m.moveMode = true
			m.message = "Move mode: j/k to reorder, Enter to confirm, Esc to cancel"
			return m, nil

		case key.Matches(msg, m.keys.Toggle), key.Matches(msg, m.keys.Select):
			// Switch to this group
			groupName := m.groups[m.cursor]
			m.manager.SetActiveGroup(groupName)
			_ = m.manager.SetLastAccessedGroup(groupName)
			m.nextCommand = "km"
			return m, nil // Don't quit - parent will handle view switch
		}
	}

	return m, nil
}

func (m *groupsModel) View() string {
	// If help is visible, show it
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder

	// Title
	title := fmt.Sprintf("%s Group Management", core_theme.IconKeyboard)
	b.WriteString(core_theme.DefaultTheme.Header.Render(title))

	// Mode indicator
	if m.moveMode {
		b.WriteString(" " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]"))
	}
	b.WriteString("\n\n")

	// Build table
	headers := []string{"#", "Name", "Prefix Key", "Sessions", "Status"}
	var rows [][]string

	for i, g := range m.groups {
		var icon, prefix, status string
		sessionCount := m.manager.GetGroupSessionCount(g)

		if g == "default" {
			if defIcon := m.manager.GetDefaultIcon(); defIcon != "" {
				icon = resolveIcon(defIcon)
			} else {
				icon = core_theme.IconHome
			}
			prefix = resolvePrefixDisplay(m.manager.GetPrefixForGroup("default"))
			status = ""
		} else {
			cfg, ok := m.manager.GetGroupConfig(g)
			if ok {
				if cfg.Icon != "" {
					icon = resolveIcon(cfg.Icon)
				} else {
					icon = core_theme.IconFolderStar // Default for groups without icon
				}
				prefix = resolvePrefixDisplay(cfg.Prefix)
				if cfg.Active != nil && !*cfg.Active {
					status = core_theme.DefaultTheme.Muted.Render("(Inactive)")
				}
			}
		}

		// Combine icon with name
		displayName := g
		if icon != "" {
			displayName = icon + " " + g
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			displayName,
			prefix,
			fmt.Sprintf("%d", sessionCount),
			status,
		})
	}

	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{})
	b.WriteString(tableStr)
	b.WriteString("\n\n")

	// Input mode UI
	if m.inputMode != "" {
		b.WriteString(core_theme.DefaultTheme.Header.Render(m.message))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(m.input.View())
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Muted.Render("  (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	} else if m.confirmMode {
		b.WriteString(core_theme.DefaultTheme.Warning.Render("  ⚠ " + m.message))
		b.WriteString("\n\n")
	} else if m.message != "" {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(m.message))
		b.WriteString("\n")
	}

	// Help
	b.WriteString(m.help.View())

	return pageStyle.Render(b.String())
}
