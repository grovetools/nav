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

	"github.com/mattsolo1/grove-core/git"
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
		noteCountsMap, _ = fetchNoteCountsMap(projects)
	}

	var claudeSessionMap map[string]*ClaudeSessionInfo
	if opts.FetchClaudeSessions {
		claudeSessionMap, _ = fetchClaudeSessionMap()
	}

	var planStatsMap map[string]*PlanStats
	if opts.FetchPlanStats {
		planStatsMap, _ = fetchPlanStatsMap()
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

					if extStatus, err := fetchGitStatusForPath(p.Path); err == nil {
						p.GitStatus = extStatus
					}
				}(project)
			}
		}
	}
	wg.Wait()
}

func fetchGitStatusForPath(path string) (*ExtendedGitStatus, error) {
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

func fetchClaudeSessionMap() (map[string]*ClaudeSessionInfo, error) {
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

func fetchNoteCountsMap(projects []*SessionizeProject) (map[string]*NoteCounts, error) {
	resultsByPath := make(map[string]*NoteCounts)
	nameToPath := make(map[string]string)
	for _, p := range projects {
		if p != nil {
			nameToPath[p.Name] = p.Path
		}
	}

	nbPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "nb")
	var cmd *exec.Cmd
	if _, err := os.Stat(nbPath); err == nil {
		cmd = exec.Command(nbPath, "list", "--workspaces", "--json")
	} else {
		cmd = exec.Command("nb", "list", "--workspaces", "--json")
	}

	output, err := cmd.Output()
	if err != nil {
		return resultsByPath, nil
	}

	type nbNote struct {
		Type      string `json:"type"`
		Workspace string `json:"workspace"`
	}

	var notes []nbNote
	if err := json.Unmarshal(output, &notes); err != nil {
		return resultsByPath, fmt.Errorf("failed to unmarshal nb output: %w", err)
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
		}
	}

	for name, counts := range countsByName {
		if path, ok := nameToPath[name]; ok {
			resultsByPath[path] = counts
		}
	}
	return resultsByPath, nil
}

func fetchPlanStatsMap() (map[string]*PlanStats, error) {
	resultsByPath := make(map[string]*PlanStats)
	flowPath := filepath.Join(os.Getenv("HOME"), ".grove", "bin", "flow")
	if _, err := os.Stat(flowPath); os.IsNotExist(err) {
		var findErr error
		flowPath, findErr = exec.LookPath("flow")
		if findErr != nil {
			return resultsByPath, nil
		}
	}

	cmd := exec.Command(flowPath, "plan", "list", "--json", "--all-workspaces", "--include-finished")
	output, err := cmd.Output()
	if err != nil {
		return resultsByPath, nil
	}

	type flowPlanSummary struct {
		ID            string `json:"id"`
		WorkspacePath string `json:"workspace_path"`
		Status        string `json:"status"`
	}

	var summaries []flowPlanSummary
	if err := json.Unmarshal(output, &summaries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flow output: %w", err)
	}

	type workspaceInfo struct {
		path  string
		plans []flowPlanSummary
	}
	workspaceMap := make(map[string]*workspaceInfo)

	for _, summary := range summaries {
		if summary.WorkspacePath == "" {
			continue
		}
		if _, ok := workspaceMap[summary.WorkspacePath]; !ok {
			workspaceMap[summary.WorkspacePath] = &workspaceInfo{
				path:  summary.WorkspacePath,
				plans: make([]flowPlanSummary, 0),
			}
		}
		workspaceMap[summary.WorkspacePath].plans = append(workspaceMap[summary.WorkspacePath].plans, summary)
	}

	activePlanChan := make(chan struct {
		workspacePath string
		activePlan    string
	}, len(workspaceMap))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)

	for _, wsInfo := range workspaceMap {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			currentCmd := exec.Command(flowPath, "plan", "current")
			currentCmd.Dir = path
			currentOutput, err := currentCmd.Output()
			if err == nil {
				outputStr := strings.TrimSpace(string(currentOutput))
				if strings.HasPrefix(outputStr, "Active job: ") {
					activePlan := strings.TrimPrefix(outputStr, "Active job: ")
					activePlanChan <- struct {
						workspacePath string
						activePlan    string
					}{path, activePlan}
				}
			}
		}(wsInfo.path)
	}

	go func() {
		wg.Wait()
		close(activePlanChan)
	}()

	activePlansByWorkspace := make(map[string]string)
	for result := range activePlanChan {
		activePlansByWorkspace[result.workspacePath] = result.activePlan
	}

	for workspacePath, wsInfo := range workspaceMap {
		stats := &PlanStats{TotalPlans: len(wsInfo.plans)}
		resultsByPath[workspacePath] = stats
		activePlan := activePlansByWorkspace[workspacePath]
		if activePlan == "" {
			continue
		}
		stats.ActivePlan = activePlan

		for _, summary := range wsInfo.plans {
			if summary.ID == activePlan {
				statusParts := strings.Split(summary.Status, ", ")
				for _, part := range statusParts {
					fields := strings.Fields(part)
					if len(fields) >= 2 {
						count, err := strconv.Atoi(fields[0])
						if err != nil {
							continue
						}
						status := fields[1]
						switch status {
						case "completed":
							stats.Completed = count
						case "running":
							stats.Running = count
						case "pending":
							stats.Pending = count
						case "failed":
							stats.Failed = count
						}
					}
				}
				break
			}
		}
	}
	return resultsByPath, nil
}
