package sessionizer

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/git"
)

// GitChangeNode is a node in the transient trie built from per-repo changed
// files. The neo-tree style overlay renders this hierarchy: repo -> dir -> file.
//
// Intermediate directory nodes carry only Name (Path is empty and Status is
// zero). Leaf file nodes carry the full Path and the git.FileStatus. Repo nodes
// have IsRepo set and Path pointing at the repository root.
type GitChangeNode struct {
	Name     string
	Path     string // full path for a file (or repo root); empty for intermediate dirs
	Status   git.FileStatus
	IsRepo   bool
	Children []*GitChangeNode
}

// buildGitChangeTree assembles a flat map of per-repo changed files into a
// trie keyed by repo root path. Each repo becomes a top-level child of the
// returned (synthetic) root node; within a repo, each changed file's path is
// split on filepath.Separator into directory nodes terminating in a file node.
//
// Children are sorted with directories before files, each alphabetically, so
// the rendered tree is deterministic.
func buildGitChangeTree(repoChanges map[string][]git.FileStatus) *GitChangeNode {
	root := &GitChangeNode{}

	// Deterministic repo ordering.
	repoPaths := make([]string, 0, len(repoChanges))
	for repoPath := range repoChanges {
		repoPaths = append(repoPaths, repoPath)
	}
	sort.Strings(repoPaths)

	for _, repoPath := range repoPaths {
		files := repoChanges[repoPath]
		if len(files) == 0 {
			continue
		}

		repoNode := &GitChangeNode{
			Name:   filepath.Base(repoPath),
			Path:   repoPath,
			IsRepo: true,
		}
		root.Children = append(root.Children, repoNode)

		for _, f := range files {
			insertFile(repoNode, f)
		}
	}

	sortChildren(root)
	return root
}

// insertFile walks (creating as needed) the directory chain for f.Path under
// repoNode, attaching a leaf node holding the file's status at the end.
func insertFile(repoNode *GitChangeNode, f git.FileStatus) {
	parts := strings.Split(filepath.ToSlash(f.Path), "/")
	cur := repoNode
	for i, part := range parts {
		if part == "" {
			continue
		}
		isLeaf := i == len(parts)-1
		child := findChild(cur, part)
		if child == nil {
			child = &GitChangeNode{Name: part}
			cur.Children = append(cur.Children, child)
		}
		if isLeaf {
			child.Path = f.Path
			child.Status = f
		}
		cur = child
	}
}

// findChild returns the existing child of parent with the given name, or nil.
func findChild(parent *GitChangeNode, name string) *GitChangeNode {
	for _, c := range parent.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// isDir reports whether a node is an intermediate directory (has children and
// no file path of its own).
func (n *GitChangeNode) isDir() bool {
	return len(n.Children) > 0 && n.Path == "" && !n.IsRepo
}

// sortChildren recursively orders children: repos/dirs first, then files,
// each group sorted by name.
func sortChildren(n *GitChangeNode) {
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		aDir := a.IsRepo || a.isDir()
		bDir := b.IsRepo || b.isDir()
		if aDir != bDir {
			return aDir // directories before files
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		sortChildren(c)
	}
}
