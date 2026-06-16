package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/tui"
	"github.com/grovetools/tend/pkg/verify"
)

// ----------------------------------------------------------------------------
// Anchor-nesting + scaffold-fold E2E coverage (nav commit 498f499)
//
// Feature under test (pkg/tui/sessionizer):
//   - An ecosystem worktree container (Kind == KindEcosystemWorktree) whose
//     ParentProjectPath points at a sub-repo nests UNDER that sub-repo in the
//     sessionize tree (GetHierarchicalParent returns the owner).
//   - The scaffold fold reuses the vim-fold system (foldedPaths) — the single
//     source of truth. Anchor groups (Kind == KindEcosystemWorktree) START
//     FOLDED by default (seeded in NewModel), so the default view is the clean
//     SUMMARY: under the container, only ACTIVE children show (git IsDirty ||
//     UntrackedCount>0 || AheadCount>0 || AheadMainCount>0); clean/behind-only
//     siblings collapse behind a "[+N clean]" badge on the container row.
//   - The ANCHOR child (child.Name == filepath.Base(container.ParentProjectPath))
//     never folds and carries the IconArrowRightBold marker ("=>" in ASCII mode).
//   - Per-group "zo" (FoldOpen) on the container reveals the FULL repo list
//     (the clean scaffold child appears).
//   - Key "A" bulk-toggles ALL anchor groups between summary (folded) and full
//     (unfolded), operating on the same foldedPaths map.
//
// Discovery strategy: rather than depend on grove's disk-scan classifier
// producing exactly the right WorkspaceKind / ParentProjectPath wiring from an
// on-disk XDG worktree layout (brittle, daemon-dependent), we drive the
// production daemon client (RemoteClient -> /api/workspaces) with a mock daemon
// that serves a hand-crafted []EnrichedWorkspace. This pins the hierarchy and
// per-child git status precisely, so the assertions exercise the sessionizer's
// fold/anchor presentation rather than the upstream classifier. This mirrors
// the existing delta scenarios (scenarios_delta.go).
//
// GROVE_ICONS=ascii is forced so the anchor marker is the deterministic "=>"
// rather than a nerd-font glyph whose presence depends on terminal detection.
// ----------------------------------------------------------------------------

// anchorWorkspace is the JSON shape consumed by the daemon RemoteClient at
// /api/workspaces. It inlines the workspace.WorkspaceNode fields and adds the
// enrichment GitStatus, matching models.EnrichedWorkspace's wire format.
type anchorWorkspace struct {
	Name                string         `json:"name"`
	Path                string         `json:"path"`
	Kind                string         `json:"kind"`
	ParentProjectPath   string         `json:"parent_project_path,omitempty"`
	ParentEcosystemPath string         `json:"parent_ecosystem_path,omitempty"`
	RootEcosystemPath   string         `json:"root_ecosystem_path,omitempty"`
	GitStatus           *anchorGitStat `json:"git_status,omitempty"`
}

type anchorGitStat struct {
	Branch          string `json:"branch"`
	IsDirty         bool   `json:"is_dirty"`
	ModifiedCount   int    `json:"modified_count"`
	UntrackedCount  int    `json:"untracked_count"`
	AheadCount      int    `json:"ahead_count"`
	AheadMainCount  int    `json:"ahead_main_count"`
	BehindMainCount int    `json:"behind_main_count"`
}

// buildAnchorWorkspacesJSON constructs the workspace tree for the anchor
// scenario. Layout (paths are real dirs created under projectsDir):
//
//	eco/                              EcosystemRoot
//	eco/flow/                         EcosystemSubProject   (the ANCHOR repo, clean)
//	eco/.grove-worktrees/feature/     EcosystemWorktree     (container, owner = eco/flow)
//	  ├─ flow                         anchor child          (clean, never folds, marker)
//	  ├─ nav                          sub-project           (DIRTY -> active, visible)
//	  └─ core                         sub-project           (clean -> scaffold, hidden)
func buildAnchorWorkspacesJSON(projectsDir string) (json.RawMessage, map[string]string) {
	ecoPath := filepath.Join(projectsDir, "eco")
	anchorRepoPath := filepath.Join(ecoPath, "flow")
	containerPath := filepath.Join(ecoPath, ".grove-worktrees", "feature")
	childFlowPath := filepath.Join(containerPath, "flow")
	childNavPath := filepath.Join(containerPath, "nav")
	childCorePath := filepath.Join(containerPath, "core")

	ws := []anchorWorkspace{
		{
			Name:              "eco",
			Path:              ecoPath,
			Kind:              "EcosystemRoot",
			RootEcosystemPath: ecoPath,
		},
		{
			// The owning sub-repo. The container nests UNDER this row.
			Name:                "flow",
			Path:                anchorRepoPath,
			Kind:                "EcosystemSubProject",
			ParentEcosystemPath: ecoPath,
			RootEcosystemPath:   ecoPath,
		},
		{
			// The ecosystem worktree container, anchored to eco/flow.
			Name:                "feature",
			Path:                containerPath,
			Kind:                "EcosystemWorktree",
			ParentProjectPath:   anchorRepoPath, // owner == anchor
			ParentEcosystemPath: ecoPath,
			RootEcosystemPath:   ecoPath,
		},
		{
			// Anchor child: name matches base(container.ParentProjectPath)=="flow".
			// Clean, but must never fold and must carry the marker.
			Name:              "flow",
			Path:              childFlowPath,
			Kind:              "EcosystemWorktreeSubProject",
			ParentProjectPath: containerPath,
		},
		{
			// Active child: dirty -> always visible, contributes to aggregate.
			Name:              "nav",
			Path:              childNavPath,
			Kind:              "EcosystemWorktreeSubProject",
			ParentProjectPath: containerPath,
			GitStatus: &anchorGitStat{
				Branch:         "feature",
				IsDirty:        true,
				ModifiedCount:  2,
				AheadCount:     0,
				AheadMainCount: 3,
			},
		},
		{
			// Scaffold child: clean -> hidden behind "[+N clean]" when folded.
			Name:              "core",
			Path:              childCorePath,
			Kind:              "EcosystemWorktreeSubProject",
			ParentProjectPath: containerPath,
		},
	}

	data, _ := json.Marshal(ws)

	paths := map[string]string{
		"eco":          ecoPath,
		"anchor_repo":  anchorRepoPath,
		"container":    containerPath,
		"child_flow":   childFlowPath,
		"child_nav":    childNavPath,
		"child_core":   childCorePath,
		"projects_dir": projectsDir,
	}
	return data, paths
}

