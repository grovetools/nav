package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/keymap"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/spf13/cobra"
)

var windowsCmd = &cobra.Command{
	Use:   "windows",
	Short: "Interactively manage windows in the current tmux session",
	Long:  `Launches a TUI to list, filter, and manage windows in the current tmux session.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := tmuxclient.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create tmux client: %w", err)
		}

		sessionName, err := client.GetCurrentSession(context.Background())
		if err != nil {
			return fmt.Errorf("not in a tmux session or failed to get session name: %w", err)
		}

		m := newWindowsModel(client, sessionName)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		if wm, ok := finalModel.(windowsModel); ok && wm.selectedWindow != nil {
			target := fmt.Sprintf("%s:%d", wm.sessionName, wm.selectedWindow.Index)
			if err := client.SwitchClient(context.Background(), target); err != nil {
				// This might fail if not in a popup, which is fine
			}
			// Close popup if we were in one
			_ = client.ClosePopupCmd().Run()
		}

		return nil
	},
}

// --- Bubbletea Model ---

type windowsModel struct {
	client          *tmuxclient.Client
	sessionName     string
	windows         []tmuxclient.Window
	filteredWindows []tmuxclient.Window
	cursor          int
	help            help.Model
	keys            windowsKeyMap
	filterInput     textinput.Model
	renameInput     textinput.Model
	mode            string // "normal", "filter", "rename"
	selectedWindow  *tmuxclient.Window
	quitting        bool
	width, height   int
	err             error
}

type windowsKeyMap struct {
	keymap.Base
	Switch key.Binding
	Filter key.Binding
	Rename key.Binding
	Close  key.Binding
}

func (k windowsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Rename, k.Close, k.Quit}
}

func (k windowsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up, k.Down,
			key.NewBinding(key.WithKeys("0-9"), key.WithHelp("0-9", "jump to window")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Switch, k.Filter, k.Rename, k.Close, k.Help, k.Quit,
		},
	}
}

var windowsKeys = windowsKeyMap{
	Base: keymap.NewBase(),
	Switch: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "switch"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Rename: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "rename"),
	),
	Close: key.NewBinding(
		key.WithKeys("X"),
		key.WithHelp("X", "close"),
	),
}

type windowsLoadedMsg struct {
	windows []tmuxclient.Window
}
type errorMsg struct{ err error }

func fetchWindowsCmd(client *tmuxclient.Client, sessionName string) tea.Cmd {
	return func() tea.Msg {
		windows, err := client.ListWindowsDetailed(context.Background(), sessionName)
		if err != nil {
			return errorMsg{err}
		}
		// Sort windows by index
		sort.Slice(windows, func(i, j int) bool {
			return windows[i].Index < windows[j].Index
		})
		return windowsLoadedMsg{windows}
	}
}

func newWindowsModel(client *tmuxclient.Client, sessionName string) windowsModel {
	filterInput := textinput.New()
	filterInput.Placeholder = "Filter by name..."
	filterInput.CharLimit = 64

	renameInput := textinput.New()
	renameInput.Placeholder = "New window name..."
	renameInput.CharLimit = 128

	return windowsModel{
		client:      client,
		sessionName: sessionName,
		keys:        windowsKeys,
		help:        help.New(windowsKeys),
		filterInput: filterInput,
		renameInput: renameInput,
		mode:        "normal",
	}
}

func (m windowsModel) Init() tea.Cmd {
	return fetchWindowsCmd(m.client, m.sessionName)
}

func (m windowsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetSize(m.width, m.height)

	case windowsLoadedMsg:
		m.windows = msg.windows
		m.applyFilter()
		return m, nil

	case errorMsg:
		m.err = msg.err
		return m, tea.Quit

	case tea.KeyMsg:
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
		}

		switch m.mode {
		case "filter":
			return m.updateFilter(msg)
		case "rename":
			return m.updateRename(msg)
		default: // "normal"
			return m.updateNormal(msg)
		}
	}

	return m, cmd
}

func (m windowsModel) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var b strings.Builder
	b.WriteString(core_theme.DefaultTheme.Header.Render("Window Selector"))
	b.WriteString("\n\n")

	for i, win := range m.filteredWindows {
		cursor := " "
		if m.cursor == i {
			cursor = "→"
		}

		icon := getIconForWindow(win)

		name := win.Name
		if win.IsActive {
			name += "*"
		}

		line := fmt.Sprintf("%s %s %d: %s", cursor, icon, win.Index, name)
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Footer
	switch m.mode {
	case "filter":
		b.WriteString("Filter: " + m.filterInput.View())
	case "rename":
		b.WriteString("Rename: " + m.renameInput.View())
	default:
		b.WriteString(m.help.View())
	}

	return pageStyle.Render(b.String())
}

func (m windowsModel) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filteredWindows)-1 {
			m.cursor++
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
	case key.Matches(msg, m.keys.Close):
		if m.cursor < len(m.filteredWindows) {
			target := fmt.Sprintf("%s:%d", m.sessionName, m.filteredWindows[m.cursor].Index)
			m.client.KillWindow(context.Background(), target)
			if m.cursor >= len(m.filteredWindows)-1 {
				m.cursor--
			}
			return m, fetchWindowsCmd(m.client, m.sessionName)
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
	}

	// Number keys for direct switching
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		if r >= '0' && r <= '9' {
			index, _ := strconv.Atoi(string(r))
			for _, win := range m.windows {
				if win.Index == index {
					m.selectedWindow = &win
					m.quitting = true
					return m, tea.Quit
				}
			}
		}
	}

	return m, nil
}

func (m windowsModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m windowsModel) updateRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.Type {
	case tea.KeyEnter:
		if m.cursor < len(m.filteredWindows) {
			target := fmt.Sprintf("%s:%d", m.sessionName, m.filteredWindows[m.cursor].Index)
			m.client.RenameWindow(context.Background(), target, m.renameInput.Value())
			m.mode = "normal"
			m.renameInput.Blur()
			return m, fetchWindowsCmd(m.client, m.sessionName)
		}
	case tea.KeyEsc:
		m.mode = "normal"
		m.renameInput.Blur()
	default:
		m.renameInput, cmd = m.renameInput.Update(msg)
	}
	return m, cmd
}

func (m *windowsModel) applyFilter() {
	filterText := strings.ToLower(m.filterInput.Value())
	if filterText == "" {
		m.filteredWindows = m.windows
	} else {
		var filtered []tmuxclient.Window
		for _, win := range m.windows {
			if strings.Contains(strings.ToLower(win.Name), filterText) {
				filtered = append(filtered, win)
			}
		}
		m.filteredWindows = filtered
	}
	if m.cursor >= len(m.filteredWindows) {
		m.cursor = 0
	}
}

func getIconForWindow(w tmuxclient.Window) string {
	switch w.Name {
	case "editor":
		return "✏️"
	case "notebook":
		return "📒"
	case "console":
		return "❯"
	case "plan":
		return "🗺️"
	}
	if strings.Contains(w.Command, "impl") {
		return core_theme.IconInteractiveAgent
	}
	return " "
}

func init() {
	rootCmd.AddCommand(windowsCmd)
}
