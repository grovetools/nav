package groups

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/components/help"
)

// pageStyle is the default lipgloss style for the groups view.
var pageStyle = lipgloss.NewStyle()

// Config collects every dependency the groups TUI needs from its host.
type Config struct {
	// Store is the mutation surface the TUI drives (create, rename,
	// reorder, delete, etc.).
	Store Store

	// ReloadConfig is an optional callback invoked after undo/redo so
	// the host can reload its multiplexer config. May be nil.
	ReloadConfig func() error

	// KeyMap lets the host override the default groups keymap. Zero
	// value uses DefaultKeyMap().
	KeyMap KeyMap
}

// Model is the groups management TUI. New() constructs one; Close()
// releases resources (currently a no-op, defined for symmetry).
type Model struct {
	cfg Config

	// EmbedMode suppresses the header in View() when the model is
	// hosted inside a pager that renders its own title row.
	EmbedMode bool

	manager      Store
	reloadConfig func() error
	groups       []string
	cursor       int
	keys         KeyMap
	help         help.Model

	// Mode state
	moveMode    bool
	inputMode   string // "new_name", "new_prefix", "rename", "edit_prefix"
	input       textinput.Model
	pendingName string // Holds name between name/prefix prompts for creation

	// Confirmation
	confirmMode bool
	message     string

	// Navigation
	nextCommand string // For handoff back to km

	// Dimensions
	width, height int
}

// New constructs a Model from the given Config.
func New(cfg Config) *Model {
	if cfg.KeyMap.Quit.Keys() == nil {
		cfg.KeyMap = DefaultKeyMap()
	}

	helpModel := help.NewBuilder().
		WithKeys(cfg.KeyMap).
		WithTitle("Group Management - Help").
		Build()

	ti := textinput.New()
	ti.CharLimit = 50
	ti.Width = 40

	return &Model{
		cfg:          cfg,
		manager:      cfg.Store,
		reloadConfig: cfg.ReloadConfig,
		groups:       cfg.Store.GetAllGroups(),
		cursor:       0,
		keys:         cfg.KeyMap,
		help:         helpModel,
		input:        ti,
	}
}

// Close releases resources owned by the Model. Currently a no-op.
func (m *Model) Close() error { return nil }

// NextCommand returns the host-handoff hint set by the model when it
// wants the parent router to switch back to a different view (currently
// only "km" for the key-manage view, used after the user finishes group
// management).
func (m *Model) NextCommand() string { return m.nextCommand }

// ClearNextCommand resets the handoff hint after the host has acted on
// it.
func (m *Model) ClearNextCommand() { m.nextCommand = "" }

// InputMode returns the current text-input mode (used by host UIs to
// know whether the user is in a text-input mode).
func (m *Model) InputMode() string { return m.inputMode }

// Reset re-reads the group list from the Store, resets the cursor, and
// clears any pending status message. Used by the host router when
// entering the groups view from another TUI.
func (m *Model) Reset() {
	m.groups = m.manager.GetAllGroups()
	m.cursor = 0
	m.message = ""
}

func (m *Model) refreshStateAfterUndoRedo() {
	m.groups = m.manager.GetAllGroups()
	if m.cursor >= len(m.groups) {
		m.cursor = len(m.groups) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
	if m.reloadConfig != nil {
		_ = m.reloadConfig()
	}
}
