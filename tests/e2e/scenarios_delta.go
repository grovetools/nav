package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// mockDaemon is a lightweight HTTP server that mimics the grove daemon's SSE
// endpoint (/api/stream) and health check (/health). It listens on a Unix socket
// so that the nav binary (spawned in the sandboxed harness environment) connects
// to it instead of the real daemon.
type mockDaemon struct {
	listener   net.Listener
	server     *http.Server
	socketPath string

	mu      sync.Mutex
	clients []chan string // SSE client channels
}

// startMockDaemon creates a mock daemon listening on the grove socket path
// derived from the harness runtime directory. projectPaths are returned by /api/workspaces
// so the TUI discovers the test projects.
func startMockDaemon(runtimeDir string, projectPaths []string) (*mockDaemon, error) {
	// The daemon client looks for the socket at $XDG_RUNTIME_DIR/grove/groved.sock
	groveDir := filepath.Join(runtimeDir, "grove")
	if err := os.MkdirAll(groveDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating grove runtime dir: %w", err)
	}
	socketPath := filepath.Join(groveDir, "groved.sock")

	// Remove any leftover socket
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket: %w", err)
	}

	// Build workspace JSON for /api/workspaces
	workspacesJSON := buildWorkspacesJSON(projectPaths)

	md := &mockDaemon{
		listener:   listener,
		socketPath: socketPath,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", md.handleHealth)
	mux.HandleFunc("/api/stream", md.handleStream())
	mux.HandleFunc("/api/workspaces", md.handleWorkspaces(workspacesJSON))
	mux.HandleFunc("/api/sessions", md.handleEmpty)
	mux.HandleFunc("/api/config", md.handleEmptyObject)
	mux.HandleFunc("/api/focus", md.handleOK)
	mux.HandleFunc("/api/refresh", md.handleOK)

	md.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = md.server.Serve(listener) }()

	return md, nil
}

func (md *mockDaemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (md *mockDaemon) handleEmpty(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[]`))
}

func (md *mockDaemon) handleWorkspaces(workspacesJSON []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(workspacesJSON)
	}
}

func (md *mockDaemon) handleEmptyObject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{}`))
}

func (md *mockDaemon) handleOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleStream returns an SSE handler that keeps the connection open
// for pushes via SendEvent.
func (md *mockDaemon) handleStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send connected comment (matches real daemon protocol)
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		// Register this client for future events
		ch := make(chan string, 10)
		md.mu.Lock()
		md.clients = append(md.clients, ch)
		md.mu.Unlock()

		// Stream events until client disconnects
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", event)
				flusher.Flush()
			}
		}
	}
}

// SendEvent pushes a JSON event to all connected SSE clients.
func (md *mockDaemon) SendEvent(jsonData string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	for _, ch := range md.clients {
		select {
		case ch <- jsonData:
		default:
			// Skip if client buffer is full
		}
	}
}

// Close shuts down the mock daemon.
func (md *mockDaemon) Close() {
	md.server.Close()
	md.listener.Close()
	os.Remove(md.socketPath)
}

// buildWorkspacesJSON creates a JSON array of enriched workspaces for /api/workspaces.
func buildWorkspacesJSON(projectPaths []string) []byte {
	type workspaceEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Kind string `json:"kind"`
	}

	var workspaces []workspaceEntry
	for _, p := range projectPaths {
		workspaces = append(workspaces, workspaceEntry{
			Name: filepath.Base(p),
			Path: p,
			Kind: "standalone_project",
		})
	}

	data, _ := json.Marshal(workspaces)
	return data
}

