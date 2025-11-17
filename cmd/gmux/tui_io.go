package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	grovecontext "github.com/mattsolo1/grove-context/pkg/context"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
)

// gitStatusMsg is sent when git status for a single project is fetched.
type gitStatusMsg struct {
	path   string
	status *manager.ExtendedGitStatus
}

// initialProjectsEnrichedMsg is sent after initial project data is loaded from session paths.
type initialProjectsEnrichedMsg struct {
	enrichedProjects map[string]*manager.SessionizeProject
	projectList      []*manager.SessionizeProject
}

// gitStatusMapMsg is sent when git statuses for multiple projects are fetched.
type gitStatusMapMsg struct {
	statuses map[string]*manager.ExtendedGitStatus
}

// claudeSessionMapMsg is sent when all active Claude sessions are fetched.
type claudeSessionMapMsg struct {
	sessions map[string]*manager.ClaudeSessionInfo
}

// noteCountsMapMsg is sent when all note counts are fetched.
type noteCountsMapMsg struct {
	counts map[string]*manager.NoteCounts
}

// planStatsMapMsg is sent when all plan stats are fetched.
type planStatsMapMsg struct {
	stats map[string]*manager.PlanStats
}

// tickMsg is sent periodically to refresh git status
type tickMsg time.Time

// spinnerTickMsg is sent frequently to animate the spinner
type spinnerTickMsg time.Time

// projectsUpdateMsg is sent when the list of discovered projects is updated
type projectsUpdateMsg struct {
	projects []*manager.SessionizeProject
}

// runningSessionsUpdateMsg is sent with the latest list of running tmux sessions
type runningSessionsUpdateMsg struct {
	sessions map[string]bool // A set of session names for quick lookups
}

// keyMapUpdateMsg is sent when the key mappings from tmux-sessions.yaml are reloaded
type keyMapUpdateMsg struct {
	keyMap   map[string]string     // map[path]key
	sessions []models.TmuxSession // Also pass the full session list
}

// rulesStateUpdateMsg is sent when the context rules have been parsed.
type rulesStateUpdateMsg struct {
	rulesState map[string]grovecontext.RuleStatus
}

// ruleToggleResultMsg is sent after a rule is toggled.
type ruleToggleResultMsg struct {
	err error
}

// fetchRulesStateCmd loads the context rules and determines the status for each project path.
func fetchRulesStateCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		mgr := grovecontext.NewManager("") // Use CWD
		rulesState := make(map[string]grovecontext.RuleStatus)

		for _, project := range projects {
			// Check for alias rule first if this is part of an ecosystem
			var status grovecontext.RuleStatus
			if project.RootEcosystemPath != "" {
				// Use the immediate parent ecosystem (worktree if applicable, otherwise root)
				ecosystemName := filepath.Base(project.RootEcosystemPath)
				if project.ParentEcosystemPath != "" && project.ParentEcosystemPath != project.RootEcosystemPath {
					// This is inside an ecosystem worktree, use the worktree name
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

			// If no alias status found, check for path-based rule
			if status == 0 {
				pathRule := filepath.Join(project.Path, "**")
				status = mgr.GetRuleStatus(pathRule)
			}

			rulesState[project.Path] = status
		}
		return rulesStateUpdateMsg{rulesState: rulesState}
	}
}

// toggleRuleCmd adds or removes a context rule for a given project.
func toggleRuleCmd(project *manager.SessionizeProject, action string) tea.Cmd {
	return func() tea.Msg {
		if project == nil {
			return ruleToggleResultMsg{err: fmt.Errorf("no project selected")}
		}
		mgr := grovecontext.NewManager("")

		// Construct alias rule using the project's ecosystem and workspace info
		rule := filepath.Join(project.Path, "**")

		// If this is part of an ecosystem, construct an alias
		if project.RootEcosystemPath != "" {
			// Use the immediate parent ecosystem (worktree if applicable, otherwise root)
			ecosystemName := filepath.Base(project.RootEcosystemPath)
			if project.ParentEcosystemPath != "" && project.ParentEcosystemPath != project.RootEcosystemPath {
				// This is inside an ecosystem worktree, use the worktree name
				ecosystemName = filepath.Base(project.ParentEcosystemPath)
			}
			workspaceName := project.Name

			// Get the default rule name from grove.yml
			ruleName := mgr.GetDefaultRuleName()
			if ruleName == "" {
				ruleName = "default"
			}

			// Construct alias: @a:ecosystem:workspace::rule
			rule = fmt.Sprintf("@a:%s:%s::%s", ecosystemName, workspaceName, ruleName)
		}

		if err := mgr.AppendRule(rule, action); err != nil {
			return ruleToggleResultMsg{err: err}
		}

		return ruleToggleResultMsg{err: nil}
	}
}

