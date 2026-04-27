package sessionizer

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/models"
	core_theme "github.com/grovetools/core/tui/theme"

	"github.com/grovetools/nav/pkg/api"
)

// resolveIcon maps a config icon reference (human name or IconXxx constant)
// to its Nerd Font glyph. Duplicated from cmd/nav's key_manage.go so the
// sessionizer package has no dependency on package main.
func resolveIcon(iconRef string) string {
	switch iconRef {
	case "IconTree", "tree":
		return core_theme.IconTree
	case "IconProject", "project":
		return core_theme.IconProject
	case "IconRepo", "repo":
		return core_theme.IconRepo
	case "IconWorktree", "worktree":
		return core_theme.IconWorktree
	case "IconEcosystem", "ecosystem":
		return core_theme.IconEcosystem
	case "IconFolder", "folder":
		return core_theme.IconFolder
	case "IconFolderStar", "folder-star", "star":
		return core_theme.IconFolderStar
	case "IconHome", "home":
		return core_theme.IconHome
	case "IconCloud", "cloud":
		return "󰅧"
	case "IconCode", "code":
		return core_theme.IconCode
	case "IconBriefcase", "briefcase", "work":
		return "󰃖"
	case "IconKeyboard", "keyboard":
		return core_theme.IconKeyboard
	case "IconNote", "note":
		return core_theme.IconNote
	case "IconPlan", "plan":
		return core_theme.IconPlan
	default:
		return iconRef
	}
}

// formatPlanStats formats plan stats into a styled string
// Shows: total plans (active plan name) [job stats]
func formatPlanStats(stats *models.PlanStats) string {
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
func formatReleaseInfo(info *models.ReleaseInfo) string {
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
func formatToolName(status *models.BinaryStatus) string {
	if status == nil || status.ToolName == "" {
		return "-"
	}
	return status.ToolName
}

// formatCurrentVersion formats the current version for display
func formatCurrentVersion(status *models.BinaryStatus) string {
	if status == nil || status.CurrentVersion == "" {
		return "-"
	}
	return status.CurrentVersion
}

// canonicalVerbOrder defines the preferred display order for validation columns.
var canonicalVerbOrder = []string{"build", "fmt", "vet", "lint", "test", "test-e2e"}

// collectTaskVerbs gathers all unique verb keys from the filtered projects and
// returns them in canonical order (build, fmt, vet, lint, test, test-e2e), with
// any remaining custom verbs sorted alphabetically at the end.
func collectTaskVerbs(projects []*api.Project) []string {
	seen := make(map[string]bool)
	for _, p := range projects {
		for verb := range p.TaskResults {
			seen[verb] = true
		}
	}
	if len(seen) == 0 {
		return nil
	}

	var result []string
	for _, verb := range canonicalVerbOrder {
		if seen[verb] {
			result = append(result, verb)
			delete(seen, verb)
		}
	}
	var extra []string
	for verb := range seen {
		extra = append(extra, verb)
	}
	sort.Strings(extra)
	return append(result, extra...)
}

// formatVerbHeader title-cases a verb name for the column header.
func formatVerbHeader(verb string) string {
	parts := strings.Split(verb, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "-")
}

// formatTaskResultCell renders a single task result cell with a pass/fail icon
// and relative timestamp.
func formatTaskResultCell(result *models.TaskResult) string {
	if result == nil {
		return core_theme.DefaultTheme.Muted.Render("-")
	}
	icon := core_theme.DefaultTheme.Success.Render("✓")
	if result.ExitCode != 0 {
		icon = core_theme.DefaultTheme.Error.Render("✗")
	}
	age := formatRelativeAge(result.Timestamp)
	return icon + " " + core_theme.DefaultTheme.Muted.Render(age)
}

// formatRelativeAge returns a compact relative time string.
func formatRelativeAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
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
