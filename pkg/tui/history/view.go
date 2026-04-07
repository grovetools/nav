package history

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
)

const mutedThreshold = 7 * 24 * time.Hour // 1 week

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder
	b.WriteString(core_theme.DefaultTheme.Header.Render("Session History"))
	if m.isLoading {
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		b.WriteString(" " + spinner)
	}

	if m.filterMode {
		b.WriteString(" " + core_theme.DefaultTheme.Muted.Render("Filter: ") + m.filterText + "█")
	} else if m.filterText != "" {
		b.WriteString(" " + core_theme.DefaultTheme.Muted.Render("Filter: ") + m.filterText)
	}

	b.WriteString("\n\n")

	headers := []string{"#", "LAST ACCESSED", "Key", "Repository", "Branch/Worktree", "Git", "Ecosystem"}
	var rows [][]string

	for i, item := range m.filteredItems {
		var repository, worktree, gitStatus, ecosystem, k string

		projInfo := item.project

		if v, ok := m.keyMap[filepath.Clean(projInfo.Path)]; ok {
			k = v
		}
		if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
			repository = core_theme.DefaultTheme.Muted.Render(core_theme.IconRepo+" ") + filepath.Base(projInfo.ParentProjectPath)
			worktreeIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconWorktree + " ")
			worktree = worktreeIcon + projInfo.Name
		} else {
			icon := core_theme.IconRepo
			if projInfo.IsEcosystem() {
				icon = core_theme.IconEcosystem
			}
			repository = core_theme.DefaultTheme.Muted.Render(icon+" ") + projInfo.Name
		}

		if projInfo.ParentEcosystemPath != "" {
			if projInfo.RootEcosystemPath != "" {
				ecosystem = filepath.Base(projInfo.RootEcosystemPath)
			} else {
				ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
			}
		} else if projInfo.IsEcosystem() {
			ecosystem = projInfo.Name
		}

		branchWorktreeDisplay := worktree
		if branchWorktreeDisplay == "" && projInfo.GitStatus != nil && projInfo.GitStatus.StatusInfo.Branch != "" {
			branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
			branchWorktreeDisplay = branchIcon + projInfo.GitStatus.StatusInfo.Branch
		} else if branchWorktreeDisplay == "" {
			branchWorktreeDisplay = dimStyle.Render("n/a")
		}

		if projInfo.GitStatus != nil {
			gitStatus = formatChanges(projInfo.GitStatus.StatusInfo, projInfo.GitStatus)
		}

		row := []string{
			fmt.Sprintf("%d", i+1),
			formatRelativeTime(item.access.LastAccessed),
			k,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}

		if time.Since(item.access.LastAccessed) > mutedThreshold {
			style := lipgloss.NewStyle().Faint(true)
			for j, cell := range row {
				if j > 0 {
					row[j] = style.Render(cell)
				}
			}
		}

		rows = append(rows, row)
	}

	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{})
	b.WriteString(tableStr)
	b.WriteString("\n\n")
	if m.statusMessage != "" {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(m.statusMessage) + "\n")
	}
	b.WriteString(m.help.View())
	if m.jumpMode {
		b.WriteString(core_theme.DefaultTheme.Warning.Render(" [GOTO: _]"))
	}

	return pageStyle.Render(b.String())
}

// formatRelativeTime converts a time.Time to a human-readable string.
func formatRelativeTime(t time.Time) string {
	delta := time.Since(t)

	if delta < time.Minute {
		return fmt.Sprintf("%ds ago", int(delta.Seconds()))
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	}
	if delta < 7*24*time.Hour {
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
	return t.Format("2006-01-02")
}

// formatChanges formats the git status into a styled string. Mirrors the
// version in cmd/nav/tui_shared.go but lives here so the package is
// self-contained.
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

	if extStatus != nil && (extStatus.LinesAdded > 0 || extStatus.LinesDeleted > 0) {
		if extStatus.LinesAdded > 0 {
			changes = append(changes, core_theme.DefaultTheme.Success.Render(fmt.Sprintf("+%d", extStatus.LinesAdded)))
		}
		if extStatus.LinesDeleted > 0 {
			changes = append(changes, core_theme.DefaultTheme.Error.Render(fmt.Sprintf("-%d", extStatus.LinesDeleted)))
		}
	}

	changesStr := strings.Join(changes, " ")

	if !status.IsDirty && changesStr == "" {
		if status.HasUpstream {
			return core_theme.DefaultTheme.Success.Render(core_theme.IconSuccess)
		}
		return core_theme.DefaultTheme.Success.Render(core_theme.IconStatusTodo)
	}

	return changesStr
}
