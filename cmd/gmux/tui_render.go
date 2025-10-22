package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/git"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
)

// highlightMatch highlights the matched portion of text with a yellow background
func highlightMatch(text, filter string) string {
	if filter == "" {
		return text
	}

	lowerText := strings.ToLower(text)
	lowerFilter := strings.ToLower(filter)

	// Find the position of the match
	index := strings.Index(lowerText, lowerFilter)
	if index == -1 {
		return text
	}

	// Split the text into parts: before, match, after
	before := text[:index]
	match := text[index : index+len(filter)]
	after := text[index+len(filter):]

	// Highlight the match with reversed warning style
	highlightStyle := core_theme.DefaultTheme.Warning.Copy().Reverse(true)

	return before + highlightStyle.Render(match) + after
}

// formatChanges formats the git status into a styled string.
func formatChanges(status *git.StatusInfo, extStatus *manager.ExtendedGitStatus) string {
	if status == nil {
		return ""
	}

	var changes []string

	isMainBranch := status.Branch == "main" || status.Branch == "master"
	hasMainDivergence := !isMainBranch && (status.AheadMainCount > 0 || status.BehindMainCount > 0)

	if hasMainDivergence {
		if status.AheadMainCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("⇡%d", status.AheadMainCount)))
		}
		if status.BehindMainCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("⇣%d", status.BehindMainCount)))
		}
	} else if status.HasUpstream {
		if status.AheadCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("↑%d", status.AheadCount)))
		}
		if status.BehindCount > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("↓%d", status.BehindCount)))
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

	// Add lines added/deleted if available
	if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
		if extStatus.LinesAdded > 0 {
			changes = append(changes, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("+%d", extStatus.LinesAdded)))
		}
		if extStatus.LinesDeleted > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("-%d", extStatus.LinesDeleted)))
		}
	}

	changesStr := strings.Join(changes, " ")

	// If repo is clean (no changes)
	if !status.IsDirty && changesStr == "" {
		if status.HasUpstream {
			// Clean with upstream: show green checkmark
			return core_theme.DefaultTheme.Success.Render("✓")
		} else {
			// Clean without upstream: show green empty circle
			return core_theme.DefaultTheme.Success.Render("○")
		}
	}

	return changesStr
}

// formatPlanStats formats plan stats into a styled string
// Shows: total plans (active plan name) [job stats]
func formatPlanStats(stats *manager.PlanStats) string {
	if stats == nil || stats.TotalPlans == 0 {
		return ""
	}

	var parts []string

	// Show total plans count
	totalPlansStr := core_theme.DefaultTheme.Info.Render(fmt.Sprintf("(%d)", stats.TotalPlans))
	parts = append(parts, totalPlansStr)

	// Show active plan name if available
	if stats.ActivePlan != "" {
		activePlanStr := core_theme.DefaultTheme.Muted.Render(stats.ActivePlan)
		parts = append(parts, activePlanStr)

		// Show job stats for active plan
		var jobStats []string
		if stats.Running > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("◐ %d", stats.Running)))
		}
		if stats.Hold > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("⏸ %d", stats.Hold)))
		}
		if stats.Todo > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("○ %d", stats.Todo)))
		}
		if stats.Pending > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("○ %d", stats.Pending)))
		}
		if stats.Completed > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("● %d", stats.Completed)))
		}
		if stats.Failed > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("✗ %d", stats.Failed)))
		}
		if stats.Abandoned > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("⊗ %d", stats.Abandoned)))
		}

		if len(jobStats) > 0 {
			parts = append(parts, strings.Join(jobStats, " "))
		}
	}

	return strings.Join(parts, " ")
}
