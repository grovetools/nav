package windows

import (
	"context"
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/embed"
)

func (m *Model) Init() tea.Cmd {
	return fetchWindowsCmd(m.driver, m.sessionName)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(m.width, m.height)
		return m, nil

	case embed.FocusMsg:
		// Refresh the window list on focus so the browser catches any
		// windows created or killed while the host had another panel
		// active.
		return m, fetchWindowsCmd(m.driver, m.sessionName)

	case embed.BlurMsg:
		return m, nil

	case embed.SetWorkspaceMsg:
		// Workspace-scoped hosts repoint by swapping in a new Model.
		// Treat SetWorkspaceMsg as a best-effort refresh of the current
		// session for embeds that reuse the same Model instance.
		return m, fetchWindowsCmd(m.driver, m.sessionName)

	case LoadedMsg:
		m.windows = msg.Windows
		if m.showChildProcesses {
			m.processCache = buildProcessCache(m.windows)
		}
		m.applyFilter()

		// Set initial cursor to the active window
		for i, win := range m.filteredWindows {
			if win.IsActive {
				m.cursor = i
				break
			}
		}

		if len(m.filteredWindows) > 0 && m.cursor < len(m.filteredWindows) {
			return m, fetchPreviewCmd(m.driver, m.sessionName, m.filteredWindows[m.cursor].Index)
		}
		return m, nil

	case PreviewLoadedMsg:
		m.preview = msg.Preview
		return m, nil

	case ErrorMsg:
		m.err = msg.Err
		return m, tea.Quit

	case tea.KeyMsg:
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

		switch m.mode {
		case "filter":
			return m.updateFilter(msg)
		case "rename":
			return m.updateRename(msg)
		case "move":
			return m.updateMove(msg)
		default: // "normal"
			return m.updateNormal(msg)
		}
	}

	return m, nil
}

func (m *Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle jumpMode (mini-leader key 'g')
	if m.jumpMode {
		m.jumpMode = false
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= '0' && r <= '9' {
				index, _ := strconv.Atoi(string(r))
				for i := range m.windows {
					if m.windows[i].Index == index {
						m.selectedWindow = &m.windows[i]
						m.quitting = true
						return m, tea.Quit
					}
				}
				return m, nil
			} else if r == 'g' {
				m.cursor = 0
				if len(m.filteredWindows) > 0 {
					return m, fetchPreviewCmd(m.driver, m.sessionName, m.filteredWindows[m.cursor].Index)
				}
				return m, nil
			}
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < len(m.filteredWindows) {
				return m, fetchPreviewCmd(m.driver, m.sessionName, m.filteredWindows[m.cursor].Index)
			}
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filteredWindows)-1 {
			m.cursor++
			if m.cursor < len(m.filteredWindows) {
				return m, fetchPreviewCmd(m.driver, m.sessionName, m.filteredWindows[m.cursor].Index)
			}
		}
	case key.Matches(msg, m.keys.Filter):
		m.mode = "filter"
		m.filterInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Rename):
		if m.cursor < len(m.filteredWindows) {
			m.mode = "rename"
			m.renameInput.SetValue(m.filteredWindows[m.cursor].Name)
			m.renameInput.Focus()
			return m, textinput.Blink
		}
	case key.Matches(msg, m.keys.MoveMode):
		m.mode = "move"
		m.originalWindows = make([]tmuxclient.Window, len(m.filteredWindows))
		copy(m.originalWindows, m.filteredWindows)
		return m, nil
	case key.Matches(msg, m.keys.Close):
		if m.cursor < len(m.filteredWindows) {
			target := fmt.Sprintf("%s:%d", m.sessionName, m.filteredWindows[m.cursor].Index)
			_ = m.driver.KillWindow(context.Background(), target)
			if m.cursor >= len(m.filteredWindows)-1 {
				m.cursor--
			}
			return m, fetchWindowsCmd(m.driver, m.sessionName)
		}
	case key.Matches(msg, m.keys.Switch):
		if m.cursor < len(m.filteredWindows) {
			m.selectedWindow = &m.filteredWindows[m.cursor]
			m.quitting = true
			return m, tea.Quit
		}
	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit
	case key.Matches(msg, m.keys.Back):
		m.quitting = true
		return m, tea.Quit
	}

	// Enter jumpMode when 'g' is pressed
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'g' {
		m.jumpMode = true
		return m, nil
	}

	return m, nil
}

func (m *Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEnter:
		if m.cursor < len(m.filteredWindows) {
			m.selectedWindow = &m.filteredWindows[m.cursor]
			m.quitting = true
			return m, tea.Quit
		}
	case tea.KeyEsc:
		m.mode = "normal"
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.applyFilter()
	default:
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.applyFilter()
	}
	return m, cmd
}

func (m *Model) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEnter:
		if m.cursor < len(m.filteredWindows) {
			target := fmt.Sprintf("%s:%d", m.sessionName, m.filteredWindows[m.cursor].Index)
			_ = m.driver.RenameWindow(context.Background(), target, m.renameInput.Value())
			m.mode = "normal"
			m.renameInput.Blur()
			return m, fetchWindowsCmd(m.driver, m.sessionName)
		}
	case tea.KeyEsc:
		m.mode = "normal"
		m.renameInput.Blur()
	default:
		m.renameInput, cmd = m.renameInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// Exit move mode - apply all changes to tmux
	case key.Matches(msg, m.keys.MoveMode), msg.Type == tea.KeyEsc, msg.Type == tea.KeyEnter:
		m.mode = "normal"

		if len(m.originalWindows) > 0 {
			baseIndex := m.originalWindows[0].Index
			for _, win := range m.originalWindows {
				if win.Index < baseIndex {
					baseIndex = win.Index
				}
			}

			ctx := context.Background()
			// First, move all windows to temporary high indices to avoid conflicts
			tempBase := 9000
			for i, win := range m.filteredWindows {
				srcTarget := fmt.Sprintf("%s:%d", m.sessionName, win.Index)
				tempTarget := fmt.Sprintf("%s:%d", m.sessionName, tempBase+i)
				_ = m.driver.MoveWindow(ctx, srcTarget, tempTarget)
			}

			// Now move them from temp indices to final positions
			for i := 0; i < len(m.filteredWindows); i++ {
				srcTarget := fmt.Sprintf("%s:%d", m.sessionName, tempBase+i)
				finalTarget := fmt.Sprintf("%s:%d", m.sessionName, baseIndex+i)
				_ = m.driver.MoveWindow(ctx, srcTarget, finalTarget)
			}

			m.originalWindows = nil
		}

		return m, fetchWindowsCmd(m.driver, m.sessionName)

	// Move window up (visual only, no tmux changes yet)
	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.MoveUp):
		if m.cursor > 0 {
			m.filteredWindows[m.cursor], m.filteredWindows[m.cursor-1] = m.filteredWindows[m.cursor-1], m.filteredWindows[m.cursor]
			m.cursor--
			return m, nil
		}

	// Move window down (visual only, no tmux changes yet)
	case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.MoveDown):
		if m.cursor < len(m.filteredWindows)-1 {
			m.filteredWindows[m.cursor], m.filteredWindows[m.cursor+1] = m.filteredWindows[m.cursor+1], m.filteredWindows[m.cursor]
			m.cursor++
			return m, nil
		}
	}
	return m, nil
}
