package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/keymap"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/internal/manager"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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

		// Load config to check if child process detection is enabled (default: false)
		showChildProcesses := false
		if tmuxCfg, err := loadTmuxConfig(); err == nil && tmuxCfg != nil {
			showChildProcesses = tmuxCfg.ShowChildProcesses
		}

		m := newWindowsModel(client, sessionName, showChildProcesses)
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
	client             *tmuxclient.Client
	sessionName        string
	windows            []tmuxclient.Window
	filteredWindows    []tmuxclient.Window
	cursor             int
	help               help.Model
	keys               windowsKeyMap
	filterInput        textinput.Model
	renameInput        textinput.Model
	mode               string // "normal", "filter", "rename"
	selectedWindow     *tmuxclient.Window
	quitting           bool
	width, height      int
	err                error
	preview            string
	processCache       map[int]string // Cache PID -> process name mapping
	showChildProcesses bool           // Whether to detect child processes
	originalWindows    []tmuxclient.Window // Original order when entering move mode
}

type windowsKeyMap struct {
	keymap.Base
	Switch   key.Binding
	Filter   key.Binding
	Rename   key.Binding
	Close    key.Binding
	MoveMode key.Binding
	MoveUp   key.Binding
	MoveDown key.Binding
}

func (k windowsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit}
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
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Reorder")),
			k.MoveMode,
			key.NewBinding(key.WithKeys("j/k"), key.WithHelp("j/k", "move (in move mode)")),
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
	MoveMode: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "move window"),
	),
	MoveUp: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "move up"),
	),
	MoveDown: key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "move down"),
	),
}

