package keymanage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
)

// View renders the keymanage TUI. It implements the tea.Model contract.
func (m *Model) View() string {
	if m.quitting && m.message != "" {
		return m.message + "\n"
	}

	// If help is visible, show it and return.
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder

	// Title with key mapping.
	if !m.EmbedMode {
		prefix := m.store.GetPrefix()
		var hotkey string
		switch prefix {
		case "<prefix>":
			hotkey = "C-b → key"
		case "<grove>":
			hotkey = "C-g → key"
		case "":
			hotkey = "no prefix"
		default:
			if strings.HasPrefix(prefix, "<prefix> ") {
				key := strings.TrimPrefix(prefix, "<prefix> ")
				hotkey = fmt.Sprintf("C-b %s → key", key)
			} else if strings.HasPrefix(prefix, "<grove> ") {
				key := strings.TrimPrefix(prefix, "<grove> ")
				hotkey = fmt.Sprintf("C-g %s → key", key)
			} else {
				hotkey = fmt.Sprintf("%s → key", prefix)
			}
		}

		title := fmt.Sprintf("%s Session Hotkeys (%s)", core_theme.IconKeyboard, hotkey)
		if m.pendingMapProject != nil {
			title += " - Map: " + m.pendingMapProject.Name
		}
		b.WriteString(core_theme.DefaultTheme.Header.Render(title))
	}

	// Render group tabs if groups feature is enabled and multiple groups exist.
	groups := m.store.GetGroups()
	if m.features.Groups && len(groups) > 1 {
		b.WriteString("\n")
		activeGroup := m.store.GetActiveGroup()
		var tabs []string
		for _, g := range groups {
			iconStr := ""
			if g == "default" {
				if defIcon := m.store.GetDefaultIcon(); defIcon != "" {
					iconStr = resolveIcon(defIcon) + " "
				} else {
					iconStr = core_theme.IconHome + " "
				}
			} else {
				if icon := m.store.GetGroupIcon(g); icon != "" {
					iconStr = resolveIcon(icon) + " "
				} else {
					iconStr = core_theme.IconFolderStar + " "
				}
			}

			tabText := iconStr + g

			isMoveTarget := m.moveToGroupMode && m.moveToGroupCursor < len(m.moveToGroupOptions) && m.moveToGroupOptions[m.moveToGroupCursor] == g
			isLoadTarget := m.loadFromGroupMode && m.loadFromGroupCursor < len(m.loadFromGroupOptions) && m.loadFromGroupOptions[m.loadFromGroupCursor] == g

			if g == activeGroup {
				arrow := core_theme.DefaultTheme.Highlight.Render(core_theme.IconArrowRightBold)
				tabs = append(tabs, arrow+" "+core_theme.DefaultTheme.Highlight.Render(tabText))
			} else if isMoveTarget || isLoadTarget {
				arrow := core_theme.DefaultTheme.Success.Render(core_theme.IconArrow)
				tabs = append(tabs, arrow+" "+core_theme.DefaultTheme.Success.Render(tabText))
			} else {
				tabs = append(tabs, "  "+core_theme.DefaultTheme.Muted.Render(tabText))
			}
		}
		b.WriteString(strings.Join(tabs, core_theme.DefaultTheme.Muted.Render(" │")))
	}

	if m.moveMode {
		b.WriteString(" " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]"))
	}
	b.WriteString("\n")

	// Separate sessions into unlocked and locked.
	var unlockedSessions, lockedSessions []models.TmuxSession
	for _, s := range m.sessions {
		if m.lockedKeys[s.Key] {
			if defaultSession, ok := m.defaultLockedSessions[s.Key]; ok {
				lockedSessions = append(lockedSessions, defaultSession)
			} else {
				lockedSessions = append(lockedSessions, s)
			}
		} else {
			unlockedSessions = append(unlockedSessions, s)
		}
	}

	spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]

	gitHeader := "Git"
	if m.enrichmentLoading["git"] {
		gitHeader = "Git " + spinner
	}

	headers := []string{"#", "Key", "Repository", "Branch/Worktree", gitHeader, "Ecosystem"}
	if m.pathDisplayMode > 0 {
		headers = append(headers, "Path")
	}

	unlockedRows := m.buildRows(unlockedSessions, true)
	lockedRows := m.buildRows(lockedSessions, false)

	cursorInUnlocked := m.cursor < len(unlockedSessions)
	var adjustedCursor int
	if cursorInUnlocked {
		adjustedCursor = m.cursor
	} else {
		adjustedCursor = m.cursor - len(unlockedSessions)
	}

	if len(unlockedRows) > 0 {
		var str string
		if cursorInUnlocked {
			str = table.SelectableTableWithOptions(headers, unlockedRows, adjustedCursor, table.SelectableTableOptions{})
		} else {
			str = table.SelectableTableWithOptions(headers, unlockedRows, -1, table.SelectableTableOptions{})
		}
		b.WriteString(str)
		b.WriteString("\n")
	}

	if len(lockedRows) > 0 {
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Muted.Render(core_theme.IconLock + " Locked"))
		b.WriteString("\n")

		var str string
		if !cursorInUnlocked {
			str = table.SelectableTableWithOptions(headers, lockedRows, adjustedCursor, table.SelectableTableOptions{})
		} else {
			str = table.SelectableTableWithOptions(headers, lockedRows, -1, table.SelectableTableOptions{})
		}
		b.WriteString(str)
	}

	b.WriteString("\n\n")

	if m.confirmMode != "" {
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Warning.Render("  ⚠ " + m.message))
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Success.Render("    [Y]es") + "  " + core_theme.DefaultTheme.Error.Render("[N]o / Esc"))
		b.WriteString("\n\n")
	}

	if m.loadFromGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Load from Group") + "\n")
		for i, opt := range m.loadFromGroupOptions {
			if i == m.loadFromGroupCursor {
				b.WriteString(core_theme.DefaultTheme.Selected.Render("  → " + opt))
			} else {
				b.WriteString(dimStyle.Render("    " + opt))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.newGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Create New Group") + "\n")
		if m.newGroupStep == 0 {
			b.WriteString("  New group name: ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.newGroupName + "█"))
		} else {
			b.WriteString(fmt.Sprintf("  Group name: %s\n", m.newGroupName))
			b.WriteString("  Prefix key (optional): ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.newGroupPrefix + "█"))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("  e.g. '<grove> g' → C-g g key"))
		}
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("  (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	}

	if m.saveToGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Save to Group") + "\n")
		if m.saveToGroupNewMode {
			b.WriteString("  New group name: ")
			b.WriteString(core_theme.DefaultTheme.Selected.Render(m.saveToGroupInput + "█"))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("  (Enter to confirm, Esc to cancel)"))
		} else {
			for i, opt := range m.saveToGroupOptions {
				if i == m.saveToGroupCursor {
					b.WriteString(core_theme.DefaultTheme.Selected.Render("  → " + opt))
				} else {
					b.WriteString(dimStyle.Render("    " + opt))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	if m.moveToGroupMode {
		b.WriteString(core_theme.DefaultTheme.Header.Render("Move to Group") + "\n")
		for i, opt := range m.moveToGroupOptions {
			if i == m.moveToGroupCursor {
				b.WriteString(core_theme.DefaultTheme.Selected.Render("  → " + opt))
			} else {
				b.WriteString(dimStyle.Render("    " + opt))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.confirmMode == "" {
		if m.pendingMapProject != nil {
			b.WriteString("\n")
			b.WriteString(core_theme.DefaultTheme.Magenta.Render("  ▶ Select slot for '" + m.pendingMapProject.Name + "', then press 'e' or Enter to map"))
			b.WriteString("\n")
			b.WriteString(core_theme.DefaultTheme.Muted.Render("    ESC to cancel"))
			b.WriteString("\n")
		} else if m.message != "" {
			b.WriteString(dimStyle.Render(m.message) + "\n")
		} else {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("\n")
	}

	var modeIndicator string
	switch {
	case m.jumpMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [GOTO: _]")
	case m.pendingMapProject != nil:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [MAPPING]")
	case m.moveMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [MOVE MODE]")
	case m.setKeyMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [SET KEY MODE]")
	case m.saveToGroupMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [SAVE TO GROUP]")
	case m.moveToGroupMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [MOVE TO GROUP]")
	case m.loadFromGroupMode:
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [LOAD FROM GROUP]")
	case m.confirmMode != "":
		modeIndicator = core_theme.DefaultTheme.Warning.Render(" [CONFIRM]")
	}
	b.WriteString(m.help.View() + modeIndicator)

	return pageStyle.Render(b.String())
}

// buildRows assembles the table row slices for either the unlocked or
// locked session section. withJustMappedHighlight is true for the
// unlocked section so newly-mapped keys get the success icon treatment.
func (m *Model) buildRows(sessions []models.TmuxSession, withJustMappedHighlight bool) [][]string {
	var rows [][]string
	for i, s := range sessions {
		var ecosystem, repository, worktree string
		gitStatus := ""

		if s.Path != "" {
			cleanPath := filepath.Clean(s.Path)
			if projInfo, found := m.enrichedProjects[cleanPath]; found {
				// Repository/worktree determination.
				if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
					parentName := filepath.Base(projInfo.ParentProjectPath)
					parentIcon := core_theme.IconRepo
					if parentProj, found := m.enrichedProjects[projInfo.ParentProjectPath]; found {
						if parentProj.Kind == workspace.KindEcosystemRoot {
							parentIcon = core_theme.IconEcosystem
						}
						if parentProj.RepoShorthand != "" {
							parts := strings.Split(parentProj.RepoShorthand, "/")
							parentName = parts[len(parts)-1]
						}
					}
					parentIconStyled := core_theme.DefaultTheme.Muted.Render(parentIcon + " ")
					repository = parentIconStyled + parentName

					worktreeIcon := core_theme.IconWorktree
					worktreeIconStyled := core_theme.DefaultTheme.Muted.Render(worktreeIcon + " ")
					worktree = worktreeIconStyled + projInfo.Name
				} else {
					icon := core_theme.IconRepo
					if projInfo.Kind == workspace.KindEcosystemRoot {
						icon = core_theme.IconEcosystem
					}
					iconStyled := core_theme.DefaultTheme.Muted.Render(icon + " ")
					repository = iconStyled + projInfo.Name
				}

				// Ecosystem column.
				if projInfo.ParentEcosystemPath != "" {
					if projInfo.RootEcosystemPath != "" {
						ecosystem = filepath.Base(projInfo.RootEcosystemPath)
					} else {
						ecosystem = filepath.Base(projInfo.ParentEcosystemPath)
					}
					if ecosystem == "cx" {
						ecosystem = "cx-repos"
					}

					if projInfo.RootEcosystemPath != "" && projInfo.ParentEcosystemPath != projInfo.RootEcosystemPath {
						ecoWorktreeName := filepath.Base(projInfo.ParentEcosystemPath)
						if projInfo.IsWorktree() && projInfo.ParentProjectPath != "" {
							repository = filepath.Base(projInfo.ParentProjectPath)
							worktree = ecoWorktreeName
						} else {
							repository = projInfo.Name
							worktree = ecoWorktreeName + " *"
						}
					}
				} else if projInfo.IsEcosystem() {
					ecosystem = projInfo.Name
				}

				if projInfo.GitStatus != nil {
					gitStatus = formatChanges(projInfo.GitStatus.StatusInfo, projInfo.GitStatus)
				}
			} else {
				repository = filepath.Base(s.Path)
			}
		}

		branchWorktreeDisplay := worktree
		if branchWorktreeDisplay == "" && repository != "" {
			if s.Path != "" {
				cleanPath := filepath.Clean(s.Path)
				if projInfo, found := m.enrichedProjects[cleanPath]; found {
					if projInfo.GitStatus != nil && projInfo.GitStatus.StatusInfo != nil && projInfo.GitStatus.StatusInfo.Branch != "" {
						branchIcon := core_theme.DefaultTheme.Muted.Render(core_theme.IconGitBranch + " ")
						branchWorktreeDisplay = branchIcon + projInfo.GitStatus.StatusInfo.Branch
					} else {
						branchWorktreeDisplay = dimStyle.Render("n/a")
					}
				} else {
					branchWorktreeDisplay = dimStyle.Render("n/a")
				}
			} else {
				branchWorktreeDisplay = dimStyle.Render("n/a")
			}
		}

		keyDisplay := s.Key
		if withJustMappedHighlight && m.justMappedKeys[s.Key] {
			keyDisplay = core_theme.DefaultTheme.Success.Render(core_theme.IconSuccess + " " + s.Key)
		}

		selectionIndicator := fmt.Sprintf("%d", i+1)
		if m.selectedKeys[s.Key] {
			selectionIndicator = core_theme.DefaultTheme.Success.Render("✓")
		}

		row := []string{
			selectionIndicator,
			keyDisplay,
			repository,
			branchWorktreeDisplay,
			gitStatus,
			ecosystem,
		}
		if m.pathDisplayMode > 0 {
			pathStr := ""
			if s.Path != "" {
				if m.pathDisplayMode == 1 {
					pathStr = strings.Replace(s.Path, os.Getenv("HOME"), "~", 1)
				} else {
					pathStr = s.Path
				}
			}
			row = append(row, pathStr)
		}
		rows = append(rows, row)
	}
	return rows
}

// resolveIcon converts icon references to actual icon characters.
// Supports preset names like "IconEcosystem" or direct unicode characters.
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

// formatChanges formats the git status into a styled string. Duplicated
// from the cmd/nav/tui_shared.go helper so this package stays free of
// cmd/nav imports.
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
