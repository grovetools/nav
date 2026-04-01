package main

import (
	"fmt"
	"strings"

	"github.com/grovetools/core/git"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/internal/manager"
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
			return core_theme.DefaultTheme.Success.Render(core_theme.IconSuccess)
		} else {
			// Clean without upstream: show green empty circle
			return core_theme.DefaultTheme.Success.Render(core_theme.IconStatusTodo)
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

	// Show total plans count (icon is in header)
	totalPlansStr := core_theme.DefaultTheme.Info.Render(fmt.Sprintf("(%d)", stats.TotalPlans))
	parts = append(parts, totalPlansStr)

	// Show active plan name if available
	if stats.ActivePlan != "" {
		activePlanStr := core_theme.DefaultTheme.Muted.Render(stats.ActivePlan)
		parts = append(parts, activePlanStr)

		// Show job stats for active plan
		var jobStats []string
		if stats.Running > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("%s %d", core_theme.IconStatusRunning, stats.Running)))
		}
		if stats.Hold > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("%s %d", core_theme.IconStatusHold, stats.Hold)))
		}
		if stats.Todo > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("%s %d", core_theme.IconStatusTodo, stats.Todo)))
		}
		if stats.Pending > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("%s %d", core_theme.IconStatusPendingUser, stats.Pending)))
		}
		if stats.Completed > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("%s %d", core_theme.IconStatusCompleted, stats.Completed)))
		}
		if stats.Failed > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("%s %d", core_theme.IconStatusFailed, stats.Failed)))
		}
		if stats.Abandoned > 0 {
			jobStats = append(jobStats, core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("%s %d", core_theme.IconStatusAbandoned, stats.Abandoned)))
		}

		if len(jobStats) > 0 {
			parts = append(parts, strings.Join(jobStats, " "))
		}
	}

	return strings.Join(parts, " ")
}

// formatTokens formats a token count in a human-readable way
func formatTokens(tokens int) string {
	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	} else if tokens < 1000000 {
		return fmt.Sprintf("~%.1fk", float64(tokens)/1000)
	}
	return fmt.Sprintf("~%.1fM", float64(tokens)/1000000)
}

// formatReleaseInfo formats release info for display
func formatReleaseInfo(info *manager.ReleaseInfo) string {
	if info == nil || info.LatestTag == "" {
		return "-"
	}
	result := info.LatestTag
	if info.CommitsAhead > 0 {
		result = fmt.Sprintf("%s (%s%d)", info.LatestTag, core_theme.IconArrowUp, info.CommitsAhead)
		// Style based on how many commits ahead
		if info.CommitsAhead > 20 {
			result = core_theme.DefaultTheme.Error.Render(result)
		} else if info.CommitsAhead > 10 {
			result = core_theme.DefaultTheme.Warning.Render(result)
		} else {
			result = core_theme.DefaultTheme.Info.Render(result)
		}
	} else {
		result = core_theme.DefaultTheme.Success.Render(result)
	}
	return result
}

// formatToolName formats the tool name for display
func formatToolName(status *manager.BinaryStatus) string {
	if status == nil || status.ToolName == "" {
		return "-"
	}
	return status.ToolName
}

// formatCurrentVersion formats the current version for display
func formatCurrentVersion(status *manager.BinaryStatus) string {
	if status == nil || status.CurrentVersion == "" {
		return "-"
	}
	return status.CurrentVersion
}

// formatLink converts a git URL to a clean https link
func formatLink(gitURL string) string {
	if gitURL == "" {
		return "-"
	}
	// Convert SSH URLs to HTTPS
	if strings.HasPrefix(gitURL, "git@") {
		// git@github.com:user/repo.git -> https://github.com/user/repo
		gitURL = strings.Replace(gitURL, ":", "/", 1)
		gitURL = strings.Replace(gitURL, "git@", "https://", 1)
	}
	// Remove .git suffix
	gitURL = strings.TrimSuffix(gitURL, ".git")
	return core_theme.DefaultTheme.Muted.Render(gitURL)
}
