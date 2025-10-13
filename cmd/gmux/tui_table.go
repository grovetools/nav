package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/components/table"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-tmux/internal/manager"
)

// renderTable renders the table view for projects
func (m sessionizeModel) renderTable() string {
	if len(m.filtered) == 0 {
		if len(m.projects) == 0 {
			return core_theme.DefaultTheme.Muted.Render("No projects found")
		} else {
			return core_theme.DefaultTheme.Muted.Render("No matching projects")
		}
	}

	// Calculate visible items based on terminal height
	// Reserve space for: header (3 lines), table header/borders (4 lines), help (1 line), search paths (2 lines)
	visibleHeight := m.height - 10
	if visibleHeight < 5 {
		visibleHeight = 5 // Minimum visible items
	}

	// Determine visible range with scrolling
	start := 0
	end := len(m.filtered)

	// Implement scrolling if there are too many items
	if end > visibleHeight {
		// Center the cursor in the visible area when possible
		if m.cursor < visibleHeight/2 {
			// Near the top
			start = 0
		} else if m.cursor >= len(m.filtered)-visibleHeight/2 {
			// Near the bottom
			start = len(m.filtered) - visibleHeight
		} else {
			// Middle - center the cursor
			start = m.cursor - visibleHeight/2
		}

		end = start + visibleHeight
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
		if start < 0 {
			start = 0
		}
	}

	// Define table headers based on what's enabled
	headers := []string{"K", "●", "WORKSPACE"}

	if m.showBranch {
		headers = append(headers, "BRANCH")
	}
	if m.showGitStatus {
		headers = append(headers, "GIT", "CHANGES")
	}
	if m.showNoteCounts {
		headers = append(headers, "NOTES")
	}
	if m.showPlanStats {
		headers = append(headers, "PLANS")
	}
	if m.showClaudeSessions {
		headers = append(headers, "CLAUDE")
	}
	if m.pathDisplayMode > 0 {
		headers = append(headers, "PATH")
	}

	// Build rows only for visible range
	visibleProjects := m.filtered[start:end]
	rows := make([][]string, len(visibleProjects))
	for i, project := range visibleProjects {
		rows[i] = m.formatProjectRow(project)
	}

	// Adjust cursor to be relative to visible window
	relativeCursor := m.cursor - start

	// Render the table with selection
	tableStr := table.SelectableTable(headers, rows, relativeCursor)

	// Add scroll indicator if needed
	if start > 0 || end < len(m.filtered) {
		scrollInfo := fmt.Sprintf("\n%s", core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("(%d-%d of %d)", start+1, end, len(m.filtered))))
		tableStr += scrollInfo
	}

	return tableStr
}

