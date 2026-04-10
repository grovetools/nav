package windows

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	core_theme "github.com/grovetools/core/tui/theme"
)

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Calculate layout: 50% for list, 50% for preview
	if m.width < 40 {
		return m.renderNarrow()
	}
	return m.renderWide()
}

func (m *Model) renderNarrow() string {
	var b strings.Builder
	if !m.EmbedMode {
		header := "Window Selector"
		if m.mode == "move" {
			header += " " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]")
		}
		b.WriteString(core_theme.DefaultTheme.Header.Render(header))
		b.WriteString("\n\n")
	} else if m.mode == "move" {
		b.WriteString(core_theme.DefaultTheme.Warning.Render("[MOVE MODE]") + "\n\n")
	}

	start, end := visibleRange(m.cursor, len(m.filteredWindows), m.height)

	for i := start; i < end; i++ {
		win := m.filteredWindows[i]
		cursor := " "
		if m.cursor == i {
			cursor = "→"
		}

		icon := getIconForWindow(win)

		name := win.Name
		if win.IsActive {
			name = core_theme.DefaultTheme.Highlight.Render(win.Name + " «")
		}

		line := fmt.Sprintf("%s %s %d: %s", cursor, icon, win.Index, name)

		if m.cursor == i && m.mode == "move" {
			b.WriteString(core_theme.DefaultTheme.Selected.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	if end-start < len(m.filteredWindows) {
		b.WriteString(core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("\n(%d-%d of %d)\n", start+1, end, len(m.filteredWindows))))
	} else {
		b.WriteString("\n")
	}

	switch m.mode {
	case "filter":
		b.WriteString("Filter: " + m.filterInput.View())
	case "rename":
		b.WriteString("Rename: " + m.renameInput.View())
	default:
		b.WriteString(m.help.View())
		if m.jumpMode {
			b.WriteString(core_theme.DefaultTheme.Warning.Render(" [GOTO: _]"))
		}
	}

	return b.String()
}

func (m *Model) renderWide() string {
	listWidth := m.width * 50 / 100
	if listWidth < 20 {
		listWidth = 20
	}
	if listWidth > m.width-20 {
		listWidth = m.width - 20
	}
	previewWidth := m.width - listWidth - 1 // -1 for separator
	if previewWidth < 10 {
		previewWidth = 10
	}

	var listBuilder strings.Builder
	if !m.EmbedMode {
		header := "Window Selector"
		if m.mode == "move" {
			header += " " + core_theme.DefaultTheme.Warning.Render("[MOVE MODE]")
		}
		listBuilder.WriteString(core_theme.DefaultTheme.Header.Render(header))
		listBuilder.WriteString("\n\n")
	} else if m.mode == "move" {
		listBuilder.WriteString(core_theme.DefaultTheme.Warning.Render("[MOVE MODE]") + "\n\n")
	}

	start, end := visibleRange(m.cursor, len(m.filteredWindows), m.height)

	for i := start; i < end; i++ {
		win := m.filteredWindows[i]
		cursor := " "
		if m.cursor == i {
			cursor = "→"
		}

		icon := getIconForWindow(win)

		name := win.Name
		if win.IsActive {
			name = core_theme.DefaultTheme.Highlight.Render(win.Name + " «")
		}

		line := fmt.Sprintf("%s %s %d: %s", cursor, icon, win.Index, name)

		processName := m.processCache[win.PID]
		if processName == "" {
			processName = win.Command
		}

		if shouldShowCommand(processName) {
			processStyle := core_theme.DefaultTheme.Muted
			line += " " + processStyle.Render(fmt.Sprintf("[%s]", processName))
		}

		if m.cursor == i && m.mode == "move" {
			listBuilder.WriteString(core_theme.DefaultTheme.Selected.Render(line))
		} else {
			listBuilder.WriteString(line)
		}
		listBuilder.WriteString("\n")
	}

	if end-start < len(m.filteredWindows) {
		listBuilder.WriteString(core_theme.DefaultTheme.Muted.Render(fmt.Sprintf("\n(%d-%d of %d)\n", start+1, end, len(m.filteredWindows))))
	} else {
		listBuilder.WriteString("\n")
	}

	switch m.mode {
	case "filter":
		listBuilder.WriteString("Filter: " + m.filterInput.View())
	case "rename":
		listBuilder.WriteString("Rename: " + m.renameInput.View())
	case "move":
		listBuilder.WriteString(core_theme.DefaultTheme.Muted.Render("Use j/k to reorder • Enter/Esc/m to apply"))
	default:
		listBuilder.WriteString(m.help.View())
		if m.jumpMode {
			listBuilder.WriteString(core_theme.DefaultTheme.Warning.Render(" [GOTO: _]"))
		}
	}

	var previewBuilder strings.Builder
	previewBuilder.WriteString(core_theme.DefaultTheme.Header.Render("Preview"))
	previewBuilder.WriteString("\n\n")

	maxPreviewHeight := m.height - 15
	if maxPreviewHeight < 5 {
		maxPreviewHeight = 5
	}

	previewLines := strings.Split(m.preview, "\n")
	lineCount := 0
	for _, line := range previewLines {
		if lineCount >= maxPreviewHeight {
			break
		}
		if len(line) > previewWidth {
			previewBuilder.WriteString(line[:previewWidth])
		} else {
			previewBuilder.WriteString(line)
		}
		previewBuilder.WriteString("\n")
		lineCount++
	}

	listStyle := lipgloss.NewStyle().Width(listWidth)
	previewStyle := lipgloss.NewStyle().Width(previewWidth)

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		listStyle.Render(listBuilder.String()),
		previewStyle.Render(previewBuilder.String()),
	)

	return pageStyle.Render(content)
}

// visibleRange computes the visible slice (start, end) given a cursor,
// total count, and viewport height. Matches the original packed layout:
// leaves room for the tab bar and caps at 15 lines.
func visibleRange(cursor, total, viewportHeight int) (int, int) {
	visibleHeight := viewportHeight - 15
	if visibleHeight < 5 {
		visibleHeight = 5
	}
	if visibleHeight > 15 {
		visibleHeight = 15
	}
	start := 0
	end := total
	if end > visibleHeight {
		start = cursor - visibleHeight/2
		if start < 0 {
			start = 0
		}
		end = start + visibleHeight
		if end > total {
			end = total
			start = end - visibleHeight
			if start < 0 {
				start = 0
			}
		}
	}
	return start, end
}

func shouldShowCommand(cmd string) bool {
	// Temporarily disabled - show all commands
	return true
}

func getIconForWindow(w tmuxclient.Window) string {
	// Check for special window name patterns first (highest priority)
	if strings.HasPrefix(w.Name, "job-") {
		return core_theme.IconRobot
	}
	if strings.Contains(w.Name, "code-review") {
		return core_theme.IconNoteReview
	}
	if strings.Contains(w.Name, "cx-edit") {
		return core_theme.IconFileTree
	}
	if strings.Contains(w.Name, "impl") || strings.Contains(w.Command, "impl") {
		return core_theme.IconInteractiveAgent
	}
	if strings.Contains(w.Name, "editor") {
		return core_theme.IconCode
	}
	if strings.Contains(w.Name, "notebook") {
		return core_theme.IconNotebook
	}
	if strings.Contains(w.Name, "term") {
		return core_theme.IconShell
	}
	if strings.Contains(w.Name, "plan") {
		return core_theme.IconPlan
	}

	if w.Command == "fish" {
		return core_theme.IconFish
	}

	return " "
}
