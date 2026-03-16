// Package manager provides session management for the nav TUI.
// Enrichment data is now provided by the daemon. When the daemon is not running,
// enrichment data is not available (graceful degradation).
package manager

import (
	"context"
	"sync"
	"time"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/daemon"
)

// EnrichmentOptions controls which data to fetch and for which projects.
type EnrichmentOptions struct {
	FetchNoteCounts bool
	FetchGitStatus  bool
	FetchPlanStats  bool
	GitStatusPaths  map[string]bool
}

// DefaultEnrichmentOptions returns options that fetch everything for all projects.
func DefaultEnrichmentOptions() *EnrichmentOptions {
	return &EnrichmentOptions{
		FetchNoteCounts: true,
		FetchGitStatus:  true,
		FetchPlanStats:  true,
		GitStatusPaths:  nil,
	}
}

// EnrichProjects updates SessionizeProject items in-place with runtime data.
// This function fetches enrichment data via the daemon client when available.
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

					if extStatus, err := git.GetExtendedStatus(p.Path); err == nil {
						p.GitStatus = extStatus
					}
				}(project)
			}
		}
	}
	wg.Wait()
}

// FetchNoteCountsMap fetches note counts via the daemon client.
// Returns empty map if daemon is not running (graceful degradation).
func FetchNoteCountsMap() (map[string]*NoteCounts, error) {
	client := daemon.New()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreCounts, err := client.GetNoteCounts(ctx)
	if err != nil {
		return make(map[string]*NoteCounts), err
	}

	// Convert core types to local types
	result := make(map[string]*NoteCounts, len(coreCounts))
	for name, coreCount := range coreCounts {
		result[name] = &NoteCounts{
			Current:    coreCount.Current,
			Issues:     coreCount.Issues,
			Inbox:      coreCount.Inbox,
			Docs:       coreCount.Docs,
			Completed:  coreCount.Completed,
			Review:     coreCount.Review,
			InProgress: coreCount.InProgress,
			Other:      coreCount.Other,
		}
	}
	return result, nil
}

// FetchPlanStatsMap fetches plan statistics via the daemon client.
// Returns empty map if daemon is not running (graceful degradation).
func FetchPlanStatsMap() (map[string]*PlanStats, error) {
	client := daemon.New()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coreStats, err := client.GetPlanStats(ctx)
	if err != nil {
		return make(map[string]*PlanStats), err
	}

	// Convert core types to local types
	result := make(map[string]*PlanStats, len(coreStats))
	for path, coreStat := range coreStats {
		result[path] = &PlanStats{
			TotalPlans: coreStat.TotalPlans,
			ActivePlan: coreStat.ActivePlan,
			Running:    coreStat.Running,
			Pending:    coreStat.Pending,
			Completed:  coreStat.Completed,
			Failed:     coreStat.Failed,
			Todo:       coreStat.Todo,
			Hold:       coreStat.Hold,
			Abandoned:  coreStat.Abandoned,
			PlanStatus: coreStat.PlanStatus,
		}
	}
	return result, nil
}
