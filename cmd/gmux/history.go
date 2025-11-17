package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/pkg/models"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/help"
	"github.com/mattsolo1/grove-core/tui/components/table"
	"github.com/mattsolo1/grove-core/tui/keymap"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"github.com/spf13/cobra"
)

const mutedThreshold = 7 * 24 * time.Hour // 1 week

// historyItem holds a project and its last access time.
type historyItem struct {
	project *manager.SessionizeProject
	access  *workspace.ProjectAccess
}

var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h"},
	Short:   "View and switch to recently accessed project sessions",
	Long:    `Shows an interactive TUI listing recently accessed project sessions, sorted from most to least recent.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}

		// Fetch all known projects
		allProjects, err := mgr.GetAvailableProjects()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}
		projectMap := make(map[string]*manager.SessionizeProject)
		for i := range allProjects {
			projectMap[allProjects[i].Path] = &allProjects[i]
		}

		// Load and sort access history
		history, err := mgr.GetAccessHistory()
		if err != nil {
			return fmt.Errorf("failed to load access history: %w", err)
		}

		var historyAccesses []*workspace.ProjectAccess
		for _, access := range history.Projects {
			historyAccesses = append(historyAccesses, access)
		}
		sort.Slice(historyAccesses, func(i, j int) bool {
			return historyAccesses[i].LastAccessed.After(historyAccesses[j].LastAccessed)
		})

		// Build final list of items to display
		var items []historyItem
		for _, access := range historyAccesses {
			if project, ok := projectMap[access.Path]; ok {
				items = append(items, historyItem{
					project: project,
					access:  access,
				})
			}
		}

		if len(items) == 0 {
			fmt.Println("No session history found.")
			return nil
		}

		// Limit to 15 most recent
		if len(items) > 15 {
			items = items[:15]
		}

		// Load key mappings
		sessions, err := mgr.GetSessions()
		if err != nil {
			sessions = []models.TmuxSession{}
		}
		keyMap := make(map[string]string)
		for _, s := range sessions {
			if s.Path != "" {
				keyMap[filepath.Clean(s.Path)] = s.Key
			}
		}

		m := newHistoryModel(items, mgr, keyMap)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running program: %w", err)
		}

		// After TUI exits, check if a project was selected and sessionize it
		if hm, ok := finalModel.(*historyModel); ok && hm.selected != nil {
			// Record access again to bump it to the top of the history
			_ = mgr.RecordProjectAccess(hm.selected.Path)
			// Sessionize will create or switch to the tmux session
			return mgr.Sessionize(hm.selected.Path)
		}

		return nil
	},
}

var historyLastCmd = &cobra.Command{
	Use:     "last",
	Aliases: []string{"l"},
	Short:   "Switch to the most recently accessed project session",
	Long:    `Switches to the most recently used project session without showing the interactive UI.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := tmux.NewManager(configDir)
		if err != nil {
			return fmt.Errorf("failed to initialize manager: %w", err)
		}

		// Fetch all known projects to validate history
		allProjects, err := mgr.GetAvailableProjects()
		if err != nil {
			return fmt.Errorf("failed to get available projects: %w", err)
		}
		projectSet := make(map[string]struct{})
		for _, p := range allProjects {
			projectSet[p.Path] = struct{}{}
		}

		// Load and sort access history
		history, err := mgr.GetAccessHistory()
		if err != nil {
			return fmt.Errorf("failed to load access history: %w", err)
		}

		var historyAccesses []*workspace.ProjectAccess
		for _, access := range history.Projects {
			historyAccesses = append(historyAccesses, access)
		}
		sort.Slice(historyAccesses, func(i, j int) bool {
			return historyAccesses[i].LastAccessed.After(historyAccesses[j].LastAccessed)
		})

		if len(historyAccesses) == 0 {
			return fmt.Errorf("no session history found")
		}

		// Get current working directory to exclude it from results
		cwd, _ := os.Getwd()
		if cwd != "" {
			cwd = filepath.Clean(cwd)
		}

		// Find the most recent, valid project that is NOT the current one
		var latestProjectPath string
		for _, access := range historyAccesses {
			cleanPath := filepath.Clean(access.Path)
			// Skip if this is the current directory (case-insensitive comparison for macOS)
			if cwd != "" && strings.EqualFold(cleanPath, cwd) {
				continue
			}
			if _, ok := projectSet[access.Path]; ok {
				latestProjectPath = access.Path
				break
			}
		}

		if latestProjectPath == "" {
			return fmt.Errorf("no valid recent sessions found")
		}

		// Record access again to bump it to the top of the history
		_ = mgr.RecordProjectAccess(latestProjectPath)
		// Sessionize will create or switch to the tmux session
		return mgr.Sessionize(latestProjectPath)
	},
}