// buildDeltaEvent creates a workspaces_delta SSE event with git status and note counts.
func buildDeltaEvent(path, branch string, isDirty bool, issues, inbox int) string {
	type statusInfo struct {
		Branch         string `json:"branch"`
		IsDirty        bool   `json:"is_dirty"`
		ModifiedCount  int    `json:"modified_count"`
		UntrackedCount int    `json:"untracked_count"`
	}
	type extendedGitStatus struct {
		statusInfo
		LinesAdded   int `json:"lines_added"`
		LinesDeleted int `json:"lines_deleted"`
	}
	type noteCounts struct {
		Issues int `json:"issues"`
		Inbox  int `json:"inbox"`
	}
	type workspaceDelta struct {
		Path       string             `json:"path"`
		GitStatus  *extendedGitStatus `json:"git_status,omitempty"`
		NoteCounts *noteCounts        `json:"note_counts,omitempty"`
	}
	type stateUpdate struct {
		WorkspaceDeltas []workspaceDelta `json:"workspace_deltas"`
		UpdateType      string           `json:"update_type"`
		Source          string           `json:"source"`
	}

	delta := workspaceDelta{
		Path: path,
		GitStatus: &extendedGitStatus{
			statusInfo: statusInfo{
				Branch:  branch,
				IsDirty: isDirty,
			},
		},
	}
	if issues > 0 || inbox > 0 {
		delta.NoteCounts = &noteCounts{
			Issues: issues,
			Inbox:  inbox,
		}
	}

	update := stateUpdate{
		WorkspaceDeltas: []workspaceDelta{delta},
		UpdateType:      "workspaces_delta",
		Source:          "git",
	}
	data, _ := json.Marshal(update)
	return string(data)
}

// setupDeltaTestEnv creates a sandboxed test environment with git repos,
// grove.yml config, and a mock daemon serving SSE deltas.
func setupDeltaTestEnv(ctx *harness.Context) error {
	projectsDir := filepath.Join(ctx.RootDir, "projects")
	if err := fs.CreateDir(projectsDir); err != nil {
		return fmt.Errorf("failed to create projects directory: %w", err)
	}

	projectNames := []string{"alpha", "beta"}
	for _, name := range projectNames {
		repoDir := filepath.Join(projectsDir, name)
		if err := fs.CreateDir(repoDir); err != nil {
			return fmt.Errorf("failed to create repo directory %s: %w", name, err)
		}
		if err := git.Init(repoDir); err != nil {
			return fmt.Errorf("failed to init git repo %s: %w", name, err)
		}
		if err := git.SetupTestConfig(repoDir); err != nil {
			return fmt.Errorf("failed to setup git config for %s: %w", name, err)
		}
		testFile := filepath.Join(repoDir, "README.md")
		if err := fs.WriteString(testFile, fmt.Sprintf("# %s\n", name)); err != nil {
			return fmt.Errorf("failed to write README for %s: %w", name, err)
		}
		if err := git.Add(repoDir, "."); err != nil {
			return fmt.Errorf("failed to stage files for %s: %w", name, err)
		}
		if err := git.Commit(repoDir, "Initial commit"); err != nil {
			return fmt.Errorf("failed to commit for %s: %w", name, err)
		}
	}

	ctx.Set("projects_dir", projectsDir)
	ctx.Set("alpha_path", filepath.Join(projectsDir, "alpha"))
	ctx.Set("beta_path", filepath.Join(projectsDir, "beta"))

	// Write grove.yml
	groveYAML := fmt.Sprintf(`version: "1.0"
groves:
  test_projects:
    path: %s
    enabled: true
tmux:
  available_keys: [a, b, c, d, e, f]
`, projectsDir)

	xdgConfigDir := ctx.ConfigDir()
	groveConfigDir := filepath.Join(xdgConfigDir, "grove")
	if err := fs.CreateDir(groveConfigDir); err != nil {
		return fmt.Errorf("failed to create grove config directory: %w", err)
	}
	if err := fs.WriteString(filepath.Join(groveConfigDir, "grove.yml"), groveYAML); err != nil {
		return fmt.Errorf("failed to write grove.yml: %w", err)
	}

	// Write empty sessions.yml
	navStateDir := filepath.Join(ctx.StateDir(), "grove", "nav")
	if err := fs.CreateDir(navStateDir); err != nil {
		return fmt.Errorf("failed to create nav state directory: %w", err)
	}
	if err := fs.WriteString(filepath.Join(navStateDir, "sessions.yml"), "sessions:\n  # No sessions configured\n"); err != nil {
		return fmt.Errorf("failed to write sessions.yml: %w", err)
	}

	return nil
}

