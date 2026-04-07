// Package groups hosts the extracted nav workspace-groups management TUI.
// It depends only on the small Store interface defined here, so it can be
// embedded by any host that satisfies the methods. Standalone nav supplies
// *tmux.Manager.
package groups

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
)

// KeyMap is the alias used inside this package for the groups keymap.
type KeyMap = navkeymap.GroupsKeyMap

// pageStyle is the default lipgloss style for the groups view.
var pageStyle = lipgloss.NewStyle()

// Store is the narrow interface the groups TUI needs from its host. The nav
// binary's *tmux.Manager satisfies it implicitly.
type Store interface {
	GetAllGroups() []string
	GetGroupSessionCount(name string) int
	GetDefaultIcon() string
	GetGroupIcon(name string) string
	GetPrefixForGroup(name string) string
	IsGroupExplicitlyInactive(name string) bool

	TakeSnapshot()
	Undo() error
	Redo() error

	SetActiveGroup(name string)
	SetLastAccessedGroup(name string) error
	CreateGroup(name, prefix string) error
	DeleteGroup(name string) error
	RenameGroup(oldName, newName string) error
	SetGroupPrefix(name, prefix string) error
	SetGroupOrder(name string, order int) error
}

// Model is the exported groups TUI model. New() constructs one.
type Model = groupsModel

// New constructs a Model. The host supplies the Store, the keymap, and an
// optional callback that fires after undo/redo so the host can reload its
// own configuration (e.g. tmux server bindings). ReloadConfig may be nil.
func New(store Store, keys KeyMap, reloadConfig func() error) *Model {
	m := newGroupsModel(store, keys, reloadConfig)
	return &m
}

// NextCommand returns the host-handoff hint set by the model when it wants
// the parent router to switch back to a different view (currently only "km"
// for the key-manage view, used after the user finishes group management).
func (m *Model) NextCommand() string { return m.nextCommand }

// ClearNextCommand resets the handoff hint after the host has acted on it.
func (m *Model) ClearNextCommand() { m.nextCommand = "" }

// InputMode returns the current text-input mode (used by host UIs to know
// whether the user is in a text-input mode).
func (m *Model) InputMode() string { return m.inputMode }

// Reset re-reads the group list from the Store, resets the cursor, and
// clears any pending status message. Used by the host router when entering
// the groups view from another TUI.
func (m *Model) Reset() {
	m.groups = m.manager.GetAllGroups()
	m.cursor = 0
	m.message = ""
}

// resolveIcon converts a configured icon reference (a name token like
// "tree" or a literal glyph) into the rendered glyph. Mirrors the helper
// in cmd/nav/key_manage.go and pkg/tui/sessionizer/render.go.
func resolveIcon(iconRef string) string {
	switch iconRef {
	case "IconTree", "tree":
		return core_theme.IconTree
	case "IconProject", "project":
		return core_theme.IconProject
	case "IconRepo", "repo":
		return core_theme.IconRepo
	case "IconWorktree", "worktree":
		return core_theme.IconWorktree
	case "IconEcosystem", "ecosystem":
		return core_theme.IconEcosystem
	case "IconFolder", "folder":
		return core_theme.IconFolder
	case "IconFolderStar", "folder-star", "star":
		return core_theme.IconFolderStar
	case "IconHome", "home":
		return core_theme.IconHome
	case "IconCloud", "cloud":
		return "󰅧"
	case "IconCode", "code":
		return core_theme.IconCode
	case "IconBriefcase", "briefcase", "work":
		return "󰃖"
	case "IconKeyboard", "keyboard":
		return core_theme.IconKeyboard
	case "IconNote", "note":
		return core_theme.IconNote
	case "IconPlan", "plan":
		return core_theme.IconPlan
	default:
		return iconRef
	}
}

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

// groupsModel is the model for the groups management TUI.
type groupsModel struct {
	manager       Store
	reloadConfig  func() error
	groups        []string
	cursor        int
	keys          KeyMap
	help          help.Model

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

func newGroupsModel(store Store, keys KeyMap, reloadConfig func() error) groupsModel {
	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Group Management - Help").
		Build()

	ti := textinput.New()
	ti.CharLimit = 50
	ti.Width = 40

	return groupsModel{
		manager:      store,
		reloadConfig: reloadConfig,
		groups:       store.GetAllGroups(),
		cursor:       0,
		keys:         keys,
		help:         helpModel,
		input:        ti,
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

		// Handle confirmation mode
		if m.confirmMode {
			switch {
			case msg.Type == tea.KeyEsc, msg.String() == "n", msg.String() == "N":
				m.confirmMode = false
				m.message = "Cancelled"
				return m, nil

			case msg.String() == "y", msg.String() == "Y":
				// Execute delete
				m.manager.TakeSnapshot()
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
					m.manager.TakeSnapshot()
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
					m.manager.TakeSnapshot()
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
					m.manager.TakeSnapshot()
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
					m.manager.TakeSnapshot()
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
					m.manager.TakeSnapshot()
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
			currentPrefix := m.manager.GetPrefixForGroup(m.groups[m.cursor])
			m.inputMode = "edit_prefix"
			m.input.SetValue(currentPrefix)
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

		case key.Matches(msg, m.keys.Undo):
			if err := m.manager.Undo(); err != nil {
				m.message = fmt.Sprintf("Undo failed: %v", err)
			} else {
				m.refreshStateAfterUndoRedo()
				m.message = "Undo applied"
			}

		case key.Matches(msg, m.keys.Redo):
			if err := m.manager.Redo(); err != nil {
				m.message = fmt.Sprintf("Redo failed: %v", err)
			} else {
				m.refreshStateAfterUndoRedo()
				m.message = "Redo applied"
			}
		}
	}

	return m, nil
}

func (m *groupsModel) refreshStateAfterUndoRedo() {
	m.groups = m.manager.GetAllGroups()
	if m.cursor >= len(m.groups) {
		m.cursor = len(m.groups) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	if m.reloadConfig != nil {
		_ = m.reloadConfig()
	}
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
			rawIcon := m.manager.GetGroupIcon(g)
			if rawIcon != "" {
				icon = resolveIcon(rawIcon)
			} else {
				icon = core_theme.IconFolderStar // Default for groups without icon
			}
			prefix = resolvePrefixDisplay(m.manager.GetPrefixForGroup(g))
			if m.manager.IsGroupExplicitlyInactive(g) {
				status = core_theme.DefaultTheme.Muted.Render("(Inactive)")
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