// TUI Model
type historyModel struct {
	items             []historyItem
	filteredItems     []historyItem
	cursor            int
	selected          *manager.SessionizeProject
	manager           *tmux.Manager
	keys              historyKeyMap
	help              help.Model
	quitting          bool
	enrichedProjects  map[string]*manager.SessionizeProject
	enrichmentLoading map[string]bool
	isLoading         bool
	spinnerFrame      int
	commandOnExit     *exec.Cmd
	keyMap            map[string]string // map[path]key
	filterMode        bool
	filterText        string
}

// TUI Keymap
type historyKeyMap struct {
	keymap.Base
	Up     key.Binding
	Down   key.Binding
	Open   key.Binding
	Filter key.Binding
}

func (k historyKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Open, k.Quit}
}

func (k historyKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Navigation")),
			k.Up,
			k.Down,
			key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1-9", "jump to row")),
		},
		{
			key.NewBinding(key.WithKeys(""), key.WithHelp("", "Actions")),
			k.Filter,
			k.Open,
			k.Help,
			k.Quit,
		},
	}
}

var historyKeys = historyKeyMap{
	Base: keymap.NewBase(),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Open: key.NewBinding(
		key.WithKeys("o", "enter"),
		key.WithHelp("enter/o", "switch to session"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
}

func newHistoryModel(items []historyItem, mgr *tmux.Manager, keyMap map[string]string) *historyModel {
	helpModel := help.NewBuilder().
		WithKeys(historyKeys).
		WithTitle("Session History - Help").
		Build()

	enrichedProjects := make(map[string]*manager.SessionizeProject)
	for _, item := range items {
		enrichedProjects[item.project.Path] = item.project
	}

	return &historyModel{
		items:             items,
		filteredItems:     items, // Initially show all items
		manager:           mgr,
		keys:              historyKeys,
		help:              helpModel,
		enrichedProjects:  enrichedProjects,
		enrichmentLoading: make(map[string]bool),
		isLoading:         true, // Start in loading state for enrichment
		keyMap:            keyMap,
	}
}

// applyFilter filters the items based on the filter text
func (m *historyModel) applyFilter() {
	if m.filterText == "" {
		m.filteredItems = m.items
		return
	}

	var filtered []historyItem
	filterLower := strings.ToLower(m.filterText)

	for _, item := range m.items {
		// Check if filter matches - prioritize repo name, then branch, then path, then ecosystem
		if strings.Contains(strings.ToLower(item.project.Name), filterLower) ||
			(item.project.GitStatus != nil && strings.Contains(strings.ToLower(item.project.GitStatus.StatusInfo.Branch), filterLower)) ||
			strings.Contains(strings.ToLower(item.project.Path), filterLower) ||
			(item.project.RootEcosystemPath != "" && strings.Contains(strings.ToLower(filepath.Base(item.project.RootEcosystemPath)), filterLower)) {
			filtered = append(filtered, item)
		}
	}

	m.filteredItems = filtered

	// Reset cursor if it's out of bounds
	if m.cursor >= len(m.filteredItems) {
		m.cursor = 0
	}
}

func (m *historyModel) Init() tea.Cmd {
	var projectList []*manager.SessionizeProject
	for _, item := range m.items {
		projectList = append(projectList, item.project)
	}

	m.enrichmentLoading["git"] = true

	return tea.Batch(
		fetchAllGitStatusesForKeyManageCmd(projectList),
		spinnerTickCmd(),
	)
}

func (m *historyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.help.SetSize(msg.Width, msg.Height)

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
		if m.help.ShowAll {
			m.help.Toggle()
			return m, nil
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

		// Handle number key navigation (1-9)
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= '1' && r <= '9' {
				num := int(r - '0')
				targetIndex := num - 1
				if targetIndex < len(m.filteredItems) {
					m.selected = m.filteredItems[targetIndex].project
					m.quitting = true
					return m, tea.Quit
				}
				return m, nil
			}
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
		}
	}

	return m, nil
}

func (m *historyModel) View() string {
	if m.quitting {
		return ""
	}
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder
	b.WriteString(core_theme.DefaultTheme.Header.Render("Session History"))
	if m.isLoading {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		b.WriteString(" " + spinner)
	}

	// Show filter text if in filter mode
	if m.filterMode {
		b.WriteString(" " + core_theme.DefaultTheme.Muted.Render("Filter: ") + m.filterText + "█")
	} else if m.filterText != "" {
		b.WriteString(" " + core_theme.DefaultTheme.Muted.Render("Filter: ") + m.filterText)
	}

	b.WriteString("\n\n")

	headers := []string{"#", "LAST ACCESSED", "Key", "Repository", "Branch/Worktree", "Git", "Ecosystem"}
	var rows [][]string

	for i, item := range m.filteredItems {
		var repository, worktree, gitStatus, ecosystem, key string

		projInfo := item.project

		// Look up the key mapping for this project
		if k, ok := m.keyMap[filepath.Clean(projInfo.Path)]; ok {
			key = k
		}
		if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
			repository = core_theme.DefaultTheme.Muted.Render(core_theme.IconRepo+" ") + filepath.Base(projInfo.ParentProjectPath)
			worktreeIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconWorktree + " ")
			worktree = worktreeIcon + projInfo.Name
		} else {
			icon := core_theme.IconRepo
			if projInfo.IsEcosystem() {
				icon = core_theme.IconEcosystem
			}
			repository = core_theme.DefaultTheme.Muted.Render(icon+" ") + projInfo.Name
		}

		// Determine Ecosystem display
		if projInfo.ParentEcosystemPath != "" {
			// Project is within an ecosystem - use the root ecosystem name
			if projInfo.RootEcosystemPath != "" {
				ecosystem = filepath.Base(projInfo.RootEcosystemPath)
			} else {
				ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
			}
		} else if projInfo.IsEcosystem() {
			// It's a root ecosystem
			ecosystem = projInfo.Name
		}

		branchWorktreeDisplay := worktree
		if branchWorktreeDisplay == "" && projInfo.GitStatus != nil && projInfo.GitStatus.StatusInfo.Branch != "" {
			branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
			branchWorktreeDisplay = branchIcon + projInfo.GitStatus.StatusInfo.Branch
		} else if branchWorktreeDisplay == "" {
			branchWorktreeDisplay = dimStyle.Render("n/a")
		}

		if projInfo.GitStatus != nil {
			gitStatus = formatChanges(projInfo.GitStatus.StatusInfo, projInfo.GitStatus)
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			formatRelativeTime(item.access.LastAccessed),
			key,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}

		// Apply muted style to old rows
		if time.Since(item.access.LastAccessed) > mutedThreshold {
			style := lipgloss.NewStyle().Faint(true)
			for i, cell := range row {
				// Don't mute the row number
				if i > 0 {
					row[i] = style.Render(cell)
				}
			}
		}

		rows = append(rows, row)
	}

	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{})
	b.WriteString(tableStr)
	b.WriteString("\n\n")
	b.WriteString(m.help.View())

	return pageStyle.Render(b.String())
}

// formatRelativeTime converts a time.Time to a human-readable string.
func formatRelativeTime(t time.Time) string {
	delta := time.Since(t)

	if delta < time.Minute {
		return fmt.Sprintf("%ds ago", int(delta.Seconds()))
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	}
	if delta < 7*24*time.Hour {
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
	return t.Format("2006-01-02")
}

func init() {
	historyCmd.AddCommand(historyLastCmd)
	rootCmd.AddCommand(historyCmd)
}
