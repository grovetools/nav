package history

import (
	"fmt"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/nav/pkg/api"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case embed.FocusMsg:
		return m, m.refreshCmd()

	case embed.BlurMsg:
		return m, nil

	case embed.SetWorkspaceMsg:
		// Workspace changed — drop the current list and reload. The new
		// workspace's loader closure was installed at construction time,
		// so we just call it again.
		m.items = nil
		m.filteredItems = nil
		m.enrichedProjects = map[string]*api.Project{}
		m.cursor = 0
		return m, m.refreshCmd()

	case historyLoadedMsg:
		if msg.err == nil {
			m.replaceItems(msg.items)
		}
		m.isLoading = false

		// Kick a git status fetch for the newly loaded projects so the
		// Git column renders accurate data.
		var projectList []*api.Project
		for _, it := range m.items {
			projectList = append(projectList, it.project)
		}
		if len(projectList) > 0 {
			m.enrichmentLoading["git"] = true
			m.isLoading = true
			return m, tea.Batch(
				fetchAllGitStatusesCmd(projectList),
				spinnerTickCmd(),
			)
		}
		return m, nil

	case gitStatusMapMsg:
		for path, status := range msg.statuses {
			if proj, ok := m.enrichedProjects[path]; ok {
				proj.GitStatus = status
			}
		}
		m.enrichmentLoading["git"] = false
		return m, nil

	case spinnerTickMsg:
		anyLoading := false
		for _, loading := range m.enrichmentLoading {
			if loading {
				anyLoading = true
				break
			}
		}
		if anyLoading {
			m.spinnerFrame++
			return m, spinnerTickCmd()
		}
		m.isLoading = false
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// refreshCmd re-invokes the HistoryLoader (if any) and returns the
// loading command chain.
func (m *Model) refreshCmd() tea.Cmd {
	if m.cfg.LoadHistory == nil {
		return nil
	}
	m.isLoading = true
	return tea.Batch(loadHistoryCmd(m.cfg.LoadHistory), spinnerTickCmd())
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	// Handle filter mode
	if m.filterMode {
		switch msg.Type {
		case tea.KeyEsc:
			m.filterMode = false
			m.filterText = ""
			m.applyFilter()
			return m, nil
		case tea.KeyEnter:
			m.filterMode = false
			return m, nil
		case tea.KeyBackspace:
			if len(m.filterText) > 0 {
				m.filterText = m.filterText[:len(m.filterText)-1]
				m.applyFilter()
			}
			return m, nil
		case tea.KeyRunes:
			m.filterText += string(msg.Runes)
			m.applyFilter()
			return m, nil
		}
		return m, nil
	}

	// Handle jumpMode (mini-leader key 'g')
	if m.jumpMode {
		m.jumpMode = false
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= '1' && r <= '9' {
				targetIndex := int(r - '1')
				if targetIndex < len(m.filteredItems) {
					m.selected = m.filteredItems[targetIndex].project
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			} else if r == 'g' {
				m.cursor = 0
				return m, nil
			}
		}
		return m, nil
	}

	// Enter jumpMode when 'g' is pressed
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'g' {
		m.jumpMode = true
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.quitting = true
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		m.filterMode = true
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filteredItems)-1 {
			m.cursor++
		}

	case key.Matches(msg, m.keys.Open):
		if m.cursor < len(m.filteredItems) {
			m.selected = m.filteredItems[m.cursor].project
			m.quitting = true
			return m, tea.Quit
		}

	case key.Matches(msg, m.keys.CopyPath):
		if m.cursor < len(m.filteredItems) {
			path := m.filteredItems[m.cursor].project.Path
			if err := clipboard.WriteAll(path); err != nil {
				m.statusMessage = fmt.Sprintf("Error copying path: %v", err)
			} else {
				m.statusMessage = fmt.Sprintf("Copied: %s", path)
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.GoToSessionize):
		if m.cursor < len(m.filteredItems) {
			path := m.filteredItems[m.cursor].project.Path
			return m, func() tea.Msg {
				return JumpToSessionizeMsg{Path: path, ApplyGroupFilter: true}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.FocusCurrent):
		if m.cursor < len(m.filteredItems) {
			path := m.filteredItems[m.cursor].project.Path
			return m, func() tea.Msg {
				return JumpToSessionizeMsg{Path: path, ApplyGroupFilter: false}
			}
		}
		return m, nil
	}

	return m, nil
}
