package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	coreconfig "github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-flow/pkg/orchestration"
	"github.com/sirupsen/logrus"
)

// EnrichmentOptions controls which data to fetch and for which projects
type EnrichmentOptions struct {
	FetchNoteCounts     bool
	FetchClaudeSessions bool
	FetchGitStatus      bool
	FetchPlanStats      bool
	GitStatusPaths      map[string]bool
}

// DefaultEnrichmentOptions returns options that fetch everything for all projects
func DefaultEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts:     true,
		FetchClaudeSessions: true,
		FetchGitStatus:      true,
		FetchPlanStats:      true,
		GitStatusPaths:      nil, // nil means all projects
	}
}

// EnrichProjects updates SessionizeProject items in-place with runtime data.
func EnrichProjects(ctx context.Context, projects []*SessionizeProject, opts *EnrichmentOptions) {
	if opts == nil {
		opts = DefaultEnrichmentOptions()
	}

	var noteCountsMap map[string]*NoteCounts
	if opts.FetchNoteCounts {
		noteCountsByName, _ := FetchNoteCountsMap()
		// Map by project name to project path
		noteCountsMap = make(map[string]*NoteCounts)
		for _, proj := range projects {
			if counts, ok := noteCountsByName[proj.Name]; ok {
				noteCountsMap[proj.Path] = counts
			}
		}
	}

	var claudeSessionMap map[string]*ClaudeSessionInfo
	if opts.FetchClaudeSessions {
		claudeSessionMap, _ = FetchClaudeSessionMap()
	}

	var planStatsMap map[string]*PlanStats
	if opts.FetchPlanStats {
		planStatsMap, _ = FetchPlanStatsMap()
	}

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10)

	for _, project := range projects {
		if noteCountsMap != nil {
			if counts, ok := noteCountsMap[project.Path]; ok {
				project.NoteCounts = counts
			}
		}

		if claudeSessionMap != nil {
			if session, ok := claudeSessionMap[project.Path]; ok {
				project.ClaudeSession = session
			}
		}

		if planStatsMap != nil {
			if stats, ok := planStatsMap[project.Path]; ok {
				project.PlanStats = stats
			}
		}

		if opts.FetchGitStatus {
			shouldFetch := opts.GitStatusPaths == nil || opts.GitStatusPaths[project.Path]
			if shouldFetch {
				wg.Add(1)
				go func(p *SessionizeProject) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					if extStatus, err := FetchGitStatusForPath(p.Path); err == nil {
						p.GitStatus = extStatus
					}
				}(project)
			}
		}
	}
	wg.Wait()
}

// FetchGitStatusForPath fetches extended git status for a single repository path.
func FetchGitStatusForPath(path string) (*ExtendedGitStatus, error) {
	cleanPath := filepath.Clean(path)
	if !git.IsGitRepo(cleanPath) {
		return nil, nil
	}

	status, err := git.GetStatus(cleanPath)
	if err != nil {
		return nil, err
	}

	extStatus := &ExtendedGitStatus{StatusInfo: status}

	if status.Branch != "main" && status.Branch != "master" {
		ahead, behind := git.GetCommitsDivergenceFromMain(cleanPath, status.Branch)
		status.AheadMainCount = ahead
		status.BehindMainCount = behind
	}

	cmd := exec.Command("git", "diff", "--numstat")
	cmd.Dir = cleanPath
	output, err := cmd.Output()
	if err == nil {
		extStatus.LinesAdded, extStatus.LinesDeleted = parseNumstat(string(output))
	}

	cmd = exec.Command("git", "diff", "--cached", "--numstat")
	cmd.Dir = cleanPath
	output, err = cmd.Output()
	if err == nil {
		stagedAdded, stagedDeleted := parseNumstat(string(output))
		extStatus.LinesAdded += stagedAdded
		extStatus.LinesDeleted += stagedDeleted
	}

	return extStatus, nil
}

