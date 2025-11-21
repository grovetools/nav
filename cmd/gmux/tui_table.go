package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	grovecontext "github.com/mattsolo1/grove-context/pkg/context"
	"github.com/mattsolo1/grove-core/pkg/repo"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/components/table"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
	"github.com/mattsolo1/grove-core/util/pathutil"
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
	headers := []string{"K", "S", "CX", "WORKSPACE"}

	// Get spinner for animation
	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	if m.showBranch {
		headers = append(headers, core_theme.IconGitBranch+" BRANCH")
	}
	if m.showGitStatus {
		gitHeader := core_theme.IconGit + " GIT"
		changesHeader := core_theme.IconGit + " CHANGES"
		if m.enrichmentLoading["git"] {
			gitHeader = core_theme.IconGit + " GIT " + spinner
			changesHeader = core_theme.IconGit + " CHANGES " + spinner
		}
		headers = append(headers, gitHeader, changesHeader)
	}
	if m.showNoteCounts {
		notesHeader := core_theme.IconNote + " NOTES"
		if m.enrichmentLoading["notes"] {
			notesHeader = core_theme.IconNote + " NOTES " + spinner
		}
		headers = append(headers, notesHeader)
	}
	if m.showPlanStats {
		plansHeader := core_theme.IconPlan + " PLANS"
		if m.enrichmentLoading["plans"] {
			plansHeader = core_theme.IconPlan + " PLANS " + spinner
		}
		headers = append(headers, plansHeader)
	}
	if m.showClaudeSessions {
		claudeHeader := core_theme.IconInteractiveAgent + " CLAUDE"
		if m.enrichmentLoading["claude"] {
			claudeHeader = core_theme.IconInteractiveAgent + " CLAUDE " + spinner
		}
		headers = append(headers, claudeHeader)
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
func (m sessionizeModel) formatProjectRow(project *manager.SessionizeProject) []string {
	// --- WORKSPACE ---
	workspaceName := project.Name

	// Find this project's index in the filtered list (for isLast detection)
	projectIndex := -1
	for i, p := range m.filtered {
		if p.Path == project.Path {
			projectIndex = i
			break
		}
	}

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
		// In focus mode - use hierarchical parent to determine indentation
		if project.Path == m.focusedProject.Path {
			// This is the focused ecosystem - no prefix
			prefix = ""
		} else if project.GetHierarchicalParent() == m.focusedProject.Path {
			// This is a direct child of the focused project
			// Check if it's the last direct child
			isLast := true
			if projectIndex >= 0 {
				for j := projectIndex + 1; j < len(m.filtered); j++ {
					if m.filtered[j].GetHierarchicalParent() == m.focusedProject.Path {
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
		} else {
			// This is a grandchild (e.g., worktree of a sub-project)
			prefix = "  └─ "
		}
	} else {
		// Normal mode - only show worktree indicator
		if project.IsWorktree() {
			prefix = "└─ "
		}
	}

	// Determine workspace icon based on project kind
	icon := ""
	// Get cx ecosystem path for bare repo detection
	cxEcoPath, _ := repo.GetCxEcosystemPath()
	isBareRepo := project.Kind == workspace.KindEcosystemSubProject && project.ParentEcosystemPath == cxEcoPath

	// Special case for cx-repos virtual ecosystem
	if project.Name == "cx-repos" && project.Kind == workspace.KindEcosystemRoot {
		icon = core_theme.IconArchive
	} else if isBareRepo {
		// Bare repos under cx-repos use repo icon (color handled separately)
		icon = core_theme.IconRepo
	} else {
		switch project.Kind {
		case workspace.KindEcosystemRoot:
			icon = core_theme.IconEcosystem
		case workspace.KindEcosystemWorktree:
			icon = core_theme.IconWorktree // Use IconWorktree as IconEcosystemWorktree is not in core
		case workspace.KindStandaloneProjectWorktree,
			workspace.KindEcosystemSubProjectWorktree,
			workspace.KindEcosystemWorktreeSubProjectWorktree:
			icon = core_theme.IconWorktree
		default:
			// Sub-projects and standalone projects
			icon = core_theme.IconRepo
		}
	}

	// Determine icon color based on session status
	sessionName := project.Identifier()
	sessionExists := m.runningSessions[sessionName]

	var iconStyle lipgloss.Style
	// Special styling for cx-repos virtual ecosystem and bare repos
	if project.Name == "cx-repos" && project.Kind == workspace.KindEcosystemRoot {
		// Use a distinct purple/violet color for cx-repos
		iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141")) // Purple
	} else if isBareRepo {
		// Bare repos use purple when session is open, muted otherwise
		if sessionExists {
			iconStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("141")) // Purple
		} else {
			iconStyle = core_theme.DefaultTheme.Muted
		}
	} else if sessionExists {
		if sessionName == m.currentSession {
			iconStyle = core_theme.DefaultTheme.Info // Current session - cyan
		} else {
			iconStyle = core_theme.DefaultTheme.Highlight // Other active session
		}
	} else {
		iconStyle = core_theme.DefaultTheme.Muted // Inactive
	}

	iconStyled := iconStyle.Render(icon + " ")

	// Apply subtle styling for different workspace types (only to the name, not icon)
	var nameStyled string
	switch project.Kind {
	case workspace.KindEcosystemWorktree,
		workspace.KindStandaloneProjectWorktree,
		workspace.KindEcosystemSubProjectWorktree,
		workspace.KindEcosystemWorktreeSubProjectWorktree:
		// Worktrees use faint/dim styling for visual distinction
		if m.focusedProject != nil && project.Path == m.focusedProject.Path {
			// Don't apply faint styling to the focused project itself
			nameStyled = project.Name
		} else {
			nameStyled = lipgloss.NewStyle().Faint(true).Render(project.Name)
		}
	case workspace.KindEcosystemRoot:
		// Ecosystem roots are normal weight
		nameStyled = project.Name
	default:
		// Sub-projects and standalone projects are normal weight
		nameStyled = project.Name
	}

	workspaceName = prefix + iconStyled + nameStyled

	// --- KEY ---
	keyMapping := ""
	cleanPath := filepath.Clean(project.Path)
	normalizedCleanPath, err := pathutil.NormalizeForLookup(cleanPath)
	if err == nil {
		// Try exact match first
		if key, hasKey := m.keyMap[cleanPath]; hasKey {
			keyMapping = key
		} else {
			// Try normalized path match
			for path, key := range m.keyMap {
				normalizedPath, err := pathutil.NormalizeForLookup(path)
				if err == nil && normalizedPath == normalizedCleanPath {
					keyMapping = key
					break
				}
			}
		}
	}
	// Leave keyMapping empty if no key is bound

	// --- STATUS ---
	statusIndicator := ""
	if sessionExists {
		statusIndicator = core_theme.DefaultTheme.Success.Render(core_theme.IconBullet)
	} else {
		statusIndicator = core_theme.DefaultTheme.Muted.Render("-")
	}

	// --- CONTEXT STATUS ---
	cxStatus := ""
	if status, ok := m.rulesState[project.Path]; ok {
		switch status {
		case grovecontext.RuleHot:
			cxStatus = core_theme.DefaultTheme.Success.Render("H")
		case grovecontext.RuleCold:
			cxStatus = core_theme.DefaultTheme.Info.Render("C")
		case grovecontext.RuleExcluded:
			cxStatus = core_theme.DefaultTheme.Error.Render("X")
		}
	}

	// --- BRANCH, GIT STATUS, CHANGES ---
	branch := "-"
	gitStatus := "-"
	changes := "-"
	if m.showBranch || m.showGitStatus {
		if project.EnrichmentStatus["git"] == "loading" {
			// Keep default dashes while loading to reduce visual noise
		} else {
			// Get git status data once for both branch and status/changes
			status := project.GetGitStatus()
			extStatus := project.GetExtendedGitStatus()

			if m.showBranch {
				if extStatus != nil && extStatus.StatusInfo != nil {
					branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
					branch = branchIcon + extStatus.StatusInfo.Branch
				}
			}
			if m.showGitStatus {
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

				// Reuse status and extStatus from above for changes calculation
				var changeParts []string

				// Add file counts for new, modified, and staged files
				if status != nil {
					if status.UntrackedCount > 0 {
						// New files (untracked)
						changeParts = append(changeParts, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("N:%d", status.UntrackedCount)))
					}
					if status.ModifiedCount > 0 {
						// Modified files (includes modified and deleted in working tree)
						changeParts = append(changeParts, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("M:%d", status.ModifiedCount)))
					}
					if status.StagedCount > 0 {
						// Staged files (includes all staged changes: adds, modifies, deletes)
						changeParts = append(changeParts, core_theme.DefaultTheme.Info.Render(fmt.Sprintf("S:%d", status.StagedCount)))
					}
				}

				// Add line changes if available
				if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
					added := core_theme.DefaultTheme.Success.Render(fmt.Sprintf("+%d", extStatus.LinesAdded))
					deleted := core_theme.DefaultTheme.Error.Render(fmt.Sprintf("-%d", extStatus.LinesDeleted))
					changeParts = append(changeParts, added+" "+deleted)
				}

				if len(changeParts) > 0 {
					changes = strings.Join(changeParts, " ")
				}
			}
		}
	}

	// --- NOTES ---
	notes := "-"
	if m.showNoteCounts && project.NoteCounts != nil {
		var parts []string
		if project.NoteCounts.Inbox > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", core_theme.IconNoteInbox, project.NoteCounts.Inbox))
		}
		if project.NoteCounts.Issues > 0 {
			parts = append(parts, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("%s %d", core_theme.IconNoteIssues, project.NoteCounts.Issues)))
		}
		if project.NoteCounts.Docs > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", core_theme.IconDocs, project.NoteCounts.Docs))
		}
		if project.NoteCounts.InProgress > 0 {
			parts = append(parts, core_theme.DefaultTheme.Warning.Render(fmt.Sprintf("%s %d", core_theme.IconNoteInProgress, project.NoteCounts.InProgress)))
		}
		if project.NoteCounts.Review > 0 {
			pinkStyle := lipgloss.NewStyle().Foreground(core_theme.DefaultTheme.Colors.Pink)
			parts = append(parts, pinkStyle.Render(fmt.Sprintf("%s %d", core_theme.IconNoteReview, project.NoteCounts.Review)))
		}
		if project.NoteCounts.Current > 0 {
			parts = append(parts, core_theme.DefaultTheme.Highlight.Render(fmt.Sprintf("%s %d", core_theme.IconNoteCurrent, project.NoteCounts.Current)))
		}
		if project.NoteCounts.Completed > 0 {
			parts = append(parts, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("%s %d", core_theme.IconNoteCompleted, project.NoteCounts.Completed)))
		}
		if project.NoteCounts.Other > 0 {
			parts = append(parts, core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("%s %d", core_theme.IconNote, project.NoteCounts.Other)))
		}
		if len(parts) > 0 {
			notes = strings.Join(parts, " ")
		}
	}

	// --- PLANS ---
	plans := "-"
	// Only show plan stats for repositories, not worktrees
	if m.showPlanStats && !project.IsWorktree() && project.PlanStats != nil {
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
			var statusStyle lipgloss.Style
			switch project.ClaudeSession.Status {
			case "running":
				statusSymbol = core_theme.IconRunning
				statusStyle = core_theme.DefaultTheme.Success
			case "idle":
				statusSymbol = core_theme.IconStatusHold
				statusStyle = core_theme.DefaultTheme.Warning
			case "completed":
				statusSymbol = core_theme.IconSuccess
				statusStyle = core_theme.DefaultTheme.Info
			case "failed", "error":
				statusSymbol = core_theme.IconError
				statusStyle = core_theme.DefaultTheme.Error
			default:
				statusStyle = core_theme.DefaultTheme.Muted
			}

			statusStyled := statusStyle.Render(statusSymbol)
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
				var statusStyle lipgloss.Style
				switch claudeStatus {
				case "running":
					statusSymbol = core_theme.IconRunning
					statusStyle = core_theme.DefaultTheme.Success
				case "idle":
					statusSymbol = core_theme.IconStatusHold
					statusStyle = core_theme.DefaultTheme.Warning
				case "completed":
					statusSymbol = core_theme.IconSuccess
					statusStyle = core_theme.DefaultTheme.Info
				case "failed", "error":
					statusSymbol = core_theme.IconError
					statusStyle = core_theme.DefaultTheme.Error
				default:
					statusStyle = core_theme.DefaultTheme.Muted
				}

				if statusSymbol != "" {
					statusStyled := statusStyle.Render(statusSymbol)
					claude = fmt.Sprintf("%s %s", statusStyled, claudeDuration)
				}
			} else {
				// Try normalized path match
				for path, status := range m.claudeStatusMap {
					normalizedPath, err := pathutil.NormalizeForLookup(path)
					if err == nil && normalizedPath == normalizedCleanPath {
						claudeStatus := status
						claudeDuration := ""
						if duration, foundDur := m.claudeDurationMap[path]; foundDur {
							claudeDuration = duration
						}

						statusSymbol := ""
						var statusStyle lipgloss.Style
						switch claudeStatus {
						case "running":
							statusSymbol = core_theme.IconRunning
							statusStyle = core_theme.DefaultTheme.Success
						case "idle":
							statusSymbol = core_theme.IconStatusHold
							statusStyle = core_theme.DefaultTheme.Warning
						case "completed":
							statusSymbol = core_theme.IconSuccess
							statusStyle = core_theme.DefaultTheme.Info
						case "failed", "error":
							statusSymbol = core_theme.IconError
							statusStyle = core_theme.DefaultTheme.Error
						default:
							statusStyle = core_theme.DefaultTheme.Muted
						}

						if statusSymbol != "" {
							statusStyled := statusStyle.Render(statusSymbol)
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
		pathDisplay = core_theme.DefaultTheme.Muted.Render(pathDisplay)
	}

	// Build row based on enabled columns
	row := []string{keyMapping, statusIndicator, cxStatus, workspaceName}

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
