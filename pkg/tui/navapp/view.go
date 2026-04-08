package navapp

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	core_theme "github.com/grovetools/core/tui/theme"
)

// tabEntry describes one clickable tab in the header row.
type tabEntry struct {
	numIcon string
	name    string
	tab     Tab
}

var tabEntries = []tabEntry{
	{core_theme.IconNumeric1CircleOutline, "Sessionize", TabSessionize},
	{core_theme.IconNumeric2CircleOutline, "Key Manage", TabKeymanage},
	{core_theme.IconNumeric3CircleOutline, "History", TabHistory},
	{core_theme.IconNumeric4CircleOutline, "Windows", TabWindows},
	{core_theme.IconNumeric5CircleOutline, "Groups", TabGroups},
}

// View renders the tab bar followed by the active sub-model's view,
// wrapped in the standard 1x2 padding used by the nav CLI.
func (m *Model) View() string {
	var b strings.Builder

	var parts []string
	for _, tab := range tabEntries {
		if !m.tabAvailable(tab.tab) {
			continue
		}
		if m.activeTab == tab.tab {
			numStyle := lipgloss.NewStyle().
				Foreground(core_theme.DefaultTheme.Colors.Violet).
				Bold(true)
			nameStyle := lipgloss.NewStyle().
				Foreground(core_theme.DefaultTheme.Colors.LightText).
				Bold(true)
			parts = append(parts, fmt.Sprintf("%s %s",
				numStyle.Render(tab.numIcon),
				nameStyle.Render(tab.name)))
		} else {
			numStyle := lipgloss.NewStyle().
				Foreground(core_theme.DefaultTheme.Colors.MutedText)
			nameStyle := core_theme.DefaultTheme.Muted
			parts = append(parts, fmt.Sprintf("%s %s",
				numStyle.Render(tab.numIcon),
				nameStyle.Render(tab.name)))
		}
	}

	separator := core_theme.DefaultTheme.Muted.Faint(true).Render("  •  ")
	b.WriteString(strings.Join(parts, separator))
	b.WriteString("\n\n")

	var childView string
	switch m.activeTab {
	case TabSessionize:
		if m.sessionize != nil {
			childView = m.sessionize.View()
		}
	case TabKeymanage:
		if m.keymanage != nil {
			childView = m.keymanage.View()
		}
	case TabHistory:
		if m.history != nil {
			childView = m.history.View()
		}
	case TabWindows:
		if m.windows != nil {
			childView = m.windows.View()
		}
	case TabGroups:
		if m.groups != nil {
			childView = m.groups.View()
		}
	}

	b.WriteString(childView)
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
