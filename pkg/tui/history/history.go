// Package history hosts the extracted nav session-history TUI. It depends
// only on the small Store interface defined here plus core/pkg/git and
// core/pkg/workspace, so it can be embedded by any host that supplies a
// list of history items. Standalone nav supplies the items from
// *tmux.Manager's access history.
package history

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
	navkeymap "github.com/grovetools/nav/pkg/keymap"
	"github.com/grovetools/nav/pkg/api"
)

// KeyMap is the alias used inside this package for the history keymap.
type KeyMap = navkeymap.HistoryKeyMap

// Item holds a project and its last access time. Hosts supply a slice of
// these when constructing the model.
type Item struct {
	Project *api.Project
	Access  *workspace.ProjectAccess
}

// JumpToSessionizeMsg is emitted when the user asks to jump from the
// history view into the sessionize view focused on the currently selected
// project. Hosts are responsible for catching this message and switching
// views.
type JumpToSessionizeMsg struct {
	Path             string
	ApplyGroupFilter bool
}

const mutedThreshold = 7 * 24 * time.Hour // 1 week

var (
	pageStyle = lipgloss.NewStyle()
	dimStyle  = core_theme.DefaultTheme.Muted
)

// Model is the exported history TUI model. New() constructs one.
type Model = historyModel

// New constructs a Model. The host supplies the list of history items, the
// current key -> path mapping (used to render the Key column), and the
// keymap. The manager field is retained for future use but not required by
// the current model.
func New(items []Item, keyMap map[string]string, keys KeyMap) *Model {
	helpModel := help.NewBuilder().
		WithKeys(keys).
		WithTitle("Session History - Help").
		Build()

	internal := make([]historyItem, len(items))
	enrichedProjects := make(map[string]*api.Project, len(items))
	for i, it := range items {
		internal[i] = historyItem{project: it.Project, access: it.Access}
		enrichedProjects[it.Project.Path] = it.Project
	}

	return &historyModel{
		items:             internal,
		filteredItems:     internal,
		keys:              keys,
		help:              helpModel,
		enrichedProjects:  enrichedProjects,
		enrichmentLoading: make(map[string]bool),
		isLoading:         true,
		keyMap:            keyMap,
	}
}

// Selected returns the project the user picked, or nil if no selection
// was made (quit without choosing).
func (m *Model) Selected() *api.Project { return m.selected }

// Quitting reports whether the model has quit.
func (m *Model) Quitting() bool { return m.quitting }

// FilterMode reports whether the model is currently in text-filter mode.
func (m *Model) FilterMode() bool { return m.filterMode }

// historyItem is the internal form of Item used by the model.
type historyItem struct {
	project *api.Project
	access  *workspace.ProjectAccess
}

type historyModel struct {
	items             []historyItem
	filteredItems     []historyItem
	cursor            int
	selected          *api.Project
	keys              KeyMap
	help              help.Model
	quitting          bool
	enrichedProjects  map[string]*api.Project
	enrichmentLoading map[string]bool
	isLoading         bool
	spinnerFrame      int
	keyMap            map[string]string // map[path]key
	filterMode        bool
	filterText        string
	statusMessage     string
	jumpMode          bool // Mini-leader: 'g' pressed, waiting for digit or 'g' for go-to-top
}

// --- Async commands (package-private copies of the shared helpers) ----------

type gitStatusMapMsg struct {
	statuses map[string]*git.ExtendedGitStatus
}

