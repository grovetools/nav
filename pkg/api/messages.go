package api

// ViewGitRequestMsg is emitted by a nav tab (Sessionize or Key Manage) when the
// user asks to view git-viewer for the row under the cursor. It carries that
// row's workspace path; the embedding host (treemux) resolves the path to a
// workspace node and opens a git-viewer scoped to it via an in-place panel swap,
// restoring the nav panel when the viewer closes. Standalone nav has no host to
// act on it, so it is a harmless no-op there.
type ViewGitRequestMsg struct {
	Path string
}
