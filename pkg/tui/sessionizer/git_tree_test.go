package sessionizer

import (
	"testing"

	"github.com/grovetools/core/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func childNames(n *GitChangeNode) []string {
	names := make([]string, len(n.Children))
	for i, c := range n.Children {
		names[i] = c.Name
	}
	return names
}

func child(t *testing.T, n *GitChangeNode, name string) *GitChangeNode {
	t.Helper()
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	require.Failf(t, "missing child", "%q not found under %q (have %v)", name, n.Name, childNames(n))
	return nil
}

func TestBuildGitChangeTree(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		root := buildGitChangeTree(map[string][]git.FileStatus{})
		assert.Empty(t, root.Children)
	})

	t.Run("nested paths under one repo", func(t *testing.T) {
		root := buildGitChangeTree(map[string][]git.FileStatus{
			"/code/grovetools/flow": {
				{Path: "pkg/plan_finish/factory.go", Staged: '.', Working: 'M'},
				{Path: "pkg/io.go", Staged: 'A', Working: '.'},
				{Path: "README.md", Staged: '?', Working: '?'},
			},
		})

		require.Len(t, root.Children, 1)
		repo := root.Children[0]
		assert.True(t, repo.IsRepo)
		assert.Equal(t, "flow", repo.Name)
		assert.Equal(t, "/code/grovetools/flow", repo.Path)

		// Directory `pkg` sorts before file `README.md`.
		assert.Equal(t, []string{"pkg", "README.md"}, childNames(repo))

		pkg := child(t, repo, "pkg")
		// Directory `plan_finish` before file `io.go`.
		assert.Equal(t, []string{"plan_finish", "io.go"}, childNames(pkg))

		factory := child(t, child(t, pkg, "plan_finish"), "factory.go")
		// Leaf nodes store the absolute path (repo root + relative file path).
		assert.Equal(t, "/code/grovetools/flow/pkg/plan_finish/factory.go", factory.Path)
		assert.Equal(t, 'M', factory.Status.Working)
		assert.Empty(t, factory.Children)

		readme := child(t, repo, "README.md")
		assert.Equal(t, '?', readme.Status.Working)
	})

	t.Run("multiple repos sorted", func(t *testing.T) {
		root := buildGitChangeTree(map[string][]git.FileStatus{
			"/code/grovetools/nav":  {{Path: "main.go", Working: 'M'}},
			"/code/grovetools/core": {{Path: "git/status.go", Working: 'M'}},
		})

		require.Len(t, root.Children, 2)
		// Sorted by repo path: core before nav.
		assert.Equal(t, []string{"core", "nav"}, childNames(root))
	})

	t.Run("repo with no changes is skipped", func(t *testing.T) {
		root := buildGitChangeTree(map[string][]git.FileStatus{
			"/code/grovetools/clean": {},
			"/code/grovetools/dirty": {{Path: "x.go", Working: 'M'}},
		})
		require.Len(t, root.Children, 1)
		assert.Equal(t, "dirty", root.Children[0].Name)
	})
}