// formatProjectRow formats a single project into a table row
func (m sessionizeModel) formatProjectRow(project manager.DiscoveredProject) []string {
	// --- WORKSPACE ---
	workspaceName := project.Name

	// Determine prefix based on mode and project type
	prefix := ""
	if m.ecosystemPickerMode {
		// In ecosystem picker mode - show tree structure for worktrees
		if project.IsWorktree() {
			// Check if this is the last worktree of its parent
			isLast := true
			for j := range m.filtered {
				if m.filtered[j].IsWorktree() &&
					m.filtered[j].ParentProjectPath == project.ParentProjectPath &&
					m.filtered[j].Path != project.Path {
					// Found another worktree after this one
					if m.filtered[j].Name > project.Name {
						isLast = false
						break
					}
				}
			}
			if isLast {
				prefix = "└─ "
			} else {
				prefix = "├─ "
			}
		}
		// Main ecosystem - no prefix
	} else if m.focusedProject != nil {
		// In focus mode
		if project.Path == m.focusedProject.Path {
			// This is the focused ecosystem - no prefix
			prefix = ""
		} else if project.IsWorktree() {
			// Worktree
			if project.ParentProjectPath == m.focusedProject.Path {
				// Direct worktree of focused ecosystem
				prefix = "└─ "
			} else {
				// Worktree of a repo within the focused ecosystem
				prefix = "  └─ "
			}
		} else {
			// Regular repo within focused ecosystem
			prefix = "├─ "
		}
	} else {
		// Normal mode - only show worktree indicator
		if project.IsWorktree() {
			prefix = "└─ "
		}
	}

	workspaceName = prefix + project.Name

	// Apply color styling
	if project.IsWorktree() {
		// Apply blue styling for worktrees
		workspaceName = lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Blue).Render(workspaceName)
	} else if m.focusedProject == nil || project.Path != m.focusedProject.Path {
		// Apply cyan styling for main projects (not the focused ecosystem itself)
		workspaceName = lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Cyan).Render(workspaceName)
	}

	// --- KEY ---
	keyMapping := ""
	cleanPath := filepath.Clean(project.Path)
	if key, hasKey := m.keyMap[cleanPath]; hasKey {
		keyMapping = key
	} else {
		// Try case-insensitive match on macOS
		for path, key := range m.keyMap {
			if strings.EqualFold(path, cleanPath) {
				keyMapping = key
				break
			}
		}
	}
	// Leave keyMapping empty if no key is bound

	// --- STATUS ---
	sessionName := project.Identifier()
	sessionExists := m.runningSessions[sessionName]
	statusIndicator := "-"
	if sessionExists {
		if sessionName == m.currentSession {
			// Current session - use blue indicator
			statusIndicator = lipgloss.NewStyle().
				Foreground(core_theme.DefaultColors.Blue).
				Render("●")
		} else {
			// Other active session - use green indicator
			statusIndicator = lipgloss.NewStyle().
				Foreground(core_theme.DefaultColors.Green).
				Render("●")
		}
	}

	// --- BRANCH ---
	branch := "-"
	if m.showBranch {
		extStatus := project.GetExtendedGitStatus()
		if extStatus != nil && extStatus.StatusInfo != nil {
			branch = extStatus.StatusInfo.Branch
		}
	}

	// --- GIT STATUS ---
	gitStatus := "-"
	if m.showGitStatus {
		status := project.GetGitStatus()
		if status != nil {
			var statusParts []string
			if status.IsDirty {
				statusParts = append(statusParts, core_theme.DefaultTheme.Warning.Render("✗"))
			}

			isMainBranch := status.Branch == "main" || status.Branch == "master"
			hasMainDivergence := !isMainBranch && (status.AheadMainCount > 0 || status.BehindMainCount > 0)

			if hasMainDivergence {
				if status.AheadMainCount > 0 {
					statusParts = append(statusParts, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("⇡%d", status.AheadMainCount)))
				}
				if status.BehindMainCount > 0 {
					statusParts = append(statusParts, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("⇣%d", status.BehindMainCount)))
				}
			} else if status.HasUpstream {
				if status.AheadCount > 0 {
					statusParts = append(statusParts, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("↑%d", status.AheadCount)))
				}
				if status.BehindCount > 0 {
					statusParts = append(statusParts, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("↓%d", status.BehindCount)))
				}
			}

			if len(statusParts) > 0 {
				gitStatus = strings.Join(statusParts, " ")
			} else if !status.IsDirty {
				gitStatus = core_theme.DefaultTheme.Success.Render("✓")
			}
		}
	}

	// --- CHANGES ---
	changes := "-"
	if m.showGitStatus {
		status := project.GetGitStatus()
		extStatus := project.GetExtendedGitStatus()
		var changeParts []string

		// Add file counts for new, modified, and staged files
		if status != nil {
			if status.UntrackedCount > 0 {
				// New files (untracked)
				changeParts = append(changeParts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render(fmt.Sprintf("N:%d", status.UntrackedCount)))
			}
			if status.ModifiedCount > 0 {
				// Modified files (includes modified and deleted in working tree)
				changeParts = append(changeParts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Yellow).Render(fmt.Sprintf("M:%d", status.ModifiedCount)))
			}
			if status.StagedCount > 0 {
				// Staged files (includes all staged changes: adds, modifies, deletes)
				changeParts = append(changeParts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Cyan).Render(fmt.Sprintf("S:%d", status.StagedCount)))
			}
		}

		// Add line changes if available
		if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
			added := lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Green).Render(fmt.Sprintf("+%d", extStatus.LinesAdded))
			deleted := lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Red).Render(fmt.Sprintf("-%d", extStatus.LinesDeleted))
			changeParts = append(changeParts, added+" "+deleted)
		}

		if len(changeParts) > 0 {
			changes = strings.Join(changeParts, " ")
		}
	}

	// --- NOTES ---
	notes := "-"
	if m.showNoteCounts && project.NoteCounts != nil {
		var parts []string
		if project.NoteCounts.Current > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Violet).Render(fmt.Sprintf("C:%d", project.NoteCounts.Current)))
		}
		if project.NoteCounts.Issues > 0 {
			parts = append(parts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Pink).Render(fmt.Sprintf("I:%d", project.NoteCounts.Issues)))
		}
		if len(parts) > 0 {
			notes = strings.Join(parts, " ")
		}
	}

	// --- PLANS ---
	plans := "-"
	if m.showPlanStats && project.PlanStats != nil {
		formattedStats := formatPlanStats(project.PlanStats)
		if formattedStats != "" {
			plans = formattedStats
		}
	}

	// --- CLAUDE SESSION ---
	claude := "-"
	if m.showClaudeSessions {
		if project.ClaudeSession != nil {
			// This is a Claude session entry - use its own status
			statusSymbol := ""
			var statusColor lipgloss.Color
			switch project.ClaudeSession.Status {
			case "running":
				statusSymbol = "▶"
				statusColor = core_theme.DefaultColors.Green
			case "idle":
				statusSymbol = "⏸"
				statusColor = core_theme.DefaultColors.Yellow
			case "completed":
				statusSymbol = "✓"
				statusColor = core_theme.DefaultColors.Cyan
			case "failed", "error":
				statusSymbol = "✗"
				statusColor = core_theme.DefaultColors.Red
			default:
				statusColor = core_theme.DefaultColors.MutedText
			}

			statusStyled := lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			claude = fmt.Sprintf("%s %s", statusStyled, project.ClaudeSession.Duration)
		} else if m.hasGroveHooks {
			// Regular project - check if it has any Claude sessions
			if status, found := m.claudeStatusMap[cleanPath]; found {
				claudeStatus := status
				claudeDuration := ""
				if duration, foundDur := m.claudeDurationMap[cleanPath]; foundDur {
					claudeDuration = duration
				}

				// Style the claude status
				statusSymbol := ""
				var statusColor lipgloss.Color
				switch claudeStatus {
				case "running":
					statusSymbol = "▶"
					statusColor = core_theme.DefaultColors.Green
				case "idle":
					statusSymbol = "⏸"
					statusColor = core_theme.DefaultColors.Yellow
				case "completed":
					statusSymbol = "✓"
					statusColor = core_theme.DefaultColors.Cyan
				case "failed", "error":
					statusSymbol = "✗"
					statusColor = core_theme.DefaultColors.Red
				default:
					statusColor = core_theme.DefaultColors.MutedText
				}

				if statusSymbol != "" {
					statusStyled := lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
					claude = fmt.Sprintf("%s %s", statusStyled, claudeDuration)
				}
			} else {
				// Try case-insensitive match on macOS
				for path, status := range m.claudeStatusMap {
					if strings.EqualFold(path, cleanPath) {
						claudeStatus := status
						claudeDuration := ""
						if duration, foundDur := m.claudeDurationMap[path]; foundDur {
							claudeDuration = duration
						}

						statusSymbol := ""
						var statusColor lipgloss.Color
						switch claudeStatus {
						case "running":
							statusSymbol = "▶"
							statusColor = core_theme.DefaultColors.Green
						case "idle":
							statusSymbol = "⏸"
							statusColor = core_theme.DefaultColors.Yellow
						case "completed":
							statusSymbol = "✓"
							statusColor = core_theme.DefaultColors.Cyan
						case "failed", "error":
							statusSymbol = "✗"
							statusColor = core_theme.DefaultColors.Red
						default:
							statusColor = core_theme.DefaultColors.MutedText
						}

						if statusSymbol != "" {
							statusStyled := lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
							claude = fmt.Sprintf("%s %s", statusStyled, claudeDuration)
						}
						break
					}
				}
			}
		}
	}

	// --- PATH ---
	pathDisplay := ""
	if m.pathDisplayMode > 0 {
		pathDisplay = project.Path
		if m.pathDisplayMode == 1 {
			// Compact: replace home with ~
			pathDisplay = strings.Replace(pathDisplay, os.Getenv("HOME"), "~", 1)
		}
		// Apply muted styling
		pathDisplay = lipgloss.NewStyle().Foreground(core_theme.DefaultColors.MutedText).Render(pathDisplay)
	}

	// Build row based on enabled columns
	row := []string{keyMapping, statusIndicator, workspaceName}

	if m.showBranch {
		row = append(row, branch)
	}
	if m.showGitStatus {
		row = append(row, gitStatus, changes)
	}
	if m.showNoteCounts {
		row = append(row, notes)
	}
	if m.showPlanStats {
		row = append(row, plans)
	}
	if m.showClaudeSessions {
		row = append(row, claude)
	}
	if m.pathDisplayMode > 0 {
		row = append(row, pathDisplay)
	}

	return row
}

// Helper function to check if an ExtendedGitStatus has any git status
func hasGitStatus(gitStatus interface{}) bool {
	if gitStatus == nil {
		return false
	}
	if extStatus, ok := gitStatus.(*manager.ExtendedGitStatus); ok && extStatus != nil && extStatus.StatusInfo != nil {
		return true
	}
	return false
}
