package groups

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/embed"
)

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case embed.FocusMsg:
		// Refresh the group list on focus so the TUI catches any
		// mutations made while it was blurred.
		m.Reset()
		return m, nil

	case embed.BlurMsg:
		return m, nil

	case embed.SetWorkspaceMsg:
		// Workspace changed — re-read from the Store (the new workspace
		// reuses the same Store in standalone nav; terminal may swap
		// Models instead).
		m.Reset()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m.handleInputMode(msg)
	}

	// Handle move mode
	if m.moveMode {
		return m.handleMoveMode(msg)
	}

	return m.handleNormal(msg)
}

func (m *Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m *Model) handleMoveMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyEsc, key.Matches(msg, m.keys.MoveMode):
		m.moveMode = false
		m.message = "Exited move mode"
		return m, nil

	case key.Matches(msg, m.keys.MoveUp):
		if m.cursor > 1 { // Can't move before default (index 0)
			currentGroup := m.groups[m.cursor]
			prevGroup := m.groups[m.cursor-1]

			m.manager.TakeSnapshot()
			_ = m.manager.SetGroupOrder(currentGroup, m.cursor-2)
			_ = m.manager.SetGroupOrder(prevGroup, m.cursor-1)

			m.groups = m.manager.GetAllGroups()
			m.cursor--
			m.message = "Moved up"
		}
		return m, nil

	case key.Matches(msg, m.keys.MoveDown):
		if m.cursor < len(m.groups)-1 && m.cursor > 0 {
			currentGroup := m.groups[m.cursor]
			nextGroup := m.groups[m.cursor+1]

			m.manager.TakeSnapshot()
			_ = m.manager.SetGroupOrder(currentGroup, m.cursor)
			_ = m.manager.SetGroupOrder(nextGroup, m.cursor-1)

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

func (m *Model) handleNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
		return m, nil

	case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyEsc:
		m.nextCommand = "km"
		return m, nil

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
		groupName := m.groups[m.cursor]
		m.manager.SetActiveGroup(groupName)
		_ = m.manager.SetLastAccessedGroup(groupName)
		m.nextCommand = "km"
		return m, nil

	case key.Matches(msg, m.keys.Undo):
		if err := m.manager.Undo(); err != nil {
			m.message = fmt.Sprintf("Undo failed: %v", err)
		} else {
			m.refreshStateAfterUndoRedo()
			m.message = "Undo applied"
		}
		return m, nil

	case key.Matches(msg, m.keys.Redo):
		if err := m.manager.Redo(); err != nil {
			m.message = fmt.Sprintf("Redo failed: %v", err)
		} else {
			m.refreshStateAfterUndoRedo()
			m.message = "Redo applied"
		}
		return m, nil
	}

	return m, nil
}
