package navapp

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/components/pager"
	core_theme "github.com/grovetools/core/tui/theme"

	"github.com/grovetools/nav/pkg/tui/groups"
	"github.com/grovetools/nav/pkg/tui/history"
	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
)

// navState holds the sub-model pointers and lazy-init bookkeeping shared
// across all page adapters. It is heap-allocated so the adapters survive
// bubbletea's value-copy Model lifecycle.
type navState struct {
	cfg         Config
	sessionize  *sessionizer.Model
	keymanage   *keymanage.Model
	history     *history.Model
	windows     *windows.Model
	groups      *groups.Model
	initialized map[Tab]bool
}

// ---------- sessionizePage (tab 0: Sessionize) ----------

type sessionizePage struct {
	s      *navState
	width  int
	height int
}

func (p *sessionizePage) Name() string  { return "Sessionize" }
func (p *sessionizePage) Title() string { return core_theme.IconFileTree + " Workspace Sessions" }
func (p *sessionizePage) Init() tea.Cmd { return p.Focus() }
func (p *sessionizePage) View() string {
	if p.s.sessionize == nil {
		return ""
	}
	return p.s.sessionize.View()
}

func (p *sessionizePage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	if p.s.sessionize == nil {
		return p, nil
	}
	updated, cmd := p.s.sessionize.Update(msg)
	if sm, ok := updated.(*sessionizer.Model); ok {
		p.s.sessionize = sm
	}
	return p, cmd
}