// NavDeltaUpdatesGitScenario tests that the TUI applies git status from
// a workspaces_delta SSE event. A mock daemon sends a delta with a branch
// name after the TUI is already displaying the initial workspace list, and
// we verify the branch name appears in the rendered output.
func NavDeltaUpdatesGitScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "nav-delta-updates-git",
		Description: "Tests that workspaces_delta SSE events update git status in the TUI",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test environment", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupDeltaTestEnv(ctx)
			}),
			harness.NewStep("Start mock daemon and launch TUI", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				alphaPath := ctx.GetString("alpha_path")
				betaPath := ctx.GetString("beta_path")

				// Start mock daemon that serves the test workspaces
				daemon, err := startMockDaemon(ctx.RuntimeDir(), []string{alphaPath, betaPath})
				if err != nil {
					return fmt.Errorf("failed to start mock daemon: %w", err)
				}
				ctx.Set("mock_daemon", daemon)

				navBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find nav binary: %w", err)
				}

				// Start TUI
				session, err := ctx.StartTUI(navBinary, []string{"sz"})
				if err != nil {
					return fmt.Errorf("failed to start nav sz: %w", err)
				}
				ctx.Set("tui_session", session)

				// Wait for initial render with diagnostic capture on failure
				if err := session.WaitForText("WORKSPACE", 15*time.Second); err != nil {
					content, _ := session.Capture()
					ctx.ShowCommandOutput("TUI screen on failure", content, "")
					return fmt.Errorf("TUI did not render WORKSPACE header: %w\nScreen:\n%s", err, content)
				}

				// Allow enrichment spinners to settle
				time.Sleep(2 * time.Second)

				// Capture initial state
				content, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture initial screen: %w", err)
				}
				ctx.ShowCommandOutput("Initial TUI Content", content, "")
				ctx.Set("initial_content", content)

				// Now send a delta update with git status for alpha
				deltaEvent := buildDeltaEvent(alphaPath, "feature/delta-test", true, 0, 0)
				daemon.SendEvent(deltaEvent)

				// Give the TUI time to process the SSE event and re-render
				time.Sleep(2 * time.Second)

				// Capture post-delta state
				postContent, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture post-delta screen: %w", err)
				}
				ctx.ShowCommandOutput("Post-Delta TUI Content", postContent, "")
				ctx.Set("post_delta_content", postContent)

				return nil
			}),
			harness.NewStep("Verify delta applied git branch to TUI", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				postContent := ctx.GetString("post_delta_content")

				return ctx.Verify(func(v *verify.Collector) {
					// The delta sent branch "feature/delta-test" for alpha
					v.Contains("alpha workspace visible", postContent, "alpha")
					v.Contains("delta branch name rendered", postContent, "feature/delta-test")
				})
			}),
			harness.NewStep("Cleanup mock daemon", func(ctx *harness.Context) error {
				if d, ok := ctx.Get("mock_daemon").(*mockDaemon); ok {
					d.Close()
				}
				return nil
			}),
		},
	}
}