// getWorktreeParent checks if a path is a Git worktree and returns the parent path
func getWorktreeParent(path string) string {
	// Check if this is inside a .grove-worktrees directory
	if strings.Contains(path, ".grove-worktrees") {
		parts := strings.Split(path, ".grove-worktrees")
		if len(parts) >= 1 {
			return parts[0]
		}
	}
	return ""
}

// tickCmd returns a command that sends a tick message after a delay
func tickCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// spinnerTickCmd returns a command that sends a spinner tick message quickly (for animation)
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// fetchAllClaudeSessionsCmd returns a command that fetches all active Claude sessions.
func fetchAllClaudeSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, _ := manager.FetchClaudeSessionMap()
		return claudeSessionMapMsg{sessions: sessions}
	}
}

// fetchProjectsCmd returns a command that re-scans configured search paths.
// This command only performs discovery and does NOT fetch enrichment data.
func fetchProjectsCmd(mgr *tmux.Manager, configDir string) tea.Cmd {
	return func() tea.Msg {
		projects, _ := mgr.GetAvailableProjects()

		// Sort by access history
		if history, err := mgr.GetAccessHistory(); err == nil {
			projects = manager.SortProjectsByAccess(history, projects)
		}

		// Convert to pointers
		projectPtrs := make([]*manager.SessionizeProject, len(projects))
		for i := range projects {
			projectPtrs[i] = &projects[i]
		}

		// Save to cache for next startup
		_ = manager.SaveProjectCache(configDir, projects)

		return projectsUpdateMsg{projects: projectPtrs}
	}
}

// fetchGitStatusCmd returns a command to fetch git status for a single path.
func fetchGitStatusCmd(path string) tea.Cmd {
	return func() tea.Msg {
		status, _ := manager.FetchGitStatusForPath(path)
		return gitStatusMsg{path: path, status: status}
	}
}

// fetchAllGitStatusesCmd returns a command to fetch git status for multiple paths concurrently.
func fetchAllGitStatusesCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := make(map[string]*manager.ExtendedGitStatus)
		semaphore := make(chan struct{}, 10) // Limit to 10 concurrent git processes

		for _, p := range projects {
			wg.Add(1)
			go func(proj *manager.SessionizeProject) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				status, err := manager.FetchGitStatusForPath(proj.Path)
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

// fetchAllNoteCountsCmd returns a command to fetch all note counts.
func fetchAllNoteCountsCmd() tea.Cmd {
	return func() tea.Msg {
		counts, _ := manager.FetchNoteCountsMap()
		return noteCountsMapMsg{counts: counts}
	}
}

// fetchAllPlanStatsCmd returns a command to fetch all plan stats.
func fetchAllPlanStatsCmd() tea.Cmd {
	return func() tea.Msg {
		stats, _ := manager.FetchPlanStatsMap()
		return planStatsMapMsg{stats: stats}
	}
}

// fetchRunningSessionsCmd returns a command that gets the list of currently running tmux sessions
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

// fetchKeyMapCmd returns a command that reloads the tmux-sessions.yaml file
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

// statusMsg represents a temporary status message to show to the user
type statusMsg struct {
	message string
}

// clearStatusCmd returns a command that clears the status message after a delay
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

		// Copy cached projects first to avoid re-analyzing paths
		for path, proj := range cachedProjects {
			enrichedProjects[path] = proj
		}

		for _, s := range sessions {
			if s.Path == "" {
				continue
			}

			// Use expanded and cleaned path as the key
			expandedPath := expandPath(s.Path)
			cleanPath, err := filepath.Abs(expandedPath)
			if err != nil {
				continue // Skip if path is invalid
			}
			cleanPath = filepath.Clean(cleanPath)

			if _, exists := enrichedProjects[cleanPath]; !exists {
				// Not in cache, so we need to get its info
				node, err := workspace.GetProjectByPath(s.Path)
				if err == nil {
					proj := &manager.SessionizeProject{WorkspaceNode: node}
					enrichedProjects[cleanPath] = proj
				}
			}
		}

		// Create list from map
		for _, proj := range enrichedProjects {
			projectList = append(projectList, proj)
		}

		return initialProjectsEnrichedMsg{
			enrichedProjects: enrichedProjects,
			projectList:      projectList,
		}
	}
}
