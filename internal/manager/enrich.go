// Package manager provides session management for the nav TUI.
// Enrichment logic has been consolidated into github.com/grovetools/core/pkg/enrichment.
// This file provides thin wrappers for backward compatibility.
package manager

import (
	"context"
	"sync"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/enrichment"
)

// EnrichmentOptions controls which data to fetch and for which projects.
// This is a local alias for the core enrichment options.
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
// This function is used for local enrichment when the daemon is not running.
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

// FetchNoteCountsMap fetches note counts for all known workspaces.
// Delegates to core/pkg/enrichment.
func FetchNoteCountsMap() (map[string]*NoteCounts, error) {
	coreCounts, err := enrichment.FetchNoteCountsMap()
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

// FetchPlanStatsMap fetches plan statistics for all workspaces.
// Delegates to core/pkg/enrichment.
func FetchPlanStatsMap() (map[string]*PlanStats, error) {
	coreStats, err := enrichment.FetchPlanStatsMap()
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