type spinnerTickMsg time.Time

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func fetchAllGitStatusesCmd(projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := make(map[string]*git.ExtendedGitStatus)
		semaphore := make(chan struct{}, 10)

		for _, p := range projects {
			if p.GitStatus != nil {
				mu.Lock()
				statuses[p.Path] = p.GitStatus
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func(proj *api.Project) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				status, err := git.GetExtendedStatus(proj.Path)
				if err == nil {
					mu.Lock()
					statuses[proj.Path] = status
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
		return gitStatusMapMsg{statuses: statuses}
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
		if strings.Contains(strings.ToLower(item.project.Name), filterLower) ||
			(item.project.GitStatus != nil && strings.Contains(strings.ToLower(item.project.GitStatus.StatusInfo.Branch), filterLower)) ||
			strings.Contains(strings.ToLower(item.project.Path), filterLower) ||
			(item.project.RootEcosystemPath != "" && strings.Contains(strings.ToLower(filepath.Base(item.project.RootEcosystemPath)), filterLower)) {
			filtered = append(filtered, item)
		}
	}

	m.filteredItems = filtered

	if m.cursor >= len(m.filteredItems) {
		m.cursor = 0
	}
}

func (m *historyModel) Init() tea.Cmd {
	var projectList []*api.Project
	for _, item := range m.items {
		projectList = append(projectList, item.project)
	}

	m.enrichmentLoading["git"] = true

	return tea.Batch(
		fetchAllGitStatusesCmd(projectList),
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
			m.jumpMode = false // Reset mode immediately
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

		// Enter jumpMode when 'g' is pressed (and not in filter mode)
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
				return m, func() tea.Msg {
					return JumpToSessionizeMsg{Path: m.filteredItems[m.cursor].project.Path, ApplyGroupFilter: true}
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.FocusCurrent):
			if m.cursor < len(m.filteredItems) {
				return m, func() tea.Msg {
					return JumpToSessionizeMsg{Path: m.filteredItems[m.cursor].project.Path, ApplyGroupFilter: false}
				}
			}
			return m, nil
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
		var repository, worktree, gitStatus, ecosystem, k string

		projInfo := item.project

		if v, ok := m.keyMap[filepath.Clean(projInfo.Path)]; ok {
			k = v
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

		if projInfo.ParentEcosystemPath != "" {
			if projInfo.RootEcosystemPath != "" {
				ecosystem = filepath.Base(projInfo.RootEcosystemPath)
			} else {
				ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
			}
		} else if projInfo.IsEcosystem() {
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
			k,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}

		if time.Since(item.access.LastAccessed) > mutedThreshold {
			style := lipgloss.NewStyle().Faint(true)
			for j, cell := range row {
				if j > 0 {
					row[j] = style.Render(cell)
				}
			}
		}

		rows = append(rows, row)
	}

	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{})
	b.WriteString(tableStr)
	b.WriteString("\n\n")
	if m.statusMessage != "" {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(m.statusMessage) + "\n")
	}
	b.WriteString(m.help.View())
	if m.jumpMode {
		b.WriteString(core_theme.DefaultTheme.Warning.Render(" [GOTO: _]"))
	}

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

// formatChanges formats the git status into a styled string. Mirrors the
// version in cmd/nav/tui_shared.go but lives here so the package is
// self-contained.
func formatChanges(status *git.StatusInfo, extStatus *git.ExtendedGitStatus) string {
	if status == nil {
		return ""
	}
	var changes []string

	isMainBranch := status.Branch == "main" || status.Branch == "master"
	hasMainDivergence := !isMainBranch && (status.AheadMainCount > 0 || status.BehindMainCount > 0)

	if hasMainDivergence {
		if status.AheadMainCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s%d", core_theme.IconArrowUp, status.AheadMainCount)))
		}
		if status.BehindMainCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("%s%d", core_theme.IconArrowDown, status.BehindMainCount)))
		}
	} else if status.HasUpstream {
		if status.AheadCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s%d", core_theme.IconArrowUp, status.AheadCount)))
		}
		if status.BehindCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("%s%d", core_theme.IconArrowDown, status.BehindCount)))
		}
	}

	if status.ModifiedCount > 0 {
		changes = append(changes, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("M:%d", status.ModifiedCount)))
	}
	if status.StagedCount > 0 {
		changes = append(changes, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("S:%d", status.StagedCount)))
	}
	if status.UntrackedCount > 0 {
		changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("?:%d", status.UntrackedCount)))
	}

	if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
		if extStatus.LinesAdded > 0 {
			changes = append(changes, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("+%d", extStatus.LinesAdded)))
		}
		if extStatus.LinesDeleted > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("-%d", extStatus.LinesDeleted)))
		}
	}

	changesStr := strings.Join(changes, " ")

	if !status.IsDirty && changesStr == "" {
		if status.HasUpstream {
			return core_theme.DefaultTheme.Success.Render(core_theme.IconSuccess)
		}
		return core_theme.DefaultTheme.Success.Render(core_theme.IconStatusTodo)
	}

	return changesStr
}