func parseNumstat(output string) (added, deleted int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if fields[0] != "-" {
				if a, err := strconv.Atoi(fields[0]); err == nil {
					added += a
				}
			}
			if fields[1] != "-" {
				if d, err := strconv.Atoi(fields[1]); err == nil {
					deleted += d
				}
			}
		}
	}
	return added, deleted
}

// claudeSessionRaw represents the raw JSON structure from grove-hooks
type claudeSessionRaw struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	PID              int    `json:"pid"`
	Status           string `json:"status"`
	WorkingDirectory string `json:"working_directory"`
	StateDuration    string `json:"state_duration"`
}

// FetchClaudeSessionMap fetches all active Claude sessions.
func FetchClaudeSessionMap() (map[string]*ClaudeSessionInfo, error) {
	sessionMap := make(map[string]*ClaudeSessionInfo)
	groveHooksPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "grove-hooks")
	var cmd *exec.Cmd
	if _, err := os.Stat(groveHooksPath); err == nil {
		cmd = exec.Command(groveHooksPath, "sessions", "list", "--active", "--json")
	} else {
		cmd = exec.Command("grove-hooks", "sessions", "list", "--active", "--json")
	}

	output, err := cmd.Output()
	if err != nil {
		return sessionMap, err
	}

	var claudeSessions []claudeSessionRaw
	if err := json.Unmarshal(output, &claudeSessions); err != nil {
		return sessionMap, err
	}

	for _, session := range claudeSessions {
		if session.Type == "claude_session" && session.WorkingDirectory != "" {
			absPath, err := filepath.Abs(expandPath(session.WorkingDirectory))
			if err != nil {
				continue
			}
			cleanPath := filepath.Clean(absPath)
			sessionMap[cleanPath] = &ClaudeSessionInfo{
				ID:       session.ID,
				PID:      session.PID,
				Status:   session.Status,
				Duration: session.StateDuration,
			}
		}
	}
	return sessionMap, nil
}

// FetchNoteCountsMap fetches note counts for all known workspaces.
// Note: This function returns counts indexed by workspace name (not path).
// The caller should map workspace names to paths as needed.
func FetchNoteCountsMap() (map[string]*NoteCounts, error) {

	nbPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "nb")
	var cmd *exec.Cmd
	if _, err := os.Stat(nbPath); err == nil {
		cmd = exec.Command(nbPath, "list", "--workspaces", "--json")
	} else {
		cmd = exec.Command("grove", "nb", "list", "--workspaces", "--json")
	}

	output, err := cmd.Output()
	if err != nil {
		return make(map[string]*NoteCounts), nil
	}

	type nbNote struct {
		Type      string `json:"type"`
		Workspace string `json:"workspace"`
	}

	var notes []nbNote
	if err := json.Unmarshal(output, &notes); err != nil {
		return make(map[string]*NoteCounts), fmt.Errorf("failed to unmarshal nb output: %w", err)
	}

	countsByName := make(map[string]*NoteCounts)
	for _, note := range notes {
		if _, ok := countsByName[note.Workspace]; !ok {
			countsByName[note.Workspace] = &NoteCounts{}
		}
		switch note.Type {
		case "current":
			countsByName[note.Workspace].Current++
		case "issues":
			countsByName[note.Workspace].Issues++
		case "inbox":
			countsByName[note.Workspace].Inbox++
		case "docs":
			countsByName[note.Workspace].Docs++
		case "completed":
			countsByName[note.Workspace].Completed++
		case "review":
			countsByName[note.Workspace].Review++
		case "in-progress":
			countsByName[note.Workspace].InProgress++
		default:
			countsByName[note.Workspace].Other++
		}
	}

	return countsByName, nil
}