// setupAnchorTestEnv writes grove.yml + empty sessions.yml and materializes the
// fixture directories on disk (so any os.Stat the binary performs succeeds).
func setupAnchorTestEnv(ctx *harness.Context) (json.RawMessage, map[string]string, error) {
	projectsDir := filepath.Join(ctx.RootDir, "projects")
	if err := fs.CreateDir(projectsDir); err != nil {
		return nil, nil, fmt.Errorf("create projects dir: %w", err)
	}

	wsJSON, paths := buildAnchorWorkspacesJSON(projectsDir)

	// Materialize the directories so the paths physically exist.
	for _, key := range []string{"eco", "anchor_repo", "container", "child_flow", "child_nav", "child_core"} {
		if err := fs.CreateDir(paths[key]); err != nil {
			return nil, nil, fmt.Errorf("create fixture dir %s: %w", paths[key], err)
		}
	}

	groveYAML := fmt.Sprintf(`version: "1.0"
groves:
  test_projects:
    path: %s
    enabled: true
tmux:
  available_keys: [a, b, c, d, e, f]
`, projectsDir)

	groveConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
	if err := fs.CreateDir(groveConfigDir); err != nil {
		return nil, nil, fmt.Errorf("create grove config dir: %w", err)
	}
	if err := fs.WriteString(filepath.Join(groveConfigDir, "grove.yml"), groveYAML); err != nil {
		return nil, nil, fmt.Errorf("write grove.yml: %w", err)
	}

	navStateDir := filepath.Join(ctx.StateDir(), "grove", "nav")
	if err := fs.CreateDir(navStateDir); err != nil {
		return nil, nil, fmt.Errorf("create nav state dir: %w", err)
	}
	if err := fs.WriteString(filepath.Join(navStateDir, "sessions.yml"), "sessions:\n  # No sessions configured\n"); err != nil {
		return nil, nil, fmt.Errorf("write sessions.yml: %w", err)
	}

	return wsJSON, paths, nil
}

