// Package history hosts the extracted nav session-history TUI. It depends
// only on a small HistoryLoader callback defined in Config (plus core
// packages for rendering), so it can be embedded by any host that can
// supply a list of history items. Standalone nav supplies a loader that
// reads from *tmux.Manager's access history.
package history

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	core_theme "github.com/grovetools/core/tui/theme"
	"github.com/grovetools/nav/pkg/api"
)

// Item holds a project and its last access time. Hosts supply a slice of
// these via HistoryLoader when the model is constructed or refreshed.
type Item struct {
	Project *api.Project
	Access  *workspace.ProjectAccess
}

// HistoryLoader fetches the full history list. The standalone nav binary
// supplies a loader that reads from *tmux.Manager; terminal supplies its
// own implementation. May be nil, in which case refreshes are no-ops.
type HistoryLoader func() ([]Item, error)

// Config collects every dependency the history TUI needs from its host.
type Config struct {
	// LoadHistory is called on Init and on embed.FocusMsg /
	// embed.SetWorkspaceMsg to (re)load the history list. May be nil —
	// refreshes become no-ops in that case and the model will render
	// whatever InitialItems were supplied.
	LoadHistory HistoryLoader

	// InitialItems is an optional seed so embedded hosts can render
	// something on first paint before LoadHistory resolves. May be nil.
	InitialItems []Item

	// KeyMapView is the current path -> session-key map used to render
	// the Key column. Standalone nav derives this from *tmux.Manager's
	// session list. Optional.
	KeyMapView map[string]string

	// KeyMap lets the host override the default history keymap. Zero
	// value uses DefaultKeyMap().
	KeyMap KeyMap
}

// JumpToSessionizeMsg is emitted when the user asks to jump from the
// history view into the sessionize view focused on the currently selected
// project. Hosts catch this message and switch views.
type JumpToSessionizeMsg struct {
	Path             string
	ApplyGroupFilter bool
}

// pageStyle is the default lipgloss style for the view.
var (
	pageStyle = lipgloss.NewStyle()
	dimStyle  = core_theme.DefaultTheme.Muted
)

// Model is the interactive history TUI. New() constructs one; Close()
// releases any resources (currently a no-op but defined for symmetry
// with other embeddable nav TUIs).
type Model struct {
	cfg Config

	// EmbedMode suppresses the header in View() when the model is
	// hosted inside a pager that renders its own title row.
	EmbedMode bool

	items             []historyItem
	filteredItems     []historyItem
	cursor            int
	selected          *api.Project
	keys              KeyMap
	help              help.Model
	quitting          bool
	enrichedProjects  map[string]*api.Project
	enrichmentLoading map[string]bool
	isLoading         bool
	spinnerFrame      int
	keyMap            map[string]string // map[path]key
	filterMode        bool
	filterText        string
	statusMessage     string
	jumpMode          bool // mini-leader: 'g' pressed
}

// historyItem is the internal form of Item used by the model.
type historyItem struct {
	project *api.Project
	access  *workspace.ProjectAccess
}

// New constructs a Model from the given Config.
func New(cfg Config) *Model {
	if cfg.KeyMap.Quit.Keys() == nil {
		cfg.KeyMap = DefaultKeyMap()
	}

	helpModel := help.NewBuilder().
		WithKeys(cfg.KeyMap).
		WithTitle("Session History - Help").
		Build()

	internal := make([]historyItem, 0, len(cfg.InitialItems))
	enriched := make(map[string]*api.Project, len(cfg.InitialItems))
	for _, it := range cfg.InitialItems {
		internal = append(internal, historyItem{project: it.Project, access: it.Access})
		enriched[it.Project.Path] = it.Project
	}

	keyMap := cfg.KeyMapView
	if keyMap == nil {
		keyMap = map[string]string{}
	}

	return &Model{
		cfg:               cfg,
		items:             internal,
		filteredItems:     internal,
		keys:              cfg.KeyMap,
		help:              helpModel,
		enrichedProjects:  enriched,
		enrichmentLoading: make(map[string]bool),
		isLoading:         cfg.LoadHistory != nil,
		keyMap:            keyMap,
	}
}

// Close releases resources owned by the Model. Currently a no-op but
// defined so embedded hosts can call it on shutdown for symmetry with
// other nav TUIs.
func (m *Model) Close() error { return nil }

// Selected returns the project the user picked, or nil if no selection
// was made (quit without choosing).
func (m *Model) Selected() *api.Project { return m.selected }

// Quitting reports whether the model has quit.
func (m *Model) Quitting() bool { return m.quitting }

// FilterMode reports whether the model is currently in text-filter mode.
func (m *Model) FilterMode() bool { return m.filterMode }

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{spinnerTickCmd()}

	if m.cfg.LoadHistory != nil {
		cmds = append(cmds, loadHistoryCmd(m.cfg.LoadHistory))
	}

	var projectList []*api.Project
	for _, item := range m.items {
		projectList = append(projectList, item.project)
	}
	if len(projectList) > 0 {
		m.enrichmentLoading["git"] = true
		cmds = append(cmds, fetchAllGitStatusesCmd(projectList))
	}

	return tea.Batch(cmds...)
}

// applyFilter filters the items based on the filter text.
func (m *Model) applyFilter() {
	m.filteredItems = filterItems(m.items, m.filterText)
	if m.cursor >= len(m.filteredItems) {
		m.cursor = 0
	}
}

// replaceItems swaps in a freshly loaded history list and rebuilds derived
// state (filter, enrichment index).
func (m *Model) replaceItems(items []Item) {
	internal := make([]historyItem, 0, len(items))
	enriched := make(map[string]*api.Project, len(items))
	for _, it := range items {
		internal = append(internal, historyItem{project: it.Project, access: it.Access})
		enriched[it.Project.Path] = it.Project
	}
	m.items = internal
	m.enrichedProjects = enriched
	m.applyFilter()
}