// FetchPlanStatsMap fetches plan statistics for all workspaces using NotebookLocator.
func FetchPlanStatsMap() (map[string]*PlanStats, error) {
	statsByPath := make(map[string]*PlanStats)
	seenDirs := make(map[string]*PlanStats) // Cache for already processed directories

	// 1. Initialize dependencies from grove-core
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	discoveryService := workspace.NewDiscoveryService(logger)
	discoveryResult, err := discoveryService.DiscoverAll()
	if err != nil {
		return nil, fmt.Errorf("workspace discovery failed: %w", err)
	}
	provider := workspace.NewProvider(discoveryResult)

	coreCfg, err := coreconfig.LoadDefault()
	if err != nil {
		coreCfg = &coreconfig.Config{}
	}
	locator := workspace.NewNotebookLocator(coreCfg)

	// 2. Process each workspace node individually
	for _, node := range provider.All() {
		// 3. Get the plans directory for this specific node
		plansRootDir, err := locator.GetPlansDir(node)
		if err != nil {
			statsByPath[node.Path] = &PlanStats{}
			continue // Skip if we can't find the plans directory
		}

		// Check if we have already processed this directory
		if cachedStats, seen := seenDirs[plansRootDir]; seen {
			statsByPath[node.Path] = cachedStats
			continue
		}

		// Get or create the stats object for this directory
		stats := &PlanStats{}
		statsByPath[node.Path] = stats
		seenDirs[plansRootDir] = stats // Add to cache before processing

		// 4. Walk the plans root directory to find individual plans
		entries, err := os.ReadDir(plansRootDir)
		if err != nil {
			continue // Skip this directory if unreadable
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Skip hidden directories (like .archive, .grove)
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			planPath := filepath.Join(plansRootDir, entry.Name())
			plan, err := orchestration.LoadPlan(planPath)
			if err != nil {
				continue // Skip if it's not a valid plan
			}

			// For worktree nodes, set PlanStatus if this plan's worktree matches the node's worktree name
			if node.IsWorktree() && plan.Config != nil && plan.Config.Worktree != "" {
				// Get the worktree name from the node
				worktreeName := node.GetWorktreeName()
				if worktreeName != "" && plan.Config.Worktree == worktreeName {
					stats.PlanStatus = plan.Config.Status
				}
			}

			// Skip finished plans
			if plan.Config != nil && plan.Config.Status == "finished" {
				continue
			}

			// 5. Aggregate stats for this node
			stats.TotalPlans++
			for _, job := range plan.Jobs {
				switch job.Status {
				case "completed":
					stats.Completed++
				case "running":
					stats.Running++
				case "pending", "pending_user":
					stats.Pending++
				case "failed":
					stats.Failed++
				case "todo":
					stats.Todo++
				case "hold":
					stats.Hold++
				case "abandoned":
					stats.Abandoned++
				}
			}
		}

		// Also try to find the active plan for this specific workspace path
		activePlan := getActivePlanForPath(node.Path)
		if activePlan != "" {
			stats.ActivePlan = activePlan
		}
	}

	return statsByPath, nil
}

// getActivePlanForPath reads the active plan from a workspace's state file
func getActivePlanForPath(workspacePath string) string {
	stateFilePath := filepath.Join(workspacePath, ".grove", "state.yml")
	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		return ""
	}

	var stateMap map[string]interface{}
	if err := json.Unmarshal(data, &stateMap); err != nil {
		// Try YAML format
		stateMap = make(map[string]interface{})
		// Simple YAML parsing - look for "flow.active_plan:" line
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "flow.active_plan:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
		return ""
	}

	// Try both keys for backward compatibility
	if val, ok := stateMap["flow.active_plan"].(string); ok {
		return val
	}
	if val, ok := stateMap["active_plan"].(string); ok {
		return val
	}
	return ""
}

// parseJobStatus extracts the status field from job frontmatter
func parseJobStatus(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				break
			}
		}

		if !inFrontmatter {
			continue
		}

		// Skip lines with leading whitespace (nested YAML structures)
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"`)

		if key == "status" {
			return value
		}
	}
	return "pending"
}
