package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	core_theme "github.com/mattsolo1/grove-core/tui/theme"
)

// renderTree renders the tree view for projects with full styling
func (m sessionizeModel) renderTree() string {
	if len(m.filtered) == 0 {
		return ""
	}

	var b strings.Builder

	// Calculate visible items based on terminal height
	// Reserve space for: header (3 lines), help (1 line), search paths (2 lines)
	visibleHeight := m.height - 6
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

	// Render visible projects
	for i := start; i < end && i < len(m.filtered); i++ {
		project := m.filtered[i]

		// Check if this project has a key mapping
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

		// Check if session exists for this project
		sessionName := project.Identifier()
		sessionExists := m.runningSessions[sessionName]

		// Get Claude session status
		var claudeStatusStyled string
		var claudeDuration string

		// Check if this is a Claude session project
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

			claudeStatusStyled = lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			claudeDuration = project.ClaudeSession.Duration
		} else if m.hasGroveHooks {
			// Regular project - check if it has any Claude sessions
			claudeStatus := ""
			if status, found := m.claudeStatusMap[cleanPath]; found {
				claudeStatus = status
				if duration, foundDur := m.claudeDurationMap[cleanPath]; foundDur {
					claudeDuration = duration
				}
			} else {
				// Try case-insensitive match on macOS
				for path, status := range m.claudeStatusMap {
					if strings.EqualFold(path, cleanPath) {
						claudeStatus = status
						if duration, foundDur := m.claudeDurationMap[path]; foundDur {
							claudeDuration = duration
						}
						break
					}
				}
			}

			// Style the claude status (without duration - that goes at the end)
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
				claudeStatusStyled = lipgloss.NewStyle().Foreground(statusColor).Render(statusSymbol)
			} else {
				claudeStatusStyled = " " // Empty space to maintain alignment
			}
		}

		// Get Git status string
		var changesStr, branchName string
		if m.showGitStatus || m.showBranch {
			if project.EnrichmentStatus["git"] == "loading" {
				changesStr = core_theme.DefaultTheme.Info.Render("◐")
			} else {
				extStatus := project.GetExtendedGitStatus()
				changesStr = formatChanges(project.GetGitStatus(), extStatus)
				if m.showBranch && extStatus != nil && extStatus.StatusInfo != nil {
					branchName = extStatus.StatusInfo.Branch
				}
			}
		}

		// Prepare display elements
		prefix := "  "
		displayName := project.Name

		// Determine prefix based on mode
		if m.ecosystemPickerMode {
			// In ecosystem picker mode - show tree structure
			if project.IsWorktree() {
				// Check if this is the last worktree of its parent
				isLast := true
				for j := i + 1; j < len(m.filtered); j++ {
					if m.filtered[j].IsWorktree() && m.filtered[j].ParentProjectPath == project.ParentProjectPath {
						isLast = false
						break
					}
				}
				if isLast {
					prefix = "  └─ "
				} else {
					prefix = "  ├─ "
				}
			} else {
				// Main ecosystem - no prefix
				prefix = "  "
			}
		} else if m.focusedProject != nil {
			// In focus mode
			if project.Path == m.focusedProject.Path {
				// This is the focused ecosystem/worktree - show as parent
				prefix = "  "
			} else if project.IsWorktree() {
				// This is a worktree - check if it's a direct child or nested
				if project.ParentProjectPath == m.focusedProject.Path {
					// Direct worktree of the focused ecosystem
					prefix = "  └─ "
				} else {
					// Worktree of a repo within the focused ecosystem - show nested with extra indent
					prefix = "     └─ "
				}
			} else {
				// Regular repo within the focused ecosystem - show as child
				prefix = "  ├─ "
			}
		} else {
			// Normal mode - show worktree indicator
			if project.IsWorktree() {
				prefix = "  └─ "
			}
		}

		// If this is a Claude session, add PID to the name
		if project.ClaudeSession != nil {
			displayName = fmt.Sprintf("%s [PID:%d]", project.Name, project.ClaudeSession.PID)
		}

		// Highlight matching search terms
		filter := strings.ToLower(m.filterInput.Value())
		if filter != "" {
			displayName = highlightMatch(displayName, filter)
		}

		if i == m.cursor {
			// Highlight selected line
			indicator := core_theme.DefaultTheme.Highlight.Render("▶ ")

			nameStyle := core_theme.DefaultTheme.Selected
			pathStyle := core_theme.DefaultTheme.Info

			keyIndicator := "  " // Default: 2 spaces
			if keyMapping != "" {
				keyIndicator = core_theme.DefaultTheme.Highlight.Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Blue).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Green).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("%s%s%s", indicator, keyIndicator, sessionIndicator)
			if m.hasGroveHooks && m.showClaudeSessions {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)

			if branchName != "" {
				mutedSelectedStyle := core_theme.DefaultTheme.Selected.Copy().Foreground(core_theme.DefaultTheme.Muted.GetForeground())
				line += " " + mutedSelectedStyle.Render("("+branchName+")")
			}

			// Add path based on display mode (0=none, 1=compact, 2=full)
			if m.pathDisplayMode > 0 {
				displayPath := project.Path
				if m.pathDisplayMode == 1 {
					// Compact: replace home with ~
					displayPath = strings.Replace(displayPath, os.Getenv("HOME"), "~", 1)
				}
				line += "  " + pathStyle.Render(displayPath)
			}

			// Add git status if enabled
			if m.showGitStatus && changesStr != "" {
				line += "  " + changesStr
			}

			// Add note counts if enabled
			if m.showNoteCounts && project.NoteCounts != nil {
				var counts []string
				if project.NoteCounts.Current > 0 {
					counts = append(counts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Violet).Render(fmt.Sprintf("⟦C:%d⟧", project.NoteCounts.Current)))
				}
				if project.NoteCounts.Issues > 0 {
					counts = append(counts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Pink).Render(fmt.Sprintf("⟦I:%d⟧", project.NoteCounts.Issues)))
				}
				if len(counts) > 0 {
					line += "  " + strings.Join(counts, " ")
				}
			}

			// Add plan stats if enabled
			if m.showPlanStats && project.PlanStats != nil {
				statsStr := formatPlanStats(project.PlanStats)
				if statsStr != "" {
					line += "  " + statsStr
				}
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && m.showClaudeSessions && claudeDuration != "" {
				line += "  " + core_theme.DefaultTheme.Muted.Render(claudeDuration)
			}

			b.WriteString(line)
		} else {
			// Normal line with colored name - style based on project type
			var nameStyle lipgloss.Style
			if project.IsWorktree() {
				// Worktrees: Blue
				nameStyle = lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Blue)
			} else {
				// Primary repos: Cyan
				nameStyle = lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Cyan)
			}
			pathStyle := core_theme.DefaultTheme.Muted

			// Always reserve space for key indicator
			keyIndicator := "  "
			if keyMapping != "" {
				keyIndicator = core_theme.DefaultTheme.Highlight.Render(fmt.Sprintf("%s ", keyMapping))
			}

			sessionIndicator := " "
			if sessionExists {
				// Check if this is the current session
				if sessionName == m.currentSession {
					// Current session - use blue indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Blue).
						Render("●")
				} else {
					// Other active session - use green indicator
					sessionIndicator = lipgloss.NewStyle().
						Foreground(core_theme.DefaultColors.Green).
						Render("●")
				}
			}

			// Build the line
			line := fmt.Sprintf("  %s%s", keyIndicator, sessionIndicator)
			if m.hasGroveHooks && m.showClaudeSessions {
				line += fmt.Sprintf(" %s", claudeStatusStyled)
			}
			line += " "
			if prefix != "" {
				line += prefix
			}
			line += nameStyle.Render(displayName)

			if branchName != "" {
				line += " " + core_theme.DefaultTheme.Muted.Render("("+branchName+")")
			}

			// Add path based on display mode (0=none, 1=compact, 2=full)
			if m.pathDisplayMode > 0 {
				displayPath := project.Path
				if m.pathDisplayMode == 1 {
					// Compact: replace home with ~
					displayPath = strings.Replace(displayPath, os.Getenv("HOME"), "~", 1)
				}
				line += "  " + pathStyle.Render(displayPath)
			}

			// Add git status if enabled
			if m.showGitStatus && changesStr != "" {
				line += "  " + changesStr
			}

			// Add note counts if enabled
			if m.showNoteCounts && project.NoteCounts != nil {
				var counts []string
				if project.NoteCounts.Current > 0 {
					counts = append(counts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Violet).Render(fmt.Sprintf("⟦C:%d⟧", project.NoteCounts.Current)))
				}
				if project.NoteCounts.Issues > 0 {
					counts = append(counts, lipgloss.NewStyle().Foreground(core_theme.DefaultColors.Pink).Render(fmt.Sprintf("⟦I:%d⟧", project.NoteCounts.Issues)))
				}
				if len(counts) > 0 {
					line += "  " + strings.Join(counts, " ")
				}
			}

			// Add plan stats if enabled
			if m.showPlanStats && project.PlanStats != nil {
				statsStr := formatPlanStats(project.PlanStats)
				if statsStr != "" {
					line += "  " + statsStr
				}
			}

			// Add Claude duration at the very end
			if m.hasGroveHooks && m.showClaudeSessions && claudeDuration != "" {
				line += "  " + core_theme.DefaultTheme.Muted.Render(claudeDuration)
			}

			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	// Show scroll indicators if needed
	if start > 0 || end < len(m.filtered) {
		scrollInfo := fmt.Sprintf(" (%d-%d of %d)", start+1, end, len(m.filtered))
		b.WriteString(core_theme.DefaultTheme.Muted.Render(scrollInfo))
	}

	return b.String()
}
