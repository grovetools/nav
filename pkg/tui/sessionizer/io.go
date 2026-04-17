package sessionizer

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
	"github.com/grovetools/core/pkg/workspace"
	grovecontext "github.com/grovetools/cx/pkg/context"
	"github.com/grovetools/nav/pkg/api"
)

// gitStatusMsg is sent when git status for a single project is fetched.
type gitStatusMsg struct {
	path   string
	status *git.ExtendedGitStatus
}

// initialProjectsEnrichedMsg is sent after initial project data is loaded from session paths.
type initialProjectsEnrichedMsg struct {
	enrichedProjects map[string]*api.Project
	projectList      []*api.Project
}

// gitStatusMapMsg is sent when git statuses for multiple projects are fetched.
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

// projectsUpdateMsg is sent when the list of discovered projects is updated.
type projectsUpdateMsg struct {
	projects []*api.Project
}

// runningSessionsUpdateMsg is sent with the latest list of active sessions.
type runningSessionsUpdateMsg struct {
	sessions map[string]bool
}

// keyMapUpdateMsg is sent when key mappings are reloaded.
type keyMapUpdateMsg struct {
	keyMap   map[string]string
	sessions []models.TmuxSession
}

type rulesStateUpdateMsg struct {
	rulesState map[string]grovecontext.RuleStatus
}

type ruleToggleResultMsg struct {
	err error
}

type daemonStateUpdateMsg struct {
	update daemon.StateUpdate
}

type daemonStreamErrorMsg struct {
	err error
}

// daemonStreamConnectedMsg is dispatched once subscribeToDaemonCmd has
// successfully opened an SSE stream. The channel and cancel func are stored
// on the Model so the stream can be torn down when the host closes the
// sessionizer (multiple embedded instances must not share state).
type daemonStreamConnectedMsg struct {
	ch     <-chan daemon.StateUpdate
	cancel context.CancelFunc
}

// statusMsg is a transient status line update.
type statusMsg struct {
	message string
}

// RequestManageGroupsMsg is emitted when the user asks to open the groups
// management view. Hosts that embed the sessionizer should translate this
// into whatever view-switching mechanism they use.
type RequestManageGroupsMsg struct{}

// RequestMapKeyMsg is emitted when the user asks to map a project to a
// tmux key binding. Hosts should route this to their key manage view.
type RequestMapKeyMsg struct {
	Project *api.Project
}

// BulkMappingDoneMsg is emitted after the sessionizer bulk-maps a set of
// selected projects to keys. Hosts typically switch to the key manage view
// and highlight the new mappings.
type BulkMappingDoneMsg struct {
	MappedKeys []string
}

// fetchRulesStateCmd loads the context rules and determines the status for each project path.
func fetchRulesStateCmd(projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		mgr := grovecontext.NewManager("")
		rulesState := make(map[string]grovecontext.RuleStatus)

		gitRules, _ := mgr.ListGitRules()

		for _, project := range projects {
			var status grovecontext.RuleStatus

			if project.RepoShorthand != "" && len(gitRules) > 0 {
				expectedRepoURL := "https://github.com/" + project.RepoShorthand
				projectHeadCommit, _ := git.GetHeadCommit(project.Path)

				projectVersion := project.Version
				if project.GitStatus != nil && project.GitStatus.Branch != "" {
					projectVersion = project.GitStatus.Branch
				} else if project.IsWorktree() && project.Name != "" {
					projectVersion = project.Name
				}
				if projectVersion == "" {
					projectVersion = "main"
				}

				for _, rule := range gitRules {
					if grovecontext.MatchesGitRule(rule, expectedRepoURL, projectVersion, projectHeadCommit, project.Path) {
						status = rule.ContextType
						break
					}
				}
			}

			if status == 0 && project.RootEcosystemPath != "" {
				ecosystemName := filepath.Base(project.RootEcosystemPath)
				if project.ParentEcosystemPath != "" && project.ParentEcosystemPath != project.RootEcosystemPath {
					ecosystemName = filepath.Base(project.ParentEcosystemPath)
				}
				workspaceName := project.Name
				ruleName := mgr.GetDefaultRuleName()
				if ruleName == "" {
					ruleName = "default"
				}
				aliasRule := fmt.Sprintf("@a:%s:%s::%s", ecosystemName, workspaceName, ruleName)
				status = mgr.GetRuleStatus(aliasRule)
			}

			if status == 0 {
				pathRule := filepath.Join(project.Path, "**")
				status = mgr.GetRuleStatus(pathRule)
			}

			rulesState[project.Path] = status
		}
		return rulesStateUpdateMsg{rulesState: rulesState}
	}
}

