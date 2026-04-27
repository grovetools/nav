package history

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/git"

	"github.com/grovetools/nav/pkg/api"
)

// historyLoadedMsg is emitted after HistoryLoader resolves.
type historyLoadedMsg struct {
	items []Item
	err   error
}

// gitStatusMapMsg carries a map of path -> extended git status after an
// async batch fetch completes.
type gitStatusMapMsg struct {
	statuses map[string]*git.ExtendedGitStatus
}

// spinnerTickMsg drives the loading spinner animation.
type spinnerTickMsg time.Time

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

// loadHistoryCmd invokes a HistoryLoader off the Bubble Tea thread and
// returns a historyLoadedMsg.
func loadHistoryCmd(loader HistoryLoader) tea.Cmd {
	return func() tea.Msg {
		items, err := loader()
		return historyLoadedMsg{items: items, err: err}
	}
}

// fetchAllGitStatusesCmd kicks off a batch fetch of extended git status
// for every project in the list. Results arrive as a single
// gitStatusMapMsg.
func fetchAllGitStatusesCmd(projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		var wg sync.WaitGroup
		var mu sync.Mutex
		statuses := make(map[string]*git.ExtendedGitStatus)
		semaphore := make(chan struct{}, 10)

		for _, p := range projects {
			if p.GitStatus != nil {
				mu.Lock()
				statuses[p.Path] = p.GitStatus
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func(proj *api.Project) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				status, err := git.GetExtendedStatus(proj.Path)
				if err == nil {
					mu.Lock()
					statuses[proj.Path] = status
					mu.Unlock()
				}
			}(p)
		}
		wg.Wait()
		return gitStatusMapMsg{statuses: statuses}
	}
}

// filterItems returns the subset of items matching the given filter
// text. Matching is case-insensitive and checks name, branch, path, and
// root-ecosystem-base components.
func filterItems(items []historyItem, filterText string) []historyItem {
	if filterText == "" {
		return items
	}
	var filtered []historyItem
	filterLower := strings.ToLower(filterText)
	for _, item := range items {
		projInfo := item.project
		if strings.Contains(strings.ToLower(projInfo.Name), filterLower) ||
			(projInfo.GitStatus != nil && strings.Contains(strings.ToLower(projInfo.GitStatus.StatusInfo.Branch), filterLower)) ||
			strings.Contains(strings.ToLower(projInfo.Path), filterLower) ||
			(projInfo.RootEcosystemPath != "" && strings.Contains(strings.ToLower(projInfo.RootEcosystemPath), filterLower)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
