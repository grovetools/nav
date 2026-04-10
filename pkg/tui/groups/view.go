package groups

import (
	"fmt"
	"strings"

	"github.com/grovetools/core/tui/components/table"
	core_theme "github.com/grovetools/core/tui/theme"
)

func (m *Model) View() string {
	// If help is visible, show it
	if m.help.ShowAll {
		return pageStyle.Render(m.help.View())
	}

	var b strings.Builder

	if !m.EmbedMode {
		title := fmt.Sprintf("%s Group Management", core_theme.IconKeyboard)
		b.WriteString(core_theme.DefaultTheme.Header.Render(title))
		if m.moveMode {
			b.WriteString(" " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]"))
		}
		b.WriteString("\n\n")
	} else if m.moveMode {
		b.WriteString(core_theme.DefaultTheme.Warning.Render("[MOVE MODE]") + "\n\n")
	}

	// Build table
	headers := []string{"#", "Name", "Prefix Key", "Sessions", "Status"}
	var rows [][]string

	for i, g := range m.groups {
		var icon, prefix, status string
		sessionCount := m.manager.GetGroupSessionCount(g)

		if g == "default" {
			if defIcon := m.manager.GetDefaultIcon(); defIcon != "" {
				icon = resolveIcon(defIcon)
			} else {
				icon = core_theme.IconHome
			}
			prefix = resolvePrefixDisplay(m.manager.GetPrefixForGroup("default"))
			status = ""
		} else {
			rawIcon := m.manager.GetGroupIcon(g)
			if rawIcon != "" {
				icon = resolveIcon(rawIcon)
			} else {
				icon = core_theme.IconFolderStar
			}
			prefix = resolvePrefixDisplay(m.manager.GetPrefixForGroup(g))
			if m.manager.IsGroupExplicitlyInactive(g) {
				status = core_theme.DefaultTheme.Muted.Render("(Inactive)")
			}
		}

		displayName := g
		if icon != "" {
			displayName = icon + " " + g
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			displayName,
			prefix,
			fmt.Sprintf("%d", sessionCount),
			status,
		})
	}

	tableStr := table.SelectableTableWithOptions(headers, rows, m.cursor, table.SelectableTableOptions{})
	b.WriteString(tableStr)
	b.WriteString("\n\n")

	// Input mode UI
	if m.inputMode != "" {
		b.WriteString(core_theme.DefaultTheme.Header.Render(m.message))
		b.WriteString("\n")
		b.WriteString("  ")
		b.WriteString(m.input.View())
		b.WriteString("\n")
		b.WriteString(core_theme.DefaultTheme.Muted.Render("  (Enter to confirm, Esc to cancel)"))
		b.WriteString("\n\n")
	} else if m.confirmMode {
		b.WriteString(core_theme.DefaultTheme.Warning.Render("  ⚠ " + m.message))
		b.WriteString("\n\n")
	} else if m.message != "" {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(m.message))
		b.WriteString("\n")
	}

	// Help
	b.WriteString(m.help.View())

	return pageStyle.Render(b.String())
}
