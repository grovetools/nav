package windows

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
)

// LoadedMsg is emitted after the async window list fetch completes.
// Exported so host routers can identify it and forward it to this model
// even when it's not the currently-active view.
type LoadedMsg struct {
	Windows []tmuxclient.Window
}

// PreviewLoadedMsg is emitted after the async pane-capture preview
// completes.
type PreviewLoadedMsg struct {
	Preview string
}

// ErrorMsg is emitted when an async fetch fails fatally.
type ErrorMsg struct{ Err error }

// fetchWindowsCmd lists windows in the given session via the driver and
// returns a LoadedMsg.
func fetchWindowsCmd(driver SessionDriver, sessionName string) tea.Cmd {
	return func() tea.Msg {
		windows, err := driver.ListWindows(context.Background(), sessionName)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		sort.Slice(windows, func(i, j int) bool {
			return windows[i].Index < windows[j].Index
		})
		return LoadedMsg{Windows: windows}
	}
}

// fetchPreviewCmd captures a pane for preview via the driver.
func fetchPreviewCmd(driver SessionDriver, sessionName string, windowIndex int) tea.Cmd {
	return func() tea.Msg {
		target := fmt.Sprintf("%s:%d", sessionName, windowIndex)
		preview, err := driver.CapturePane(context.Background(), target)
		if err != nil {
			return PreviewLoadedMsg{Preview: fmt.Sprintf("Error: %v", err)}
		}
		return PreviewLoadedMsg{Preview: preview}
	}
}

// filterWindows returns the subset of windows matching the filter text
// (case-insensitive substring match on name).
func filterWindows(windows []tmuxclient.Window, filterText string) []tmuxclient.Window {
	filterText = strings.ToLower(filterText)
	if filterText == "" {
		return windows
	}
	var filtered []tmuxclient.Window
	for _, win := range windows {
		if strings.Contains(strings.ToLower(win.Name), filterText) {
			filtered = append(filtered, win)
		}
	}
	return filtered
}
