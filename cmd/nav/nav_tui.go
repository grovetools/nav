package main

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/compositor"
	"github.com/grovetools/core/pkg/daemon"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nav/internal/manager"
	"github.com/grovetools/nav/pkg/api"
	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/navapp"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
)

// NavTUIOptions contains options for initializing the nav TUI.
type NavTUIOptions struct {
	// CwdFocusPath is the path the sessionizer tab should focus on when
	// first opened — normally the current working directory's
	// ecosystem root.
	CwdFocusPath string
}

// runNavTUI runs the unified nav TUI starting in the keymanage tab.
// This matches the pre-extraction default used by `nav key manage`.
func runNavTUI() error {
	return runNavTUIWithTab(navapp.TabKeymanage, NavTUIOptions{})
}

// runNavTUIWithTab launches the navapp meta-panel focused on the given
// initial tab. Sub-models are built lazily by factories that capture
// the manager, tmux client, and cwd configured here.
func runNavTUIWithTab(initialTab navapp.Tab, opts NavTUIOptions) error {
	mgr, err := tmux.NewManager(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Try to create a tmux client (may fail if not in tmux).
	var client *tmuxclient.Client
	if os.Getenv("TMUX") != "" {
		client, _ = tmuxclient.NewClient()
	}

	// Auto-select the active group — CWD match first, then the
	// workspace's default group, then the last-accessed group.
	if targetGroup == "" {
		if matched := mgr.FindGroupForPath(cwd); matched != "" {
			mgr.SetActiveGroup(matched)
		} else if _, err := workspace.GetProjectByPath(cwd); err == nil {
			mgr.SetActiveGroup("default")
		} else if last := mgr.GetLastAccessedGroup(); last != "" {
			mgr.SetActiveGroup(last)
		} else {
			mgr.SetActiveGroup("default")
		}
	} else {
		mgr.SetActiveGroup(targetGroup)
	}

	// Forward-declare the model so the reentry hooks can call back
	// into its sub-model getters. The actual assignment happens below.
	var model *navapp.Model

	cfg := navapp.Config{
		InitialTab:    initialTab,
		NewSessionize: newSessionizeFactory(mgr, client, opts.CwdFocusPath),
		NewKeymanage:  newKeymanageFactory(mgr, client, cwd),
		NewHistory:    newHistoryFactory(mgr),
		NewGroups:     newGroupsFactory(mgr),
		OnReenterKeymanage: func() {
			if km := model.Keymanage(); km != nil {
				km.RefreshAfterGroupSwitch()
			}
		},
		OnReenterGroups: func() {
			if gm := model.Groups(); gm != nil {
				gm.Reset()
			}
		},
	}

	// Windows tab is only meaningful inside a tmux session.
	if client != nil {
		cfg.NewWindows = newWindowsFactory(client)
	}

	model = navapp.New(cfg)

	compModel := compositor.NewModel(model)
	p := tea.NewProgram(compModel, tea.WithAltScreen())
	finalModel, runErr := p.Run()

	// Free compositor resources and unwrap to recover the navapp.Model
	// so post-exit type assertions succeed.
	if cm, ok := finalModel.(*compositor.Model); ok {
		cm.Free()
		finalModel = cm.Unwrap()
	}

	// Tear down any SSE / background listeners held by sub-models.
	// Navapp fans Close() out to every initialized sub-model.
	if nm, ok := finalModel.(*navapp.Model); ok {
		_ = nm.Close()
	}

	// Clear daemon focus so it stops high-frequency scanning post-TUI.
	// Use zero-arg so the client inherits GROVE_SCOPE from env — nav
	// run ad-hoc from a shell goes to the global daemon, nav run
	// inside a treemux pane scopes to treemux's daemon.
	clientDaemon := daemon.NewWithAutoStart()
	if clientDaemon.IsRunning() {
		ctxDaemon, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = clientDaemon.SetFocus(ctxDaemon, []string{})
		cancel()
	}
	clientDaemon.Close()

	if runErr != nil {
		return fmt.Errorf("error running program: %w", runErr)
	}

	return handlePostExit(finalModel, mgr, client)
}

// handlePostExit dispatches the various "what did the user pick?" tail
// actions that used to live inline at the bottom of runNavTUIWithView —
// keymanage save-on-exit, sessionize jump, history jump, and windows
// switch.
func handlePostExit(finalModel tea.Model, mgr *tmux.Manager, client *tmuxclient.Client) error {
	nm, ok := finalModel.(*navapp.Model)
	if !ok {
		return nil
	}

	// Keymanage save-on-exit.
	if km := nm.Keymanage(); km != nil {
		if km.ChangesMade() {
			if err := mgr.UpdateSessionsAndLocks(km.Sessions(), km.LockedKeysSlice()); err != nil {
				return fmt.Errorf("failed to save sessions: %w", err)
			}
			if err := mgr.RegenerateBindings(); err != nil {
				return fmt.Errorf("failed to regenerate bindings: %w", err)
			}
			_ = reloadTmuxConfig()
		}
		if cmdOnExit := km.CommandOnExit(); cmdOnExit != nil {
			cmdOnExit.Stdin = os.Stdin
			cmdOnExit.Stdout = os.Stdout
			cmdOnExit.Stderr = os.Stderr
			_ = cmdOnExit.Run()
		}
	}

	// Sessionize jump.
	if sz := nm.Sessionize(); sz != nil {
		if selected := sz.Selected(); selected != nil &&
			selected.WorkspaceNode != nil && selected.Path != "" {
			_ = mgr.RecordProjectAccess(selected.Path)
			if selected.IsWorktree() && selected.ParentProjectPath != "" {
				_ = mgr.RecordProjectAccess(selected.ParentProjectPath)
			}
			return sessionizeProject(selected)
		}
	}

	// History jump.
	if hm := nm.History(); hm != nil {
		if selected := hm.Selected(); selected != nil {
			_ = mgr.RecordProjectAccess(selected.Path)
			return mgr.Sessionize(selected.Path)
		}
	}

	// Windows switch.
	if wm := nm.Windows(); wm != nil && wm.SelectedWindow() != nil && client != nil {
		target := fmt.Sprintf("%s:%d", wm.SessionName(), wm.SelectedWindow().Index)
		_ = client.SwitchClient(context.Background(), target)
		_ = client.ClosePopupCmd().Run()
	}

	return nil
}

// newSessionizeFactory builds the sessionizer sub-model factory. The
// factory reads the project cache (or falls back to the full project
// loader) on first access so cold start and warm start produce the
// same sorted, ecosystem-grouped project list.
func newSessionizeFactory(mgr *tmux.Manager, client *tmuxclient.Client, cwdFocusPath string) navapp.SessionizeFactory {
	return func() *sessionizer.Model {
		usedCache := false
		var projects []manager.SessionizeProject

		if cache, err := manager.LoadProjectCache(configDir); err == nil && cache != nil && len(cache.Projects) > 0 {
			projects = make([]manager.SessionizeProject, len(cache.Projects))
			for i, cached := range cache.Projects {
				projects[i] = manager.SessionizeProject{
					WorkspaceNode: cached.WorkspaceNode,
					GitStatus:     cached.GitStatus,
					NoteCounts:    cached.NoteCounts,
					PlanStats:     cached.PlanStats,
					ReleaseInfo:   cached.ReleaseInfo,
					ActiveBinary:  cached.ActiveBinary,
					CxStats:       cached.CxStats,
					GitRemoteURL:  cached.GitRemoteURL,
				}
			}
			usedCache = true
		}

		// Cold start: run the full project loader so we get the same
		// sorting and virtual-ecosystem grouping as cache rebuilds.
		// A raw GetAvailableProjects fetch would skip
		// SortProjectsByAccess and groupClonedProjectsAsEcosystem.
		if len(projects) == 0 {
			loader := buildProjectLoader(mgr, configDir)
			if ptrs, err := loader(); err == nil && len(ptrs) > 0 {
				projects = make([]manager.SessionizeProject, len(ptrs))
				for i, p := range ptrs {
					projects[i] = *p
				}
			}
		}

		if len(projects) == 0 {
			return nil
		}

		projectPtrs := make([]*api.Project, len(projects))
		for i := range projects {
			projectPtrs[i] = &projects[i]
		}
		searchPaths, _ := mgr.GetEnabledSearchPaths()
		currentSession := ""
		if client != nil {
			if cur, err := client.GetCurrentSession(context.Background()); err == nil {
				currentSession = cur
			}
		}
		driver := NewTmuxDriver(client)
		cwd, _ := os.Getwd()
		cfg := sessionizer.Config{
			Store:                mgr,
			SessionDriver:        driver,
			SessionStateProvider: driver,
			ConfigDir:            configDir,
			SearchPaths:          searchPaths,
			Features:             mgr.GetResolvedFeatures(),
			CwdFocusPath:         cwdFocusPath,
			ActiveWorkspacePath:  cwd,
			UsedCache:            usedCache,
			CurrentSession:       currentSession,
			LoadProjects:         buildProjectLoader(mgr, configDir),
			ReloadConfig:         reloadTmuxConfig,
			KeyMap:               sessionizeKeys,
		}
		return sessionizer.New(cfg, projectPtrs)
	}
}

// newKeymanageFactory builds the keymanage sub-model factory. It warms
// the enriched-project cache so the table populates without a stall
// on first paint.
func newKeymanageFactory(mgr *tmux.Manager, client *tmuxclient.Client, cwd string) navapp.KeymanageFactory {
	return func() *keymanage.Model {
		enrichedProjects := make(map[string]*api.Project)
		usedCache := false
		if cache, err := api.LoadKeyManageCache(configDir); err == nil && cache != nil && len(cache.EnrichedProjects) > 0 {
			for path, cached := range cache.EnrichedProjects {
				enrichedProjects[path] = &api.Project{
					WorkspaceNode: cached.WorkspaceNode,
					GitStatus:     cached.GitStatus,
					NoteCounts:    cached.NoteCounts,
					PlanStats:     cached.PlanStats,
				}
			}
			usedCache = len(enrichedProjects) > 0
		}

		var driver keymanage.SessionDriver
		if client != nil {
			driver = NewTmuxDriver(client)
		}

		return keymanage.New(keymanage.Config{
			Store:            mgr,
			SessionDriver:    driver,
			ConfigDir:        configDir,
			CwdPath:          cwd,
			Features:         mgr.GetResolvedFeatures(),
			EnrichedProjects: enrichedProjects,
			UsedCache:        usedCache,
			ReloadConfig:     reloadTmuxConfig,
			KeyMap:           manageKeys,
		})
	}
}

// newHistoryFactory builds the history sub-model factory.
func newHistoryFactory(mgr *tmux.Manager) navapp.HistoryFactory {
	return func() *history.Model {
		loader := buildHistoryLoader(mgr)
		initialItems, _ := loader()
		return history.New(history.Config{
			LoadHistory:  loader,
			InitialItems: initialItems,
			KeyMapView:   buildHistoryKeyMap(mgr),
			KeyMap:       historyKeys,
		})
	}
}

// newWindowsFactory builds the windows sub-model factory. The caller
// must only supply this factory when a live tmux client is present.
func newWindowsFactory(client *tmuxclient.Client) navapp.WindowsFactory {
	return func() *windows.Model {
		ctx := context.Background()
		currentSession, err := client.GetCurrentSession(ctx)
		if err != nil || currentSession == "" {
			return nil
		}
		showChildProcesses := false
		if tmuxCfg, err := loadTmuxConfig(); err == nil && tmuxCfg != nil {
			showChildProcesses = tmuxCfg.ShowChildProcesses
		}
		return windows.New(windows.Config{
			Driver:             newWindowsDriver(client),
			SessionName:        currentSession,
			ShowChildProcesses: showChildProcesses,
			KeyMap:             windowsKeys,
		})
	}
}

// newGroupsFactory builds the groups sub-model factory.
func newGroupsFactory(mgr *tmux.Manager) navapp.GroupsFactory {
	return func() *groups.Model {
		return groups.New(groups.Config{
			Store:        mgr,
			ReloadConfig: reloadTmuxConfig,
			KeyMap:       groupsKeys,
		})
	}
}