type windowsLoadedMsg struct {
	windows []tmuxclient.Window
}
type previewLoadedMsg struct {
	preview string
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

func fetchPreviewCmd(client *tmuxclient.Client, sessionName string, windowIndex int) tea.Cmd {
	return func() tea.Msg {
		target := fmt.Sprintf("%s:%d", sessionName, windowIndex)
		preview, err := client.CapturePane(context.Background(), target)
		if err != nil {
			return previewLoadedMsg{preview: fmt.Sprintf("Error: %v", err)}
		}
		return previewLoadedMsg{preview: preview}
	}
}

func newWindowsModel(client *tmuxclient.Client, sessionName string, showChildProcesses bool) windowsModel {
	filterInput := textinput.New()
	filterInput.Placeholder = "Filter by name..."
	filterInput.CharLimit = 64

	renameInput := textinput.New()
	renameInput.Placeholder = "New window name..."
	renameInput.CharLimit = 128

	return windowsModel{
		client:             client,
		sessionName:        sessionName,
		keys:               windowsKeys,
		help:               help.New(windowsKeys),
		filterInput:        filterInput,
		renameInput:        renameInput,
		mode:               "normal",
		showChildProcesses: showChildProcesses,
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
		// Build process cache once for all windows (if enabled)
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

		// Fetch preview for the initially selected window
		if len(m.filteredWindows) > 0 && m.cursor < len(m.filteredWindows) {
			return m, fetchPreviewCmd(m.client, m.sessionName, m.filteredWindows[m.cursor].Index)
		}
		return m, nil

	case previewLoadedMsg:
		m.preview = msg.preview
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
		case "move":
			return m.updateMove(msg)
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

	// Calculate layout: 50% for list, 50% for preview
	if m.width < 40 {
		// Terminal too narrow for split view, just show list
		var b strings.Builder
		header := "Window Selector"
		if m.mode == "move" {
			header += " " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]")
		}
		b.WriteString(core_theme.DefaultTheme.Header.Render(header))
		b.WriteString("\n\n")

		for i, win := range m.filteredWindows {
			cursor := " "
			if m.cursor == i {
				cursor = "→"
			}

			icon := getIconForWindow(win)

			name := win.Name
			if win.IsActive {
				name = core_theme.DefaultTheme.Highlight.Render(win.Name + " «")
			}

			line := fmt.Sprintf("%s %s %d: %s", cursor, icon, win.Index, name)

			// Highlight the entire line when selected in move mode
			if m.cursor == i && m.mode == "move" {
				b.WriteString(core_theme.DefaultTheme.Selected.Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")

		switch m.mode {
		case "filter":
			b.WriteString("Filter: " + m.filterInput.View())
		case "rename":
			b.WriteString("Rename: " + m.renameInput.View())
		default:
			b.WriteString(m.help.View())
		}

		return b.String()
	}

	listWidth := m.width * 50 / 100
	if listWidth < 20 {
		listWidth = 20
	}
	if listWidth > m.width-20 {
		listWidth = m.width - 20
	}
	previewWidth := m.width - listWidth - 1 // -1 for separator
	if previewWidth < 10 {
		previewWidth = 10
	}

	// Build window list
	var listBuilder strings.Builder
	header := "Window Selector"
	if m.mode == "move" {
		header += " " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]")
	}
	listBuilder.WriteString(core_theme.DefaultTheme.Header.Render(header))
	listBuilder.WriteString("\n\n")

	for i, win := range m.filteredWindows {
		cursor := " "
		if m.cursor == i {
			cursor = "→"
		}

		icon := getIconForWindow(win)

		name := win.Name
		if win.IsActive {
			name = core_theme.DefaultTheme.Highlight.Render(win.Name + " «")
		}

		// Show process name in muted style
		line := fmt.Sprintf("%s %s %d: %s", cursor, icon, win.Index, name)

		// Get cached process name or fall back to current command
		processName := m.processCache[win.PID]
		if processName == "" {
			processName = win.Command
		}

		// Only show command if it's not a generic shell or wrapper
		if shouldShowCommand(processName) {
			processStyle := core_theme.DefaultTheme.Muted
			line += " " + processStyle.Render(fmt.Sprintf("[%s]", processName))
		}

		// Highlight the entire line when selected in move mode
		if m.cursor == i && m.mode == "move" {
			listBuilder.WriteString(core_theme.DefaultTheme.Selected.Render(line))
		} else {
			listBuilder.WriteString(line)
		}
		listBuilder.WriteString("\n")
	}

	listBuilder.WriteString("\n")

	// Footer for list
	switch m.mode {
	case "filter":
		listBuilder.WriteString("Filter: " + m.filterInput.View())
	case "rename":
		listBuilder.WriteString("Rename: " + m.renameInput.View())
	case "move":
		listBuilder.WriteString(core_theme.DefaultTheme.Muted.Render("Use j/k to reorder • Enter/Esc/m to apply"))
	default:
		listBuilder.WriteString(m.help.View())
	}

	// Build preview panel
	var previewBuilder strings.Builder
	previewBuilder.WriteString(core_theme.DefaultTheme.Header.Render("Preview"))
	previewBuilder.WriteString("\n\n")

	// Calculate max height for preview (account for header, footer, padding)
	maxPreviewHeight := m.height - 6 // Leave room for headers and footer
	if maxPreviewHeight < 1 {
		maxPreviewHeight = 1
	}

	// Wrap preview content and limit to maxPreviewHeight
	previewLines := strings.Split(m.preview, "\n")
	lineCount := 0
	for _, line := range previewLines {
		if lineCount >= maxPreviewHeight {
			break
		}
		if len(line) > previewWidth {
			previewBuilder.WriteString(line[:previewWidth])
		} else {
			previewBuilder.WriteString(line)
		}
		previewBuilder.WriteString("\n")
		lineCount++
	}

	// Use lipgloss to create side-by-side layout with height constraints
	listStyle := lipgloss.NewStyle().Width(listWidth).Height(m.height)
	previewStyle := lipgloss.NewStyle().Width(previewWidth).Height(m.height)

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		listStyle.Render(listBuilder.String()),
		previewStyle.Render(previewBuilder.String()),
	)

	return pageStyle.Render(content)
}

func (m windowsModel) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < len(m.filteredWindows) {
				return m, fetchPreviewCmd(m.client, m.sessionName, m.filteredWindows[m.cursor].Index)
			}
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filteredWindows)-1 {
			m.cursor++
			if m.cursor < len(m.filteredWindows) {
				return m, fetchPreviewCmd(m.client, m.sessionName, m.filteredWindows[m.cursor].Index)
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
		// Save original window order so we can apply changes on exit
		m.originalWindows = make([]tmuxclient.Window, len(m.filteredWindows))
		copy(m.originalWindows, m.filteredWindows)
		return m, nil
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
	case key.Matches(msg, m.keys.Back):
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

func (m windowsModel) updateMove(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// Exit move mode - apply all changes to tmux
	case key.Matches(msg, m.keys.MoveMode), msg.Type == tea.KeyEsc, msg.Type == tea.KeyEnter:
		m.mode = "normal"

		// Only apply reordering if we have original windows saved
		if len(m.originalWindows) > 0 {
			// Find the base index (minimum index from original windows)
			baseIndex := m.originalWindows[0].Index
			for _, win := range m.originalWindows {
				if win.Index < baseIndex {
					baseIndex = win.Index
				}
			}

			// First, move all windows to temporary high indices to avoid conflicts
			tempBase := 9000
			for i, win := range m.filteredWindows {
				srcTarget := fmt.Sprintf("%s:%d", m.sessionName, win.Index)
				tempTarget := fmt.Sprintf("%s:%d", m.sessionName, tempBase+i)

				cmd := exec.Command("tmux", "move-window", "-s", srcTarget, "-t", tempTarget)
				cmd.Run() // Best effort
			}

			// Now move them from temp indices to final positions
			for i := 0; i < len(m.filteredWindows); i++ {
				srcTarget := fmt.Sprintf("%s:%d", m.sessionName, tempBase+i)
				finalTarget := fmt.Sprintf("%s:%d", m.sessionName, baseIndex+i)

				cmd := exec.Command("tmux", "move-window", "-s", srcTarget, "-t", finalTarget)
				cmd.Run() // Best effort
			}

			// Clear original windows
			m.originalWindows = nil
		}

		// Refresh to get actual tmux state
		return m, fetchWindowsCmd(m.client, m.sessionName)

	// Move window up (visual only, no tmux changes yet)
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			// Just swap in the local list for instant visual feedback
			m.filteredWindows[m.cursor], m.filteredWindows[m.cursor-1] = m.filteredWindows[m.cursor-1], m.filteredWindows[m.cursor]
			m.cursor--
			return m, nil
		}

	// Move window down (visual only, no tmux changes yet)
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filteredWindows)-1 {
			// Just swap in the local list for instant visual feedback
			m.filteredWindows[m.cursor], m.filteredWindows[m.cursor+1] = m.filteredWindows[m.cursor+1], m.filteredWindows[m.cursor]
			m.cursor++
			return m, nil
		}
	}
	return m, nil
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

func shouldShowCommand(cmd string) bool {
	// Temporarily disabled - show all commands
	return true

	// Skip generic shells and wrappers that don't provide useful info
	// genericCommands := []string{
	// 	"fish",
	// 	"bash",
	// 	"zsh",
	// 	"sh",
	// 	"volta-shim",
	// 	"node-shim",
	// }

	// for _, generic := range genericCommands {
	// 	if cmd == generic {
	// 		return false
	// 	}
	// }

	// return true
}

func getIconForWindow(w tmuxclient.Window) string {
	// Check for special window name patterns first (highest priority)
	if strings.HasPrefix(w.Name, "job-") {
		return core_theme.IconRobot
	}
	if strings.Contains(w.Name, "code-review") {
		return core_theme.IconNoteReview
	}
	if strings.Contains(w.Name, "cx-edit") {
		return core_theme.IconFileTree
	}
	if strings.Contains(w.Name, "impl") || strings.Contains(w.Command, "impl") {
		return core_theme.IconInteractiveAgent
	}
	if strings.Contains(w.Name, "editor") {
		return core_theme.IconCode
	}
	if strings.Contains(w.Name, "notebook") {
		return core_theme.IconNotebook
	}
	if strings.Contains(w.Name, "term") {
		return core_theme.IconShell
	}
	if strings.Contains(w.Name, "plan") {
		return core_theme.IconPlan
	}

	// Check command as fallback (lower priority)
	if w.Command == "fish" {
		return core_theme.IconFish
	}

	return " "
}

func loadTmuxConfig() (*manager.TmuxConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	groveConfigPath := filepath.Join(homeDir, ".grove", "grove.yml")
	data, err := os.ReadFile(groveConfigPath)
	if err != nil {
		return nil, err
	}

	var rawConfig struct {
		Tmux *manager.TmuxConfig `yaml:"tmux"`
	}
	if err := yaml.Unmarshal(data, &rawConfig); err != nil {
		return nil, err
	}

	return rawConfig.Tmux, nil
}

func init() {
	rootCmd.AddCommand(windowsCmd)
}
