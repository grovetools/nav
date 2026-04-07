package main

// This file contains the shared message types, helper commands, and small
// formatting helpers that the remaining cmd/nav TUIs (key manage, history)
// used to import from the extracted sessionizer files. The sessionizer
// package keeps its own private copies of these types; we duplicate them
// here because the other TUIs haven't been extracted yet (commit 5).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
)

// buildProjectLoader returns a sessionizer.ProjectLoader that talks to the
// nav *tmux.Manager: it fetches projects, sorts by access history, applies
// the cloned-repo virtual ecosystem grouping, saves the project cache, and
// returns the pointer slice the sessionizer TUI expects. The standalone nav
// binary supplies this loader at TUI construction time; terminal will supply
// its own implementation when it embeds the package.
func buildProjectLoader(mgr *tmux.Manager, configDir string) sessionizer.ProjectLoader {
	return func() ([]*api.Project, error) {
		projects, err := mgr.GetAvailableProjects()
		if err != nil {
			return nil, err
		}

		if history, histErr := mgr.GetAccessHistory(); histErr == nil {
			projects = manager.SortProjectsByAccess(history, projects)
		}

		projects = groupClonedProjectsAsEcosystem(projects)

		_ = api.SaveProjectCache(configDir, projects)

		ptrs := make([]*api.Project, len(projects))
		for i := range projects {
			ptrs[i] = &projects[i]
		}
		return ptrs, nil
	}
}

// ----- Message types used by the remaining cmd/nav TUIs ---------------------

// gitStatusMsg is sent when git status for a single project is fetched.
type gitStatusMsg struct {
	path   string
	status *git.ExtendedGitStatus
}

type initialProjectsEnrichedMsg struct {
	enrichedProjects map[string]*manager.SessionizeProject
	projectList      []*manager.SessionizeProject
}

type gitStatusMapMsg struct {
	statuses map[string]*git.ExtendedGitStatus
}
type noteCountsMapMsg struct {
	counts map[string]*models.NoteCounts
}
type planStatsMapMsg struct {
	stats map[string]*models.PlanStats
}

type releaseInfoMapMsg struct{ releases map[string]*models.ReleaseInfo }
type binaryStatusMapMsg struct{ statuses map[string]*models.BinaryStatus }
type cxStatsMapMsg struct{ stats map[string]*models.CxStats }
type remoteURLMapMsg struct{ urls map[string]string }

type tickMsg time.Time
type spinnerTickMsg time.Time

type projectsUpdateMsg struct {
	projects []*manager.SessionizeProject
}

type runningSessionsUpdateMsg struct {
	sessions map[string]bool
}

type keyMapUpdateMsg struct {
	keyMap   map[string]string
	sessions []models.TmuxSession
}

type daemonStateUpdateMsg struct {
	update daemon.StateUpdate
}

type daemonStreamErrorMsg struct {
	err error
}

type daemonStreamStartedMsg struct{}

type statusMsg struct {
	message string
}

type jumpToMappingMsg struct {
	path string
}

type jumpToSessionizeMsg struct {
	path             string
	applyGroupFilter bool
}

type focusCwdEcosystemMsg struct{}

// ----- Async commands --------------------------------------------------------

func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func fetchGitStatusCmd(path string) tea.Cmd {
	return func() tea.Msg {
		status, _ := git.GetExtendedStatus(path)
		return gitStatusMsg{path: path, status: status}
	}
}

func fetchAllGitStatusesCmd(projects []*manager.SessionizeProject) tea.Cmd {
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
			go func(proj *manager.SessionizeProject) {
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

func fetchAllNoteCountsCmd() tea.Cmd {
	return func() tea.Msg {
		counts, _ := manager.FetchNoteCountsMap()
		return noteCountsMapMsg{counts: counts}
	}
}

func fetchAllPlanStatsCmd() tea.Cmd {
	return func() tea.Msg {
		stats, _ := manager.FetchPlanStatsMap()
		return planStatsMapMsg{stats: stats}
	}
}

func fetchRunningSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessionsMap := make(map[string]bool)
		if os.Getenv("TMUX") != "" {
			client, err := tmuxclient.NewClient()
			if err == nil {
				ctx := context.Background()
				sessionNames, _ := client.ListSessions(ctx)
				for _, name := range sessionNames {
					sessionsMap[name] = true
				}
			}
		}
		return runningSessionsUpdateMsg{sessions: sessionsMap}
	}
}