func toggleRuleCmd(project *api.Project, action string, currentStatus grovecontext.RuleStatus) tea.Cmd {
	return func() tea.Msg {
		if project == nil {
			return ruleToggleResultMsg{err: fmt.Errorf("no project selected")}
		}
		mgr := grovecontext.NewManager("")

		var rule string

		if project.RepoShorthand != "" {
			version := project.Version
			if version == "" {
				version = "main"
			}
			ruleset := "default"
			rule = fmt.Sprintf("@a:git:%s@%s::%s", project.RepoShorthand, version, ruleset)
		} else if project.RootEcosystemPath != "" {
			ecosystemName := filepath.Base(project.RootEcosystemPath)
			if project.ParentEcosystemPath != "" && project.ParentEcosystemPath != project.RootEcosystemPath {
				ecosystemName = filepath.Base(project.ParentEcosystemPath)
			}
			workspaceName := project.Name

			ruleName := mgr.GetDefaultRuleName()
			if ruleName == "" {
				ruleName = "default"
			}

			rule = fmt.Sprintf("@a:%s:%s::%s", ecosystemName, workspaceName, ruleName)
		} else {
			rule = filepath.Join(project.Path, "**")
		}

		var targetStatus grovecontext.RuleStatus
		switch action {
		case "hot":
			targetStatus = grovecontext.RuleHot
		case "cold":
			targetStatus = grovecontext.RuleCold
		case "exclude":
			targetStatus = grovecontext.RuleExcluded
		}

		if currentStatus == targetStatus {
			if err := mgr.RemoveRule(rule); err != nil {
				return ruleToggleResultMsg{err: err}
			}
			return ruleToggleResultMsg{err: nil}
		}

		if err := mgr.AppendRule(rule, action); err != nil {
			return ruleToggleResultMsg{err: err}
		}

		return ruleToggleResultMsg{err: nil}
	}
}

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

// fetchProjectsCmd reloads the project list via the loader supplied on Config.
// If the daemon is running, it also triggers a daemon refresh first so the
// daemon re-discovers workspaces and broadcasts the update via SSE.
func fetchProjectsCmd(dir string, loader ProjectLoader) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		if client.IsRunning() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = client.Refresh(ctx)
			cancel()
		}
		client.Close()

		if loader == nil {
			return projectsUpdateMsg{projects: nil}
		}
		projects, _ := loader()
		return projectsUpdateMsg{projects: projects}
	}
}

// reloadProjectsCmd loads projects without triggering a daemon refresh.
func reloadProjectsCmd(loader ProjectLoader) tea.Cmd {
	return func() tea.Msg {
		if loader == nil {
			return projectsUpdateMsg{projects: nil}
		}
		projects, _ := loader()
		return projectsUpdateMsg{projects: projects}
	}
}

func fetchGitStatusCmd(path string) tea.Cmd {
	return func() tea.Msg {
		status, _ := git.GetExtendedStatus(path)
		return gitStatusMsg{path: path, status: status}
	}
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

func fetchAllNoteCountsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		counts, _ := client.GetNoteCounts(ctx)
		return noteCountsMapMsg{counts: counts}
	}
}

func fetchAllPlanStatsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stats, _ := client.GetPlanStats(ctx)
		return planStatsMapMsg{stats: stats}
	}
}

