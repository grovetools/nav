package sessionizer

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	grovecontext "github.com/grovetools/cx/pkg/context"
	"github.com/grovetools/nav/pkg/api"
)

// makeProject builds a minimal *api.Project for use in tests.
func makeProject(path, name string, kind workspace.WorkspaceKind, parentEco, parentProject string, status *git.StatusInfo) *api.Project {
	p := &api.Project{
		WorkspaceNode: &workspace.WorkspaceNode{
			Path:                path,
			Name:                name,
			Kind:                kind,
			ParentEcosystemPath: parentEco,
			ParentProjectPath:   parentProject,
		},
	}
	if status != nil {
		p.GitStatus = &git.ExtendedGitStatus{StatusInfo: status}
	}
	return p
}

// TestUpdateFiltered_ScaffoldFold verifies that the scaffold fold correctly
// partitions active/anchor vs clean children of an EcosystemWorktree container.
func TestUpdateFiltered_ScaffoldFold(t *testing.T) {
	const (
		ecoPath       = "/eco"
		ecoFlowPath   = "/eco/flow"
		containerPath = "/wt/eco/feature"
		anchorPath    = "/wt/eco/feature/flow"
		cleanPath     = "/wt/eco/feature/core"
		dirtyPath     = "/wt/eco/feature/nav"
	)

	// Container: KindEcosystemWorktree anchored on "flow"
	container := makeProject(containerPath, "feature", workspace.KindEcosystemWorktree, ecoPath, ecoFlowPath, nil)
	// Anchor child: clean, but must never fold
	anchor := makeProject(anchorPath, "flow", workspace.KindEcosystemWorktreeSubProject, containerPath, "", nil)
	// Clean sibling: no active changes → should be hidden when scaffoldFolded
	clean := makeProject(cleanPath, "core", workspace.KindEcosystemWorktreeSubProject, containerPath, "", nil)
	// Dirty sibling: active → must be visible
	dirty := makeProject(dirtyPath, "nav", workspace.KindEcosystemWorktreeSubProject, containerPath, "",
		&git.StatusInfo{IsDirty: true, AheadCount: 2, BehindMainCount: 3},
	)

	projects := []*api.Project{container, anchor, clean, dirty}
	projectMap := make(map[string]*api.Project, len(projects))
	for _, p := range projects {
		projectMap[p.Path] = p
	}

	ti := textinput.New()
	m := &Model{
		projects:         projects,
		projectMap:       projectMap,
		filterInput:      ti,
		scaffoldFolded:   true,
		foldedPaths:      make(map[string]bool),
		hasChildren:      make(map[string]bool),
		contextOnlyPaths: make(map[string]bool),
		keyMap:           make(map[string]string),
		selectedPaths:    make(map[string]bool),
		rulesState:       make(map[string]grovecontext.RuleStatus),
	}
	m.updateFiltered()

	// Container + anchor (flow) + dirty (nav) = 3 visible; clean (core) hidden
	if got := len(m.filtered); got != 3 {
		t.Errorf("filtered length = %d, want 3", got)
	}

	// Check container transient fields
	if container.HiddenCleanCount != 1 {
		t.Errorf("container.HiddenCleanCount = %d, want 1", container.HiddenCleanCount)
	}
	if container.AggregateAhead != 2 {
		t.Errorf("container.AggregateAhead = %d, want 2 (dirty child AheadCount=2)", container.AggregateAhead)
	}
	if container.AggregateBehind != 3 {
		t.Errorf("container.AggregateBehind = %d, want 3 (dirty child BehindMainCount=3)", container.AggregateBehind)
	}

	// Verify the anchor and dirty child appear (not the clean one)
	filtered := make(map[string]bool)
	for _, p := range m.filtered {
		filtered[p.Path] = true
	}
	if !filtered[containerPath] {
		t.Error("container should be visible")
	}
	if !filtered[anchorPath] {
		t.Error("anchor child should always be visible")
	}
	if !filtered[dirtyPath] {
		t.Error("dirty child should be visible")
	}
	if filtered[cleanPath] {
		t.Error("clean non-anchor child should be hidden when scaffoldFolded=true")
	}
}

// TestUpdateFiltered_ScaffoldUnfold verifies that all children are visible when
// scaffoldFolded is false.
func TestUpdateFiltered_ScaffoldUnfold(t *testing.T) {
	const (
		ecoPath       = "/eco"
		ecoFlowPath   = "/eco/flow"
		containerPath = "/wt/eco/feature"
		anchorPath    = "/wt/eco/feature/flow"
		cleanPath     = "/wt/eco/feature/core"
		dirtyPath     = "/wt/eco/feature/nav"
	)

	container := makeProject(containerPath, "feature", workspace.KindEcosystemWorktree, ecoPath, ecoFlowPath, nil)
	anchor := makeProject(anchorPath, "flow", workspace.KindEcosystemWorktreeSubProject, containerPath, "", nil)
	clean := makeProject(cleanPath, "core", workspace.KindEcosystemWorktreeSubProject, containerPath, "", nil)
	dirty := makeProject(dirtyPath, "nav", workspace.KindEcosystemWorktreeSubProject, containerPath, "",
		&git.StatusInfo{IsDirty: true},
	)

	projects := []*api.Project{container, anchor, clean, dirty}
	projectMap := make(map[string]*api.Project, len(projects))
	for _, p := range projects {
		projectMap[p.Path] = p
	}

	ti := textinput.New()
	m := &Model{
		projects:         projects,
		projectMap:       projectMap,
		filterInput:      ti,
		scaffoldFolded:   false,
		foldedPaths:      make(map[string]bool),
		hasChildren:      make(map[string]bool),
		contextOnlyPaths: make(map[string]bool),
		keyMap:           make(map[string]string),
		selectedPaths:    make(map[string]bool),
		rulesState:       make(map[string]grovecontext.RuleStatus),
	}
	m.updateFiltered()

	// All 4 should appear
	if got := len(m.filtered); got != 4 {
		t.Errorf("filtered length = %d, want 4 (scaffoldFolded=false shows all)", got)
	}
	if container.HiddenCleanCount != 0 {
		t.Errorf("container.HiddenCleanCount = %d, want 0 when not folding", container.HiddenCleanCount)
	}
}