// NavSzAnchorScaffoldFoldScenario covers anchor nesting + scaffold fold + marker.
func NavSzAnchorScaffoldFoldScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "nav-sz-anchor-scaffold-fold",
		Description: "Ecosystem worktree nests under its anchor sub-repo; clean scaffold children fold behind [+N clean]; anchor carries the marker; 'A' toggles the fold",
		LocalOnly:   true, // TUI test requires tmux
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Drive anchored sessionize and assert fold + marker + toggle", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				wsJSON, paths, err := setupAnchorTestEnv(ctx)
				if err != nil {
					return err
				}

				// Mock daemon serving our hand-crafted workspace tree.
				daemon, err := startMockDaemonWithJSON(ctx.RuntimeDir(), wsJSON)
				if err != nil {
					return fmt.Errorf("start mock daemon: %w", err)
				}
				defer daemon.Close()

				navBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("find nav binary: %w", err)
				}

				// Force ASCII icons so the anchor marker is the deterministic "=>".
				session, err := ctx.StartTUI(navBinary, []string{"sz"}, tui.WithEnv("GROVE_ICONS=ascii"))
				if err != nil {
					return fmt.Errorf("start nav sz: %w", err)
				}

				// Brief settle so the tmux pane is allocated before the first
				// capture — avoids an intermittent "failed to capture pane" race
				// when polling a pane that hasn't finished its initial draw.
				time.Sleep(1 * time.Second)

				if err := session.WaitForText("WORKSPACE", 20*time.Second); err != nil {
					content, _ := session.Capture()
					ctx.ShowCommandOutput("TUI screen on failure", content, "")
					return fmt.Errorf("WORKSPACE header never rendered: %w", err)
				}
				// Let enrichment / tree build settle.
				time.Sleep(2 * time.Second)
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize: %w", err)
				}

				folded, err := session.Capture()
				if err != nil {
					return fmt.Errorf("capture folded screen: %w", err)
				}
				ctx.ShowCommandOutput("Folded (default: anchor group seeded folded)", folded, "")

				// (a) Assert the DEFAULT folded (summary) state.
				if verr := ctx.Verify(func(v *verify.Collector) {
					// Hierarchy + active children visible.
					v.Contains("anchor repo 'flow' present", folded, "flow")
					v.Contains("container 'feature' present", folded, "feature")
					v.Contains("active child 'nav' visible", folded, "nav")
					// Scaffold child folded away.
					v.NotContains("clean scaffold child 'core' hidden when folded", folded, "core")
					// Fold badge on the container row (one clean non-anchor child: core).
					v.Contains("scaffold fold badge present", folded, "[+1 clean]")
					// Anchor marker (ASCII IconArrowRightBold) somewhere in the tree.
					v.Contains("anchor marker present", folded, "=>")
				}); verr != nil {
					return verr
				}

				// (b) Per-group FoldOpen: move the cursor onto the 'feature'
				// container row, then press 'zo'. The default tree (folded) is
				// eco(0) -> flow anchor-repo(1) -> feature container(2), so two
				// 'j' presses land on the container.
				if err := session.SendKeys("j"); err != nil {
					return fmt.Errorf("send 'j': %w", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := session.SendKeys("j"); err != nil {
					return fmt.Errorf("send 'j': %w", err)
				}
				time.Sleep(300 * time.Millisecond)
				if err := session.SendKeys("zo"); err != nil {
					return fmt.Errorf("send 'zo' fold-open: %w", err)
				}
				time.Sleep(1500 * time.Millisecond)
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize after zo: %w", err)
				}

				perGroupOpen, err := session.Capture()
				if err != nil {
					return fmt.Errorf("capture per-group unfold screen: %w", err)
				}
				ctx.ShowCommandOutput("Per-group unfold (after 'zo' on container)", perGroupOpen, "")

				if verr := ctx.Verify(func(v *verify.Collector) {
					// FoldOpen on the container reveals the FULL repo list — the
					// previously hidden clean scaffold child now shows. Load-bearing
					// proof that per-group vim-fold drives the scaffold view.
					v.Contains("clean scaffold child 'core' visible after zo", perGroupOpen, "core")
					// Badge gone once nothing is hidden under the (now unfolded) group.
					v.NotContains("fold badge removed after zo", perGroupOpen, "[+1 clean]")
					// Anchor still present.
					v.Contains("anchor repo 'flow' still present", perGroupOpen, "flow")
				}); verr != nil {
					return verr
				}

				// (c) Bulk toggle with 'A': with the group unfolded, 'A' folds ALL
				// anchor groups back to summary (clean child hidden, badge returns).
				if err := session.SendKeys("A"); err != nil {
					return fmt.Errorf("send 'A' bulk-fold: %w", err)
				}
				time.Sleep(1500 * time.Millisecond)
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize after first 'A': %w", err)
				}
				bulkFolded, err := session.Capture()
				if err != nil {
					return fmt.Errorf("capture bulk-folded screen: %w", err)
				}
				ctx.ShowCommandOutput("Bulk-folded (after 'A')", bulkFolded, "")
				if verr := ctx.Verify(func(v *verify.Collector) {
					v.NotContains("clean scaffold child 'core' hidden after bulk-fold", bulkFolded, "core")
					v.Contains("fold badge back after bulk-fold", bulkFolded, "[+1 clean]")
				}); verr != nil {
					return verr
				}

				// 'A' again: bulk-unfold ALL anchor groups back to the full list.
				if err := session.SendKeys("A"); err != nil {
					return fmt.Errorf("send 'A' bulk-unfold: %w", err)
				}
				time.Sleep(1500 * time.Millisecond)
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("TUI did not stabilize after second 'A': %w", err)
				}
				bulkUnfolded, err := session.Capture()
				if err != nil {
					return fmt.Errorf("capture bulk-unfolded screen: %w", err)
				}
				ctx.ShowCommandOutput("Bulk-unfolded (after second 'A')", bulkUnfolded, "")

				_ = paths // fixture path map retained for diagnostics/future assertions

				return ctx.Verify(func(v *verify.Collector) {
					// Bulk-unfold reveals the clean scaffold child again across all
					// anchor groups via the shared foldedPaths map.
					v.Contains("clean scaffold child 'core' visible after bulk-unfold", bulkUnfolded, "core")
					v.NotContains("fold badge removed after bulk-unfold", bulkUnfolded, "[+1 clean]")
					v.Contains("anchor repo 'flow' still present", bulkUnfolded, "flow")
				})
			}),
		},
	}
}