func (p *sessionizePage) Focus() tea.Cmd {
	if !p.s.initialized[TabSessionize] {
		p.s.initialized[TabSessionize] = true
		if p.s.cfg.NewSessionize != nil {
			p.s.sessionize = p.s.cfg.NewSessionize()
		}
		if p.s.sessionize == nil {
			return nil
		}
		p.s.sessionize.SetEmbedMode(true)
		var cmds []tea.Cmd
		cmds = append(cmds, p.s.sessionize.Init())
		if p.width > 0 && p.height > 0 {
			updated, c := p.s.sessionize.Update(tea.WindowSizeMsg{Width: p.width, Height: p.height})
			if sm, ok := updated.(*sessionizer.Model); ok {
				p.s.sessionize = sm
			}
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		return tea.Batch(cmds...)
	}
	if p.s.sessionize != nil && p.s.cfg.OnReenterSessionize != nil {
		return p.s.cfg.OnReenterSessionize()
	}
	return nil
}

func (p *sessionizePage) Blur()            {}
func (p *sessionizePage) SetSize(w, h int) { p.width = w; p.height = h }
func (p *sessionizePage) Enabled() bool {
	if p.s.initialized[TabSessionize] {
		return p.s.sessionize != nil
	}
	return p.s.cfg.NewSessionize != nil
}

func (p *sessionizePage) IsTextEntryActive() bool {
	return p.s.sessionize != nil && p.s.sessionize.IsTextInputFocused()
}

func (p *sessionizePage) Footer() string {
	if p.s.sessionize == nil {
		return ""
	}
	return p.s.sessionize.Footer()
}

func (p *sessionizePage) TabID() string { return "sessionize" }

var (
	_ pager.Page              = (*sessionizePage)(nil)
	_ pager.PageWithID        = (*sessionizePage)(nil)
	_ pager.PageWithTitle     = (*sessionizePage)(nil)
	_ pager.PageWithEnabled   = (*sessionizePage)(nil)
	_ pager.PageWithTextInput = (*sessionizePage)(nil)
	_ pager.PageWithFooter    = (*sessionizePage)(nil)
)

// ---------- keymanagePage (tab 1: Key Manage) ----------

type keymanagePage struct {
	s      *navState
	width  int
	height int
}

func (p *keymanagePage) Name() string  { return "Key Manage" }
func (p *keymanagePage) Title() string { return core_theme.IconKeyboard + " Session Hotkeys" }
func (p *keymanagePage) Init() tea.Cmd { return p.Focus() }
func (p *keymanagePage) View() string {
	if p.s.keymanage == nil {
		return ""
	}
	return p.s.keymanage.View()
}

func (p *keymanagePage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	if p.s.keymanage == nil {
		return p, nil
	}
	updated, cmd := p.s.keymanage.Update(msg)
	if km, ok := updated.(*keymanage.Model); ok {
		p.s.keymanage = km
	}
	return p, cmd
}

func (p *keymanagePage) Focus() tea.Cmd {
	if !p.s.initialized[TabKeymanage] {
		p.s.initialized[TabKeymanage] = true
		if p.s.cfg.NewKeymanage != nil {
			p.s.keymanage = p.s.cfg.NewKeymanage()
		}
		if p.s.keymanage == nil {
			return nil
		}
		p.s.keymanage.EmbedMode = true
		var cmds []tea.Cmd
		cmds = append(cmds, p.s.keymanage.Init())
		if p.width > 0 && p.height > 0 {
			updated, c := p.s.keymanage.Update(tea.WindowSizeMsg{Width: p.width, Height: p.height})
			if km, ok := updated.(*keymanage.Model); ok {
				p.s.keymanage = km
			}
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		return tea.Batch(cmds...)
	}
	if p.s.keymanage != nil && p.s.cfg.OnReenterKeymanage != nil {
		p.s.cfg.OnReenterKeymanage()
	}
	return nil
}

func (p *keymanagePage) Blur()            {}
func (p *keymanagePage) SetSize(w, h int) { p.width = w; p.height = h }
func (p *keymanagePage) Enabled() bool {
	if p.s.initialized[TabKeymanage] {
		return p.s.keymanage != nil
	}
	return p.s.cfg.NewKeymanage != nil
}

func (p *keymanagePage) IsTextEntryActive() bool {
	return p.s.keymanage != nil && p.s.keymanage.IsTextInputFocused()
}

func (p *keymanagePage) Footer() string {
	if p.s.keymanage == nil {
		return ""
	}
	return p.s.keymanage.Footer()
}

func (p *keymanagePage) TabID() string { return "keymanage" }

var (
	_ pager.Page              = (*keymanagePage)(nil)
	_ pager.PageWithID        = (*keymanagePage)(nil)
	_ pager.PageWithTitle     = (*keymanagePage)(nil)
	_ pager.PageWithEnabled   = (*keymanagePage)(nil)
	_ pager.PageWithTextInput = (*keymanagePage)(nil)
	_ pager.PageWithFooter    = (*keymanagePage)(nil)
)

// ---------- historyPage (tab 2: History) ----------

type historyPage struct {
	s      *navState
	width  int
	height int
}

func (p *historyPage) Name() string  { return "History" }
func (p *historyPage) Title() string { return core_theme.IconClock + " Session History" }
func (p *historyPage) Init() tea.Cmd { return p.Focus() }
func (p *historyPage) View() string {
	if p.s.history == nil {
		return ""
	}
	return p.s.history.View()
}

func (p *historyPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	if p.s.history == nil {
		return p, nil
	}
	updated, cmd := p.s.history.Update(msg)
	if hm, ok := updated.(*history.Model); ok {
		p.s.history = hm
	}
	return p, cmd
}

func (p *historyPage) Focus() tea.Cmd {
	if !p.s.initialized[TabHistory] {
		p.s.initialized[TabHistory] = true
		if p.s.cfg.NewHistory != nil {
			p.s.history = p.s.cfg.NewHistory()
		}
		if p.s.history == nil {
			return nil
		}
		p.s.history.EmbedMode = true
		var cmds []tea.Cmd
		cmds = append(cmds, p.s.history.Init())
		if p.width > 0 && p.height > 0 {
			updated, c := p.s.history.Update(tea.WindowSizeMsg{Width: p.width, Height: p.height})
			if hm, ok := updated.(*history.Model); ok {
				p.s.history = hm
			}
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (p *historyPage) Blur()            {}
func (p *historyPage) SetSize(w, h int) { p.width = w; p.height = h }
func (p *historyPage) Enabled() bool {
	if p.s.initialized[TabHistory] {
		return p.s.history != nil
	}
	return p.s.cfg.NewHistory != nil
}

func (p *historyPage) IsTextEntryActive() bool {
	return p.s.history != nil && p.s.history.FilterMode()
}

func (p *historyPage) Footer() string {
	if p.s.history == nil {
		return ""
	}
	return p.s.history.Footer()
}

func (p *historyPage) TabID() string { return "history" }

var (
	_ pager.Page              = (*historyPage)(nil)
	_ pager.PageWithID        = (*historyPage)(nil)
	_ pager.PageWithTitle     = (*historyPage)(nil)
	_ pager.PageWithEnabled   = (*historyPage)(nil)
	_ pager.PageWithTextInput = (*historyPage)(nil)
	_ pager.PageWithFooter    = (*historyPage)(nil)
)

// ---------- windowsPage (tab 3: Windows) ----------

type windowsPage struct {
	s      *navState
	width  int
	height int
}

func (p *windowsPage) Name() string  { return "Windows" }
func (p *windowsPage) Title() string { return core_theme.IconViewDashboard + " Window Selector" }
func (p *windowsPage) Init() tea.Cmd { return p.Focus() }
func (p *windowsPage) View() string {
	if p.s.windows == nil {
		return ""
	}
	return p.s.windows.View()
}

func (p *windowsPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	if p.s.windows == nil {
		return p, nil
	}
	updated, cmd := p.s.windows.Update(msg)
	if wm, ok := updated.(*windows.Model); ok {
		p.s.windows = wm
	}
	return p, cmd
}

func (p *windowsPage) Focus() tea.Cmd {
	if !p.s.initialized[TabWindows] {
		p.s.initialized[TabWindows] = true
		if p.s.cfg.NewWindows != nil {
			p.s.windows = p.s.cfg.NewWindows()
		}
		if p.s.windows == nil {
			return nil
		}
		p.s.windows.EmbedMode = true
		var cmds []tea.Cmd
		cmds = append(cmds, p.s.windows.Init())
		if p.width > 0 && p.height > 0 {
			updated, c := p.s.windows.Update(tea.WindowSizeMsg{Width: p.width, Height: p.height})
			if wm, ok := updated.(*windows.Model); ok {
				p.s.windows = wm
			}
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (p *windowsPage) Blur()            {}
func (p *windowsPage) SetSize(w, h int) { p.width = w; p.height = h }
func (p *windowsPage) Enabled() bool {
	if p.s.initialized[TabWindows] {
		return p.s.windows != nil
	}
	return p.s.cfg.NewWindows != nil
}

func (p *windowsPage) IsTextEntryActive() bool {
	if p.s.windows == nil {
		return false
	}
	mode := p.s.windows.Mode()
	return mode == "filter" || mode == "rename"
}

func (p *windowsPage) Footer() string {
	if p.s.windows == nil {
		return ""
	}
	return p.s.windows.Footer()
}

func (p *windowsPage) TabID() string { return "windows" }

var (
	_ pager.Page              = (*windowsPage)(nil)
	_ pager.PageWithID        = (*windowsPage)(nil)
	_ pager.PageWithTitle     = (*windowsPage)(nil)
	_ pager.PageWithEnabled   = (*windowsPage)(nil)
	_ pager.PageWithTextInput = (*windowsPage)(nil)
	_ pager.PageWithFooter    = (*windowsPage)(nil)
)

// ---------- groupsPage (tab 4: Groups) ----------

type groupsPage struct {
	s      *navState
	width  int
	height int
}

func (p *groupsPage) Name() string  { return "Groups" }
func (p *groupsPage) Title() string { return core_theme.IconFileTree + " Group Management" }
func (p *groupsPage) Init() tea.Cmd { return p.Focus() }
func (p *groupsPage) View() string {
	if p.s.groups == nil {
		return ""
	}
	return p.s.groups.View()
}

func (p *groupsPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	if p.s.groups == nil {
		return p, nil
	}
	updated, cmd := p.s.groups.Update(msg)
	if gm, ok := updated.(*groups.Model); ok {
		p.s.groups = gm
	}
	return p, cmd
}

func (p *groupsPage) Focus() tea.Cmd {
	if !p.s.initialized[TabGroups] {
		p.s.initialized[TabGroups] = true
		if p.s.cfg.NewGroups != nil {
			p.s.groups = p.s.cfg.NewGroups()
		}
		if p.s.groups == nil {
			return nil
		}
		p.s.groups.EmbedMode = true
		// groups.Model has no Init cmd — just forward size.
		if p.width > 0 && p.height > 0 {
			updated, _ := p.s.groups.Update(tea.WindowSizeMsg{Width: p.width, Height: p.height})
			if gm, ok := updated.(*groups.Model); ok {
				p.s.groups = gm
			}
		}
		return nil
	}
	if p.s.groups != nil && p.s.cfg.OnReenterGroups != nil {
		p.s.cfg.OnReenterGroups()
	}
	return nil
}

func (p *groupsPage) Blur()            {}
func (p *groupsPage) SetSize(w, h int) { p.width = w; p.height = h }
func (p *groupsPage) Enabled() bool {
	if p.s.initialized[TabGroups] {
		return p.s.groups != nil
	}
	return p.s.cfg.NewGroups != nil
}

func (p *groupsPage) IsTextEntryActive() bool {
	return p.s.groups != nil && p.s.groups.InputMode() != ""
}

func (p *groupsPage) Footer() string {
	if p.s.groups == nil {
		return ""
	}
	return p.s.groups.Footer()
}

func (p *groupsPage) TabID() string { return "groups" }

var (
	_ pager.Page              = (*groupsPage)(nil)
	_ pager.PageWithID        = (*groupsPage)(nil)
	_ pager.PageWithTitle     = (*groupsPage)(nil)
	_ pager.PageWithEnabled   = (*groupsPage)(nil)
	_ pager.PageWithTextInput = (*groupsPage)(nil)
	_ pager.PageWithFooter    = (*groupsPage)(nil)
)
