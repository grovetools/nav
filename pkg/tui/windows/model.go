package windows

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	tmuxclient "github.com/grovetools/core/pkg/tmux"
	"github.com/grovetools/core/tui/components/help"
)

// pageStyle is the default lipgloss style. Hosts can override by wrapping
// the View() output if they need different padding.
var pageStyle = lipgloss.NewStyle()

// Config collects every dependency the windows TUI needs from its host.
type Config struct {
	// Driver is the tmux-operation surface used to list, preview,
	// rename, close, and reorder windows.
	Driver SessionDriver

	// SessionName is the tmux session whose windows will be browsed.
	SessionName string

	// ShowChildProcesses, when true, causes the model to run ps(1) and
	// annotate each window with its active child process.
	ShowChildProcesses bool

	// KeyMap lets the host override the default windows keymap. Zero
	// value uses DefaultKeyMap().
	KeyMap KeyMap
}

// Model is the interactive window browser. New() constructs one; Close()
// releases any resources (currently a no-op, defined for symmetry).
type Model struct {
	cfg Config

	// EmbedMode suppresses the header in View() when the model is
	// hosted inside a pager that renders its own title row.
	EmbedMode bool

	driver             SessionDriver
	sessionName        string
	windows            []tmuxclient.Window
	filteredWindows    []tmuxclient.Window
	cursor             int
	help               help.Model
	keys               KeyMap
	filterInput        textinput.Model
	renameInput        textinput.Model
	mode               string // "normal", "filter", "rename", "move"
	selectedWindow     *tmuxclient.Window
	quitting           bool
	width, height      int
	err                error
	preview            string
	processCache       map[int]string      // Cache PID -> process name mapping
	showChildProcesses bool                // Whether to detect child processes
	originalWindows    []tmuxclient.Window // Original order when entering move mode
	jumpMode           bool                // Mini-leader mode: 'g' pressed
}

// New constructs a Model from the given Config.
func New(cfg Config) *Model {
	if cfg.KeyMap.Quit.Keys() == nil {
		cfg.KeyMap = DefaultKeyMap()
	}

	filterInput := textinput.New()
	filterInput.Placeholder = "Filter by name..."
	filterInput.CharLimit = 64

	renameInput := textinput.New()
	renameInput.Placeholder = "New window name..."
	renameInput.CharLimit = 128

	return &Model{
		cfg:                cfg,
		driver:             cfg.Driver,
		sessionName:        cfg.SessionName,
		keys:               cfg.KeyMap,
		help:               help.New(cfg.KeyMap),
		filterInput:        filterInput,
		renameInput:        renameInput,
		mode:               "normal",
		showChildProcesses: cfg.ShowChildProcesses,
	}
}

// Close releases resources owned by the Model. Currently a no-op.
func (m *Model) Close() error { return nil }

// SelectedWindow returns the window the user picked, or nil if no
// selection was made (quit without choosing).
func (m *Model) SelectedWindow() *tmuxclient.Window { return m.selectedWindow }

// Quitting reports whether the model entered the quit state.
func (m *Model) Quitting() bool { return m.quitting }

// Mode returns the current input mode (used by host UIs to know whether
// the user is in a text-input mode and should not propagate global keys).
func (m *Model) Mode() string { return m.mode }

// SessionName returns the name of the tmux session this picker is
// browsing.
func (m *Model) SessionName() string { return m.sessionName }

// applyFilter narrows filteredWindows by the current filterInput value.
func (m *Model) applyFilter() {
	m.filteredWindows = filterWindows(m.windows, m.filterInput.Value())
	if m.cursor >= len(m.filteredWindows) {
		m.cursor = 0
	}
}