func fetchKeyMapCmd(mgr *tmux.Manager) tea.Cmd {
	return func() tea.Msg {
		keyMap := make(map[string]string)
		sessions, err := mgr.GetSessions()
		if err != nil {
			sessions = []models.TmuxSession{}
		}
		for _, s := range sessions {
			if s.Path != "" {
				expandedPath := expandPath(s.Path)
				absPath, err := filepath.Abs(expandedPath)
				if err == nil {
					cleanPath := filepath.Clean(absPath)
					keyMap[cleanPath] = s.Key
				}
			}
		}
		return keyMapUpdateMsg{keyMap: keyMap, sessions: sessions}
	}
}

func clearStatusCmd(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return statusMsg{message: ""}
	})
}

// enrichInitialProjectsCmd gets WorkspaceNode info for all mapped sessions.
func enrichInitialProjectsCmd(sessions []models.TmuxSession, cachedProjects map[string]*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		enrichedProjects := make(map[string]*manager.SessionizeProject)
		var projectList []*manager.SessionizeProject

		for path, proj := range cachedProjects {
			enrichedProjects[path] = proj
		}

		for _, s := range sessions {
			if s.Path == "" {
				continue
			}
			expandedPath := expandPath(s.Path)
			cleanPath, err := filepath.Abs(expandedPath)
			if err != nil {
				continue
			}
			cleanPath = filepath.Clean(cleanPath)

			if _, exists := enrichedProjects[cleanPath]; !exists {
				node, err := workspace.GetProjectByPath(s.Path)
				if err == nil {
					proj := &manager.SessionizeProject{WorkspaceNode: node}
					enrichedProjects[cleanPath] = proj
				}
			}
		}

		for _, proj := range enrichedProjects {
			projectList = append(projectList, proj)
		}

		return initialProjectsEnrichedMsg{
			enrichedProjects: enrichedProjects,
			projectList:      projectList,
		}
	}
}

// ----- Daemon streaming shared state ----------------------------------------

var daemonStreamState struct {
	mu      sync.Mutex
	ch      <-chan daemon.StateUpdate
	cancel  context.CancelFunc
	started bool
}

func subscribeToDaemonCmd() tea.Cmd {
	return func() tea.Msg {
		daemonStreamState.mu.Lock()
		defer daemonStreamState.mu.Unlock()

		if daemonStreamState.started {
			return daemonStreamStartedMsg{}
		}

		client := daemon.New()
		if !client.IsRunning() {
			client.Close()
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		ch, err := client.StreamState(ctx)
		if err != nil {
			cancel()
			client.Close()
			return daemonStreamErrorMsg{err: err}
		}

		daemonStreamState.ch = ch
		daemonStreamState.cancel = cancel
		daemonStreamState.started = true

		return daemonStreamStartedMsg{}
	}
}

func listenToDaemonCmd() tea.Cmd {
	return func() tea.Msg {
		daemonStreamState.mu.Lock()
		ch := daemonStreamState.ch
		started := daemonStreamState.started
		daemonStreamState.mu.Unlock()

		if !started || ch == nil {
			return nil
		}

		update, ok := <-ch
		if !ok {
			return daemonStreamErrorMsg{err: nil}
		}

		return daemonStateUpdateMsg{update: update}
	}
}

var lastFocusPaths string

func updateDaemonFocusCmd(paths []string) tea.Cmd {
	sort.Strings(paths)
	key := strings.Join(paths, "\x00")
	if key == lastFocusPaths {
		return nil
	}
	lastFocusPaths = key

	return func() tea.Msg {
		client := daemon.New()
		defer client.Close()

		if !client.IsRunning() {
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = client.SetFocus(ctx, paths)
		return nil
	}
}

// ----- Formatting helpers ----------------------------------------------------

// formatChanges formats the git status into a styled string. Used by the
// manage and history TUIs to display a compact per-repo change summary.
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

// Avoid unused-import warnings if only some symbols end up referenced.
var (
	_ = sessionizer.RequestManageGroupsMsg{}
)
