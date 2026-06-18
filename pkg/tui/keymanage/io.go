package keymanage

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	grovecontext "github.com/grovetools/cx/pkg/context"

	"github.com/grovetools/nav/pkg/api"
)

// ----- Exported cross-TUI messages ------------------------------------------
//
// These are the message types a host (the standalone nav TUI router, or
// the terminal embedder) must route to/from the keymanage model.

// RequestMapKeyMsg is delivered by the host when the user asks the
// sessionizer to map a project to a key. The keymanage model enters
// "pending map" mode, showing a prompt until the user picks a slot.
type RequestMapKeyMsg struct {
	Project *api.Project
}

// BulkMappingDoneMsg is delivered by the host after the sessionizer
// bulk-maps a batch of projects. The keymanage model refreshes and
// highlights the newly-mapped keys.
type BulkMappingDoneMsg struct {
	MappedKeys []string
}

// CancelMappingMsg is emitted by the keymanage model when the user
// presses ESC while in pending-map mode. Hosts should route this by
// switching back to the sessionizer view.
type CancelMappingMsg struct{}

// MappingDoneMsg is emitted by the keymanage model after the user
// successfully maps a project (handed off from the sessionizer) to a key.
// Hosts should route this by switching back to the sessionizer view, which
// triggers its focus refresh so the new (key) indicator appears immediately.
type MappingDoneMsg struct{}

// JumpToSessionizeMsg is emitted when the user asks to jump to the
// sessionizer view for a specific path (g/S keybindings). Hosts should
// switch to the sessionizer view and focus on the given path.
type JumpToSessionizeMsg struct {
	Path             string
	ApplyGroupFilter bool
}

// RequestManageGroupsMsg is emitted when the user asks to open the
// groups management view. Hosts should switch to whatever view they use
// for group management.
type RequestManageGroupsMsg struct{}

// ----- Internal messages ----------------------------------------------------

type initialProjectsEnrichedMsg struct {
	enrichedProjects map[string]*api.Project
	projectList      []*api.Project
}

type rulesStateMsg struct {
	rulesState map[string]grovecontext.RuleStatus
}

type cwdProjectEnrichedMsg struct {
	project *api.Project
}

type gitStatusMapMsg struct {
	statuses map[string]*git.ExtendedGitStatus
}

type noteCountsMapMsg struct {
	counts map[string]*models.NoteCounts
}

type planStatsMapMsg struct {
	stats map[string]*models.PlanStats
}

type spinnerTickMsg time.Time

type clearHighlightMsg struct{}

// ----- Internal tea.Cmds ----------------------------------------------------

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg(t)
	})
}

func clearHighlightCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearHighlightMsg{}
	})
}

// enrichInitialProjectsCmd turns each mapped session's path into a
// fully-populated *api.Project via workspace.GetProjectByPath. Always
// re-resolves from disk to avoid stale cache data (e.g. wrong Kind or
// Name for anchored/XDG worktrees whose lookup logic evolved). Cached
// entries are used only as a seed for enrichment fields (GitStatus,
// NoteCounts, etc.) that are expensive to re-fetch.
func enrichInitialProjectsCmd(sessions []models.TmuxSession, cached map[string]*api.Project) tea.Cmd {
	return func() tea.Msg {
		enriched := make(map[string]*api.Project)

		for _, s := range sessions {
			if s.Path == "" {
				continue
			}
			cleanPath, err := filepath.Abs(s.Path)
			if err != nil {
				continue
			}
			cleanPath = filepath.Clean(cleanPath)
			if _, exists := enriched[cleanPath]; exists {
				continue
			}
			if node, err := workspace.GetProjectByPath(s.Path); err == nil {
				proj := &api.Project{WorkspaceNode: node}
				if prev, ok := cached[cleanPath]; ok {
					proj.GitStatus = prev.GitStatus
					proj.NoteCounts = prev.NoteCounts
					proj.PlanStats = prev.PlanStats
					proj.ContextStatus = prev.ContextStatus
				}
				enriched[cleanPath] = proj
			} else if prev, ok := cached[cleanPath]; ok {
				enriched[cleanPath] = prev
			}
		}

		var list []*api.Project
		for _, proj := range enriched {
			list = append(list, proj)
		}
		return initialProjectsEnrichedMsg{enrichedProjects: enriched, projectList: list}
	}
}

// enrichCwdProjectCmd looks up the CWD path and wraps it in an
// *api.Project. Returns a cwdProjectEnrichedMsg with a nil project if
// the CWD is not a recognizable workspace node.
func enrichCwdProjectCmd(cwdPath string) tea.Cmd {
	return func() tea.Msg {
		node, err := workspace.GetProjectByPath(cwdPath)
		if err != nil {
			return cwdProjectEnrichedMsg{project: nil}
		}
		return cwdProjectEnrichedMsg{project: &api.Project{WorkspaceNode: node}}
	}
}

// fetchAllGitStatusesCmd fetches git status for each project in parallel
// (skipping projects that already have a cached GitStatus).
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

// fetchAllNoteCountsCmd fetches note counts via the daemon client.
// Returns an empty map if the daemon is not running.
func fetchAllNoteCountsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart() // inherit GROVE_SCOPE from treemux host
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		counts, _ := client.GetNoteCounts(ctx)
		return noteCountsMapMsg{counts: counts}
	}
}

// fetchAllPlanStatsCmd fetches plan statistics via the daemon client.
// Returns an empty map if the daemon is not running.
func fetchAllPlanStatsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewWithAutoStart() // inherit GROVE_SCOPE from treemux host
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stats, _ := client.GetPlanStats(ctx)
		return planStatsMapMsg{stats: stats}
	}
}

// fetchRulesStateCmd loads the context rules and determines the status
// for each project path (hot/cold/excluded/none).
func fetchRulesStateCmd(projects []*api.Project) tea.Cmd {
	return func() tea.Msg {
		mgr := grovecontext.NewManager("")
		state := make(map[string]grovecontext.RuleStatus)
		for _, p := range projects {
			rule := filepath.Join(p.Path, "**")
			state[p.Path] = mgr.GetRuleStatus(rule)
		}
		return rulesStateMsg{rulesState: state}
	}
}
