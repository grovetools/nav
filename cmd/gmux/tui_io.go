package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	grovecontext "github.com/mattsolo1/grove-context/pkg/context"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/models"
	tmuxclient "github.com/mattsolo1/grove-core/pkg/tmux"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-tmux/internal/manager"
	"github.com/mattsolo1/grove-tmux/pkg/tmux"
	"gopkg.in/yaml.v3"
)

// gitStatusMsg is sent when git status for a single project is fetched.
type gitStatusMsg struct {
	path   string
	status *git.ExtendedGitStatus
}

// initialProjectsEnrichedMsg is sent after initial project data is loaded from session paths.
type initialProjectsEnrichedMsg struct {
	enrichedProjects map[string]*manager.SessionizeProject
	projectList      []*manager.SessionizeProject
}

// gitStatusMapMsg is sent when git statuses for multiple projects are fetched.
type gitStatusMapMsg struct {
	statuses map[string]*git.ExtendedGitStatus
}

// noteCountsMapMsg is sent when all note counts are fetched.
type noteCountsMapMsg struct {
	counts map[string]*manager.NoteCounts
}

// planStatsMapMsg is sent when all plan stats are fetched.
type planStatsMapMsg struct {
	stats map[string]*manager.PlanStats
}

// New message types for additional column data
type releaseInfoMapMsg struct{ releases map[string]*manager.ReleaseInfo }
type binaryStatusMapMsg struct{ statuses map[string]*manager.BinaryStatus }
type cxStatsMapMsg struct{ stats map[string]*manager.CxStats }
type remoteURLMapMsg struct{ urls map[string]string }

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
// This uses version-aware matching via grove-context's MatchesGitRule.
func fetchRulesStateCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		mgr := grovecontext.NewManager("") // Use CWD
		rulesState := make(map[string]grovecontext.RuleStatus)

		// Fetch all Git rules upfront for version-aware matching
		gitRules, _ := mgr.ListGitRules()

		for _, project := range projects {
			var status grovecontext.RuleStatus

			// Check for git-based rule first if this is a cx-repo managed project
			if project.RepoShorthand != "" && len(gitRules) > 0 {
				// Construct the expected repo URL from the shorthand
				expectedRepoURL := "https://github.com/" + project.RepoShorthand

				// Get the project's current HEAD commit for comparison
				projectHeadCommit, _ := git.GetHeadCommit(project.Path)

				// Get the project's current version (branch or commit)
				// Priority: GitStatus.Branch (actual checked out branch) > Name (for worktrees) > Version
				projectVersion := project.Version
				if project.GitStatus != nil && project.GitStatus.Branch != "" {
					projectVersion = project.GitStatus.Branch
				} else if project.IsWorktree() && project.Name != "" {
					projectVersion = project.Name
				}
				if projectVersion == "" {
					projectVersion = "main"
				}

				// Match against git rules using centralized matching logic
				for _, rule := range gitRules {
					if grovecontext.MatchesGitRule(rule, expectedRepoURL, projectVersion, projectHeadCommit, project.Path) {
						status = rule.ContextType
						break
					}
				}
			}

			// Check for ecosystem alias rule if this is part of an ecosystem
			if status == 0 && project.RootEcosystemPath != "" {
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

		var rule string

		// Check if this is a cx-repo managed project (has RepoShorthand set)
		if project.RepoShorthand != "" {
			version := project.Version
			if version == "" {
				version = "main" // Sensible fallback
			}
			// For git aliases, we always use the 'default' ruleset,
			// as it's a generic reference to an external repository.
			ruleset := "default"
			rule = fmt.Sprintf("@a:git:%s@%s::%s", project.RepoShorthand, version, ruleset)
		} else if project.RootEcosystemPath != "" {
			// If this is part of an ecosystem, construct an alias
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
		} else {
			// Fallback to path-based rule
			rule = filepath.Join(project.Path, "**")
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

// fetchProjectsCmd returns a command that re-scans configured search paths.
// This command only performs discovery and does NOT fetch enrichment data.
func fetchProjectsCmd(mgr *tmux.Manager, configDir string) tea.Cmd {
	return func() tea.Msg {
		projects, _ := mgr.GetAvailableProjects()

		// Sort by access history
		if history, err := mgr.GetAccessHistory(); err == nil {
			projects = manager.SortProjectsByAccess(history, projects)
		}

		// Group cloned repos under a virtual "Cloned Repos" ecosystem
		projects = groupClonedProjectsAsEcosystem(projects)

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
		status, _ := git.GetExtendedStatus(path)
		return gitStatusMsg{path: path, status: status}
	}
}

// fetchAllGitStatusesCmd returns a command to fetch git status for multiple paths concurrently.
func fetchAllGitStatusesCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := make(map[string]*git.ExtendedGitStatus)
		semaphore := make(chan struct{}, 10) // Limit to 10 concurrent git processes

		for _, p := range projects {
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

// fetchAllReleaseInfoCmd fetches release info using `grove list --json`.
func fetchAllReleaseInfoCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		releases := make(map[string]*manager.ReleaseInfo)

		// Run `grove list --json` once to get all release info
		toolsByRepo := make(map[string]*groveListEntry)
		cmd := exec.Command("grove", "list", "--json")
		if output, err := cmd.Output(); err == nil {
			var tools []groveListEntry
			if json.Unmarshal(output, &tools) == nil {
				for i := range tools {
					toolsByRepo[tools[i].RepoName] = &tools[i]
				}
			}
		}

		for _, p := range projects {
			// Match project to tool by repo name
			repoName := filepath.Base(p.Path)
			if p.IsWorktree() && p.ParentProjectPath != "" {
				repoName = filepath.Base(p.ParentProjectPath)
			}

			if tool, ok := toolsByRepo[repoName]; ok && tool.LatestRelease != "" {
				releases[p.Path] = &manager.ReleaseInfo{
					LatestTag:    tool.LatestRelease,
					CommitsAhead: 0, // grove list doesn't provide this, but it's less important
				}
			}
		}
		return releaseInfoMapMsg{releases: releases}
	}
}

// groveListEntry represents a single tool from `grove list --json`
type groveListEntry struct {
	Name          string `json:"name"`
	RepoName      string `json:"repo_name"`
	Status        string `json:"status"`
	ActiveVersion string `json:"active_version"`
	LatestRelease string `json:"latest_release"`
}

// projectBinaryConfig represents the binary config in grove.yml
type projectBinaryConfig struct {
	Binary struct {
		Name string `yaml:"name"`
	} `yaml:"binary"`
}

// fetchAllBinaryStatusCmd fetches active binary status for all projects.
// Uses `grove list --json` to get tool info efficiently in one call.
func fetchAllBinaryStatusCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		statuses := make(map[string]*manager.BinaryStatus)

		// Run `grove list --json` once to get all tool info
		toolsByRepo := make(map[string]*groveListEntry)
		cmd := exec.Command("grove", "list", "--json")
		if output, err := cmd.Output(); err == nil {
			var tools []groveListEntry
			if json.Unmarshal(output, &tools) == nil {
				for i := range tools {
					toolsByRepo[tools[i].RepoName] = &tools[i]
				}
			}
		}

		for _, p := range projects {
			// Read binary name from project's grove.yml
			groveYmlPath := filepath.Join(p.Path, "grove.yml")
			data, err := os.ReadFile(groveYmlPath)
			if err != nil {
				continue
			}
			var projCfg projectBinaryConfig
			if err := yaml.Unmarshal(data, &projCfg); err != nil || projCfg.Binary.Name == "" {
				continue
			}
			binaryName := projCfg.Binary.Name

			// Look up tool info from grove list output by repo name
			repoName := filepath.Base(p.Path)
			// Handle worktrees: strip worktree suffix to get base repo name
			if p.IsWorktree() && p.ParentProjectPath != "" {
				repoName = filepath.Base(p.ParentProjectPath)
			}

			isDev := false
			currentVersion := ""
			if tool, ok := toolsByRepo[repoName]; ok {
				isDev = tool.Status == "dev"
				currentVersion = tool.ActiveVersion
			}

			statuses[p.Path] = &manager.BinaryStatus{
				ToolName:       binaryName,
				IsDevActive:    isDev,
				LinkName:       "", // Not needed with this approach
				CurrentVersion: currentVersion,
			}
		}
		return binaryStatusMapMsg{statuses: statuses}
	}
}