func fetchAllReleaseInfoCmd(dir string, projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workspaces, _ := client.GetEnrichedWorkspaces(ctx, &models.EnrichmentOptions{FetchReleaseInfo: true})

		releases := make(map[string]*models.ReleaseInfo)
		for _, ws := range workspaces {
			if ws.ReleaseInfo != nil {
				releases[ws.Path] = ws.ReleaseInfo
			}
		}
		return releaseInfoMapMsg{releases: releases}
	}
}

func fetchAllBinaryStatusCmd(dir string, projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workspaces, _ := client.GetEnrichedWorkspaces(ctx, &models.EnrichmentOptions{FetchBinaryStatus: true})

		statuses := make(map[string]*models.BinaryStatus)
		for _, ws := range workspaces {
			if ws.ActiveBinary != nil {
				statuses[ws.Path] = ws.ActiveBinary
			}
		}
		return binaryStatusMapMsg{statuses: statuses}
	}
}

func fetchCxPerLineStatsCmd(dir string, projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workspaces, _ := client.GetEnrichedWorkspaces(ctx, &models.EnrichmentOptions{FetchCxStats: true})

		stats := make(map[string]*models.CxStats)
		for _, ws := range workspaces {
			if ws.CxStats != nil {
				stats[ws.Path] = ws.CxStats
			}
		}
		return cxStatsMapMsg{stats: stats}
	}
}

func fetchAllRemoteURLsCmd(dir string, projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workspaces, _ := client.GetEnrichedWorkspaces(ctx, &models.EnrichmentOptions{FetchRemoteURL: true})

		urls := make(map[string]string)
		for _, ws := range workspaces {
			if ws.GitRemoteURL != "" {
				urls[ws.Path] = ws.GitRemoteURL
			}
		}
		return remoteURLMapMsg{urls: urls}
	}
}

// fetchRunningSessionsCmd asks the SessionStateProvider for the active set.
func fetchRunningSessionsCmd(state SessionStateProvider) tea.Cmd {
	return func() tea.Msg {
		sessionsMap := make(map[string]bool)
		if state != nil {
			ctx := context.Background()
			names, _ := state.ListActive(ctx)
			for _, name := range names {
				sessionsMap[name] = true
			}
		}
		return runningSessionsUpdateMsg{sessions: sessionsMap}
	}
}

// fetchKeyMapCmd reloads sessions via Store and rebuilds the path→key map.
func fetchKeyMapCmd(store Store) tea.Cmd {
	return func() tea.Msg {
		keyMap := make(map[string]string)
		sessions, err := store.GetSessions()
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

// enrichInitialProjectsCmd populates session-mapped paths into a project map
// using workspace.GetProjectByPath. Used by other TUIs (key manage); the
// sessionizer keeps it here so its message types stay self-contained.
func enrichInitialProjectsCmd(sessions []models.TmuxSession, cachedProjects map[string]*api.Project) tea.Cmd {
	return func() tea.Msg {
		enrichedProjects := make(map[string]*api.Project)
		var projectList []*api.Project

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
					proj := &api.Project{WorkspaceNode: node}
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

// subscribeToDaemonCmd opens an SSE stream to the daemon and returns the
// channel + cancel function as a daemonStreamConnectedMsg. The host model
// owns the lifecycle and tears it down via Close().
func subscribeToDaemonCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)

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

		return daemonStreamConnectedMsg{ch: ch, cancel: cancel}
	}
}

func listenToDaemonCmd(ch <-chan daemon.StateUpdate) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
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

func updateDaemonFocusCmd(dir string, paths []string) tea.Cmd {
	sort.Strings(paths)
	key := strings.Join(paths, "\x00")
	if key == lastFocusPaths {
		return nil
	}
	lastFocusPaths = key

	return func() tea.Msg {
		client := daemon.NewWithAutoStart(dir)
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

// expandPath expands a leading ~/ in a path. Duplicated here so the
// sessionizer package has no cmd/nav dependency.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