// NavDeltaUpdatesNotesScenario tests that note count enrichment from a
// workspaces_delta SSE event is reflected in the TUI's NOTES column.
func NavDeltaUpdatesNotesScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "nav-delta-updates-notes",
		Description: "Tests that workspaces_delta SSE events update note counts in the TUI",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test environment", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupDeltaTestEnv(ctx)
			}),
			harness.NewStep("Start mock daemon, send note delta, verify", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				alphaPath := ctx.GetString("alpha_path")
				betaPath := ctx.GetString("beta_path")

				daemon, err := startMockDaemon(ctx.RuntimeDir(), []string{alphaPath, betaPath})
				if err != nil {
					return fmt.Errorf("failed to start mock daemon: %w", err)
				}
				defer daemon.Close()

				navBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find nav binary: %w", err)
				}

				session, err := ctx.StartTUI(navBinary, []string{"sz"})
				if err != nil {
					return fmt.Errorf("failed to start nav sz: %w", err)
				}

				if err := session.WaitForText("WORKSPACE", 15*time.Second); err != nil {
					return fmt.Errorf("TUI did not render: %w", err)
				}

				// Allow enrichment spinners to settle
				time.Sleep(2 * time.Second)

				// Send delta with note counts for beta (3 issues, 2 inbox items)
				deltaEvent := buildDeltaEvent(betaPath, "main", false, 3, 2)
				daemon.SendEvent(deltaEvent)

				// Give TUI time to process the SSE event
				time.Sleep(2 * time.Second)

				content, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture screen: %w", err)
				}
				ctx.ShowCommandOutput("Post-Note-Delta TUI Content", content, "")

				return ctx.Verify(func(v *verify.Collector) {
					v.Contains("beta workspace visible", content, "beta")
					// Note counts render as icon + count. Issues=3, Inbox=2
					// The exact icon depends on the theme, but the number should appear
					v.Contains("issue count rendered", content, "3")
					v.Contains("inbox count rendered", content, "2")
				})
			}),
		},
	}
}

// NavDeltaIgnoresUnknownPathScenario verifies that a delta referencing a
// workspace path not present in the TUI's projectMap is silently ignored
// (no crash, no render change).
func NavDeltaIgnoresUnknownPathScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "nav-delta-ignores-unknown-path",
		Description: "Tests that deltas for unknown workspace paths are silently ignored",
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Check tmux availability", skipIfNoTmux),
			harness.NewStep("Setup test environment", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}
				return setupDeltaTestEnv(ctx)
			}),
			harness.NewStep("Send delta for unknown path and verify TUI stays stable", func(ctx *harness.Context) error {
				if shouldSkipTmuxTest(ctx) {
					return nil
				}

				alphaPath := ctx.GetString("alpha_path")
				betaPath := ctx.GetString("beta_path")

				daemon, err := startMockDaemon(ctx.RuntimeDir(), []string{alphaPath, betaPath})
				if err != nil {
					return fmt.Errorf("failed to start mock daemon: %w", err)
				}
				defer daemon.Close()

				navBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find nav binary: %w", err)
				}

				session, err := ctx.StartTUI(navBinary, []string{"sz"})
				if err != nil {
					return fmt.Errorf("failed to start nav sz: %w", err)
				}

				if err := session.WaitForText("WORKSPACE", 15*time.Second); err != nil {
					return fmt.Errorf("TUI did not render: %w", err)
				}

				// Allow enrichment spinners to settle
				time.Sleep(2 * time.Second)

				// Send delta for a path that doesn't exist in the project list
				unknownDelta := buildDeltaEvent("/nonexistent/project", "phantom-branch", true, 99, 99)
				daemon.SendEvent(unknownDelta)

				// Give TUI time to process
				time.Sleep(1 * time.Second)

				// Capture post-delta
				afterContent, err := session.Capture()
				if err != nil {
					return fmt.Errorf("failed to capture post-delta screen: %w", err)
				}
				ctx.ShowCommandOutput("After unknown delta", afterContent, "")

				return ctx.Verify(func(v *verify.Collector) {
					// TUI should still be rendering normally
					v.Contains("WORKSPACE header present", afterContent, "WORKSPACE")
					v.Contains("alpha still visible", afterContent, "alpha")
					v.Contains("beta still visible", afterContent, "beta")
					// The phantom branch should NOT appear
					v.NotContains("phantom branch not rendered", afterContent, "phantom-branch")
				})
			}),
		},
	}
}