// fetchAllCxStatsCmd fetches context stats for all projects.
func fetchAllCxStatsCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		stats := make(map[string]*manager.CxStats)
		var wg sync.WaitGroup
		var mu sync.Mutex
		semaphore := make(chan struct{}, 10)

		for _, p := range projects {
			wg.Add(1)
			go func(proj *manager.SessionizeProject) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				cmd := exec.Command("cx", "stats", "--json")
				cmd.Dir = proj.Path
				output, err := cmd.Output()
				if err != nil || len(output) == 0 {
					return
				}

				trimmed := strings.TrimSpace(string(output))
				if strings.HasPrefix(trimmed, "[") {
					var statsArray []manager.CxStats
					if json.Unmarshal([]byte(trimmed), &statsArray) == nil && len(statsArray) > 0 {
						mu.Lock()
						stats[proj.Path] = &statsArray[0]
						mu.Unlock()
					}
				}
			}(p)
		}
		wg.Wait()
		return cxStatsMapMsg{stats: stats}
	}
}

// fetchAllRemoteURLsCmd fetches the git remote URL for all projects.
func fetchAllRemoteURLsCmd(projects []*manager.SessionizeProject) tea.Cmd {
	return func() tea.Msg {
		urls := make(map[string]string)
		for _, p := range projects {
			cmd := exec.Command("git", "remote", "get-url", "origin")
			cmd.Dir = p.Path
			if output, err := cmd.Output(); err == nil {
				urls[p.Path] = strings.TrimSpace(string(output))
			}
		}
		return remoteURLMapMsg{urls: urls}
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
