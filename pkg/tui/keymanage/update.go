package keymanage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/tui/embed"
	grovecontext "github.com/grovetools/cx/pkg/context"

	"github.com/grovetools/nav/pkg/api"
)

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetSize(msg.Width, msg.Height)
		return m, nil

	case embed.FocusMsg:
		// Refresh on focus so mutations made while blurred appear.
		m.sessions, _ = m.store.GetSessions()
		m.rebuildSessionsOrder()
		return m, nil

	case embed.BlurMsg:
		return m, nil

	case embed.SetWorkspaceMsg:
		// Workspace changed — re-read from the Store.
		m.sessions, _ = m.store.GetSessions()
		m.rebuildSessionsOrder()
		// Re-point the CWD-aware "map to CWD" helper at the new workspace so
		// keymanage honours the host's notion of "current" instead of the
		// stale process launch directory.
		if msg.Node != nil {
			m.cwdPath = msg.Node.Path
			return m, enrichCwdProjectCmd(m.cwdPath)
		}
		return m, nil

	case RequestMapKeyMsg:
		m.pendingMapProject = msg.Project
		m.message = ""
		return m, nil

	case BulkMappingDoneMsg:
		m.ApplyBulkMapping(msg.MappedKeys)
		return m, clearHighlightCmd()

	case initialProjectsEnrichedMsg:
		m.enrichedProjects = msg.enrichedProjects
		m.isLoading = false

		var cmds []tea.Cmd
		cmds = append(cmds, fetchAllGitStatusesCmd(msg.projectList))
		cmds = append(cmds, fetchAllNoteCountsCmd(m.cwdPath))
		cmds = append(cmds, fetchAllPlanStatsCmd(m.cwdPath))
		cmds = append(cmds, fetchRulesStateCmd(msg.projectList))

		m.enrichmentLoading["git"] = true
		m.enrichmentLoading["notes"] = true
		m.enrichmentLoading["plans"] = true
		cmds = append(cmds, spinnerTickCmd())

		_ = api.SaveKeyManageCache(m.cfg.ConfigDir, m.enrichedProjects)
		return m, tea.Batch(cmds...)

	case rulesStateMsg:
		for path, status := range msg.rulesState {
			if proj, ok := m.enrichedProjects[path]; ok {
				switch status {
				case grovecontext.RuleHot:
					proj.ContextStatus = "H"
				case grovecontext.RuleCold:
					proj.ContextStatus = "C"
				case grovecontext.RuleExcluded:
					proj.ContextStatus = "X"
				default:
					proj.ContextStatus = ""
				}
			}
		}
		_ = api.SaveKeyManageCache(m.cfg.ConfigDir, m.enrichedProjects)
		return m, nil

	case cwdProjectEnrichedMsg:
		m.cwdProject = msg.project
		return m, nil

	case clearHighlightMsg:
		m.justMappedKeys = make(map[string]bool)
		return m, nil

	case gitStatusMapMsg:
		for path, status := range msg.statuses {
			if proj, ok := m.enrichedProjects[path]; ok {
				proj.GitStatus = status
			}
		}
		m.enrichmentLoading["git"] = false
		m.isLoading = false
		_ = api.SaveKeyManageCache(m.cfg.ConfigDir, m.enrichedProjects)
		return m, nil

	case noteCountsMapMsg:
		for _, proj := range m.enrichedProjects {
			if counts, ok := msg.counts[proj.Name]; ok {
				proj.NoteCounts = counts
			}
		}
		m.enrichmentLoading["notes"] = false
		_ = api.SaveKeyManageCache(m.cfg.ConfigDir, m.enrichedProjects)
		return m, nil

	case planStatsMapMsg:
		for _, proj := range m.enrichedProjects {
			if stats, ok := msg.stats[proj.Path]; ok {
				proj.PlanStats = stats
			}
		}
		m.enrichmentLoading["plans"] = false
		_ = api.SaveKeyManageCache(m.cfg.ConfigDir, m.enrichedProjects)
		return m, nil

	case spinnerTickMsg:
		anyLoading := m.isLoading
		for _, loading := range m.enrichmentLoading {
			if loading {
				anyLoading = true
				break
			}
		}
		if anyLoading {
			m.spinnerFrame++
			return m, spinnerTickCmd()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If help is visible, pass navigation keys through for scrolling.
	if m.help.ShowAll {
		switch {
		case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Help), msg.Type == tea.KeyEsc:
			m.help.Toggle()
			return m, nil
		default:
			var cmd tea.Cmd
			m.help, cmd = m.help.Update(msg)
			return m, cmd
		}
	}

	// Handle confirmation mode.
	if m.confirmMode != "" {
		switch {
		case msg.Type == tea.KeyEsc, msg.String() == "n", msg.String() == "N":
			m.confirmMode = ""
			m.confirmSource = ""
			m.message = "Cancelled"
			return m, nil

		case msg.String() == "y", msg.String() == "Y":
			switch m.confirmMode {
			case "load":
				m.store.TakeSnapshot()
				m.executeLoadIntoDefault(m.confirmSource)
			case "clear":
				m.store.TakeSnapshot()
				m.executeClearGroup()
			case "delete_group":
				m.store.TakeSnapshot()
				groupToDelete := m.store.GetActiveGroup()
				if err := m.store.DeleteGroup(groupToDelete); err != nil {
					m.message = fmt.Sprintf("Error deleting group: %v", err)
				} else {
					m.store.SetActiveGroup("default")
					_ = m.store.SetLastAccessedGroup("default")
					m.sessions, _ = m.store.GetSessions()
					m.rebuildSessionsOrder()
					m.message = fmt.Sprintf("Deleted group '%s'", groupToDelete)
				}
			}
			m.confirmMode = ""
			m.confirmSource = ""
			return m, nil
		}
		return m, nil
	}

	// Handle new group mode.
	if m.newGroupMode {
		switch msg.Type {
		case tea.KeyEsc:
			m.newGroupMode = false
			m.message = "New group cancelled"
			return m, nil
		case tea.KeyEnter:
			if m.newGroupStep == 0 {
				if m.newGroupName == "" {
					m.message = "Group name cannot be empty"
					return m, nil
				}
				m.newGroupStep = 1
				m.message = ""
			} else {
				m.store.TakeSnapshot()
				if err := m.store.CreateGroup(m.newGroupName, m.newGroupPrefix); err != nil {
					m.message = fmt.Sprintf("Error creating group: %v", err)
				} else {
					m.store.SetActiveGroup(m.newGroupName)
					_ = m.store.SetLastAccessedGroup(m.newGroupName)
					m.sessions, _ = m.store.GetSessions()
					m.rebuildSessionsOrder()
					m.message = fmt.Sprintf("Created and switched to group '%s'", m.newGroupName)
				}
				m.newGroupMode = false
			}
			return m, nil
		case tea.KeyBackspace:
			if m.newGroupStep == 0 {
				if len(m.newGroupName) > 0 {
					m.newGroupName = m.newGroupName[:len(m.newGroupName)-1]
				}
			} else {
				if len(m.newGroupPrefix) > 0 {
					m.newGroupPrefix = m.newGroupPrefix[:len(m.newGroupPrefix)-1]
				}
			}
			return m, nil
		case tea.KeySpace:
			if m.newGroupStep == 1 {
				m.newGroupPrefix += " "
			}
			return m, nil
		case tea.KeyRunes:
			if m.newGroupStep == 0 {
				for _, r := range msg.Runes {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
						m.newGroupName += string(r)
					}
				}
			} else {
				m.newGroupPrefix += string(msg.Runes)
			}
			return m, nil
		}
		return m, nil
	}

	// Handle load from group mode.
	if m.loadFromGroupMode {
		switch {
		case msg.Type == tea.KeyEsc:
			m.loadFromGroupMode = false
			m.message = "Load from group cancelled"
			return m, nil

		case key.Matches(msg, m.keys.Up):
			if m.loadFromGroupCursor > 0 {
				m.loadFromGroupCursor--
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.loadFromGroupCursor < len(m.loadFromGroupOptions)-1 {
				m.loadFromGroupCursor++
			}
			return m, nil

		case msg.Type == tea.KeyEnter:
			selected := m.loadFromGroupOptions[m.loadFromGroupCursor]
			m.loadFromGroupMode = false
			if m.store.ConfirmKeyUpdates() {
				m.confirmMode = "load"
				m.confirmSource = selected
				m.message = fmt.Sprintf("Load '%s' into default? This will replace non-locked mappings.", selected)
			} else {
				m.store.TakeSnapshot()
				m.executeLoadIntoDefault(selected)
			}
			return m, nil
		}
		return m, nil
	}

	// Handle save to group mode.
	if m.saveToGroupMode {
		switch {
		case msg.Type == tea.KeyEsc:
			m.saveToGroupMode = false
			m.saveToGroupNewMode = false
			m.saveToGroupInput = ""
			m.message = "Save to group cancelled"
			return m, nil

		case m.saveToGroupNewMode:
			switch msg.Type {
			case tea.KeyEnter:
				if m.saveToGroupInput != "" {
					m.store.TakeSnapshot()
					m.saveDefaultToGroup(m.saveToGroupInput)
				} else {
					m.message = "Group name cannot be empty"
				}
				m.saveToGroupMode = false
				m.saveToGroupNewMode = false
				m.saveToGroupInput = ""
				return m, nil
			case tea.KeyBackspace:
				if len(m.saveToGroupInput) > 0 {
					m.saveToGroupInput = m.saveToGroupInput[:len(m.saveToGroupInput)-1]
				}
				return m, nil
			case tea.KeyRunes:
				for _, r := range msg.Runes {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
						m.saveToGroupInput += string(r)
					}
				}
				return m, nil
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			if m.saveToGroupCursor > 0 {
				m.saveToGroupCursor--
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			if m.saveToGroupCursor < len(m.saveToGroupOptions)-1 {
				m.saveToGroupCursor++
			}
			return m, nil

		case msg.Type == tea.KeyEnter:
			selected := m.saveToGroupOptions[m.saveToGroupCursor]
			if selected == "+ New group..." {
				m.saveToGroupNewMode = true
				m.saveToGroupInput = ""
				m.message = "Enter new group name:"
			} else {
				m.store.TakeSnapshot()
				m.saveDefaultToGroup(selected)
				m.saveToGroupMode = false
			}
			return m, nil
		}
		return m, nil
	}

	// Handle move to group mode.
	if m.moveToGroupMode {
		switch {
		case msg.Type == tea.KeyEsc:
			m.moveToGroupMode = false
			m.message = "Move cancelled"
			return m, nil

		case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.PrevGroup):
			if m.moveToGroupCursor > 0 {
				m.moveToGroupCursor--
			} else {
				m.moveToGroupCursor = len(m.moveToGroupOptions) - 1
			}
			return m, nil

		case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.NextGroup):
			if m.moveToGroupCursor < len(m.moveToGroupOptions)-1 {
				m.moveToGroupCursor++
			} else {
				m.moveToGroupCursor = 0
			}
			return m, nil

		case msg.Type == tea.KeyEnter, msg.Type == tea.KeySpace:
			targetGroup := m.moveToGroupOptions[m.moveToGroupCursor]
			m.store.TakeSnapshot()
			m.executeMoveToGroup(targetGroup)
			m.moveToGroupMode = false
			return m, clearHighlightCmd()
		}
		return m, nil
	}

	// Handle set key mode.
	if m.setKeyMode {
		switch msg.Type {
		case tea.KeyEsc:
			m.setKeyMode = false
			m.message = "Set key cancelled."
		case tea.KeyRunes:
			input := msg.String()
			if num, err := strconv.Atoi(input); err == nil && num > 0 {
				targetIndex := num - 1
				if targetIndex < len(m.sessions) {
					m.mapKeyToCwd(targetIndex)
				} else {
					m.message = fmt.Sprintf("Invalid number: %d", num)
				}
			} else {
				targetKey := strings.ToLower(input)
				targetIndex := -1
				for i, s := range m.sessions {
					if s.Key == targetKey {
						targetIndex = i
						break
					}
				}
				if targetIndex != -1 {
					m.mapKeyToCwd(targetIndex)
				} else {
					m.message = fmt.Sprintf("Invalid key: %s", targetKey)
				}
			}
		}
		return m, nil
	}

	// Handle jumpMode (mini-leader key 'g').
	if m.jumpMode {
		m.jumpMode = false
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			r := msg.Runes[0]
			if r >= '1' && r <= '9' {
				targetIndex := int(r - '1')
				if targetIndex < len(m.sessions) {
					session := m.sessions[targetIndex]
					if session.Path != "" {
						return m, m.openSessionForPath(context.Background(), session.Path)
					}
					m.message = "No session mapped to this key"
				}
				return m, nil
			} else if r == 'g' {
				m.cursor = 0
				return m, nil
			}
		}
		return m, nil
	}

	// Enter jumpMode when 'g' is pressed.
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'g' {
		m.jumpMode = true
		return m, nil
	}

	// Handle move mode.
	if m.moveMode {
		switch {
		case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.MoveMode), msg.Type == tea.KeyEsc:
			m.moveMode = false
			m.message = "Exited move mode"
			return m, nil

		case key.Matches(msg, m.keys.Lock):
			if m.store.GetActiveGroup() != "default" {
				m.message = "Locked keys can only be modified in default group"
				return m, nil
			}
			if m.cursor < len(m.sessions) {
				m.store.TakeSnapshot()
				currentKey := m.sessions[m.cursor].Key
				currentSession := m.sessions[m.cursor]
				if m.lockedKeys[currentKey] {
					delete(m.lockedKeys, currentKey)
					delete(m.defaultLockedSessions, currentKey)
					m.message = fmt.Sprintf("Unlocked key '%s'", currentKey)
				} else {
					m.lockedKeys[currentKey] = true
					m.defaultLockedSessions[currentKey] = currentSession
					m.message = fmt.Sprintf("Locked key '%s'", currentKey)
				}
				m.rebuildSessionsOrder()
				m.saveChanges()
			}
			return m, nil

		case key.Matches(msg, m.keys.MoveUp):
			if m.cursor > 0 && m.cursor < len(m.sessions) {
				currentKey := m.sessions[m.cursor].Key
				if m.lockedKeys[currentKey] {
					m.message = "Cannot move locked key"
					return m, nil
				}
				m.store.TakeSnapshot()
				targetPos := m.cursor - 1
				for targetPos >= 0 && m.lockedKeys[m.sessions[targetPos].Key] {
					targetPos--
				}
				if targetPos >= 0 {
					m.swapSessionFields(m.cursor, targetPos)
					m.cursor = targetPos
					m.message = "Moved up"
					m.saveChanges()
				} else {
					m.message = "Cannot move past locked keys"
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.MoveDown):
			if m.cursor >= 0 && m.cursor < len(m.sessions)-1 {
				currentKey := m.sessions[m.cursor].Key
				if m.lockedKeys[currentKey] {
					m.message = "Cannot move locked key"
					return m, nil
				}
				m.store.TakeSnapshot()
				targetPos := m.cursor + 1
				for targetPos < len(m.sessions) && m.lockedKeys[m.sessions[targetPos].Key] {
					targetPos++
				}
				if targetPos < len(m.sessions) {
					m.swapSessionFields(m.cursor, targetPos)
					m.cursor = targetPos
					m.message = "Moved down"
					m.saveChanges()
				} else {
					m.message = "Cannot move past locked keys"
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.ConfirmMove):
			if err := m.store.UpdateSessionsAndLocks(m.sessions, m.getLockedKeysSlice()); err != nil {
				m.message = fmt.Sprintf("Error saving: %v", err)
			} else {
				if err := m.store.RegenerateBindings(); err != nil {
					m.message = fmt.Sprintf("Error regenerating bindings: %v", err)
				} else {
					if m.cfg.ReloadConfig != nil {
						_ = m.cfg.ReloadConfig()
					}
					m.message = "Order saved!"
				}
			}
			m.moveMode = false
			return m, nil
		}
		return m, nil
	}

	// Handle ESC to cancel pending mapping mode.
	if msg.Type == tea.KeyEsc {
		if m.pendingMapProject != nil {
			m.pendingMapProject = nil
			m.message = "Mapping cancelled"
			return m, func() tea.Msg { return CancelMappingMsg{} }
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Select):
		if m.cursor < len(m.sessions) {
			k := m.sessions[m.cursor].Key
			if m.selectedKeys[k] {
				delete(m.selectedKeys, k)
			} else {
				m.selectedKeys[k] = true
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.SelectAll):
		for _, s := range m.sessions {
			if s.Path != "" {
				m.selectedKeys[s.Key] = true
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.SelectNone):
		m.selectedKeys = make(map[string]bool)
		return m, nil

	case key.Matches(msg, m.keys.MoveMode):
		m.moveMode = true
		m.message = "Move mode: use j/k to reorder, l to lock/unlock, enter to save, q/m to cancel"
		return m, nil

	case key.Matches(msg, m.keys.Lock):
		if m.store.GetActiveGroup() != "default" {
			m.message = "Locked keys can only be modified in default group"
			return m, nil
		}
		keysToLock := []string{}
		if len(m.selectedKeys) > 0 {
			for k := range m.selectedKeys {
				keysToLock = append(keysToLock, k)
			}
		} else if m.cursor < len(m.sessions) {
			keysToLock = append(keysToLock, m.sessions[m.cursor].Key)
		}
		if len(keysToLock) > 0 {
			targetStateLocked := false
			for _, k := range keysToLock {
				if !m.lockedKeys[k] {
					targetStateLocked = true
					break
				}
			}
			count := 0
			for _, k := range keysToLock {
				var sess models.TmuxSession
				for _, s := range m.sessions {
					if s.Key == k {
						sess = s
						break
					}
				}
				if targetStateLocked {
					m.lockedKeys[k] = true
					m.defaultLockedSessions[k] = sess
				} else {
					delete(m.lockedKeys, k)
					delete(m.defaultLockedSessions, k)
				}
				count++
			}
			if targetStateLocked {
				m.message = fmt.Sprintf("Locked %d keys", count)
			} else {
				m.message = fmt.Sprintf("Unlocked %d keys", count)
			}
			m.selectedKeys = make(map[string]bool)
			m.rebuildSessionsOrder()
			m.saveChanges()
		}
		return m, nil

	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
		return m, nil

	case key.Matches(msg, m.keys.TogglePaths):
		m.pathDisplayMode = (m.pathDisplayMode + 1) % 3
		return m, nil

	case key.Matches(msg, m.keys.NextGroup):
		m.cycleGroup(1)
		return m, nil

	case key.Matches(msg, m.keys.PrevGroup):
		m.cycleGroup(-1)
		return m, nil

	case key.Matches(msg, m.keys.LoadDefault):
		if m.store.GetActiveGroup() == "default" {
			m.loadFromGroupOptions = []string{}
			for _, g := range m.store.GetGroups() {
				if g != "default" {
					m.loadFromGroupOptions = append(m.loadFromGroupOptions, g)
				}
			}
			if len(m.loadFromGroupOptions) == 0 {
				m.message = "No other groups to load from"
				return m, nil
			}
			m.loadFromGroupMode = true
			m.loadFromGroupCursor = 0
			m.message = "Select group to load from (↑/↓ to navigate, Enter to select, Esc to cancel)"
			return m, nil
		}
		sourceGroup := m.store.GetActiveGroup()
		if m.store.ConfirmKeyUpdates() {
			m.confirmMode = "load"
			m.confirmSource = sourceGroup
			m.message = fmt.Sprintf("Load '%s' into default? This will replace non-locked mappings.", sourceGroup)
		} else {
			m.store.TakeSnapshot()
			m.executeLoadIntoDefault(sourceGroup)
		}
		return m, nil

	case key.Matches(msg, m.keys.UnloadDefault):
		hasNonLocked := false
		for _, s := range m.sessions {
			if s.Path != "" && !m.lockedKeys[s.Key] {
				hasNonLocked = true
				break
			}
		}
		if !hasNonLocked {
			m.message = "No non-locked mappings to clear"
			return m, nil
		}
		if m.store.ConfirmKeyUpdates() {
			m.confirmMode = "clear"
			m.message = fmt.Sprintf("Clear all non-locked mappings from '%s'?", m.store.GetActiveGroup())
		} else {
			m.store.TakeSnapshot()
			m.executeClearGroup()
		}
		return m, nil

	case key.Matches(msg, m.keys.NewGroup):
		m.newGroupMode = true
		m.newGroupStep = 0
		m.newGroupName = ""
		m.newGroupPrefix = ""
		m.message = "Enter new group name:"
		return m, nil

	case key.Matches(msg, m.keys.Groups):
		m.nextCommand = "groups"
		return m, func() tea.Msg { return RequestManageGroupsMsg{} }

	case key.Matches(msg, m.keys.DeleteGroup):
		if m.store.GetActiveGroup() == "default" {
			m.message = "Cannot delete default group"
			return m, nil
		}
		m.confirmMode = "delete_group"
		m.message = fmt.Sprintf("Delete group '%s'? All mappings will be lost.", m.store.GetActiveGroup())
		return m, nil

	case key.Matches(msg, m.keys.SaveToGroup):
		hasMappings := false
		for _, s := range m.sessions {
			if s.Path != "" {
				hasMappings = true
				break
			}
		}
		if !hasMappings {
			m.message = "No mappings to save"
			return m, nil
		}
		m.saveToGroupOptions = []string{}
		currentGroup := m.store.GetActiveGroup()
		for _, g := range m.store.GetGroups() {
			if g != currentGroup && g != "default" {
				m.saveToGroupOptions = append(m.saveToGroupOptions, g)
			}
		}
		m.saveToGroupOptions = append(m.saveToGroupOptions, "+ New group...")
		m.saveToGroupMode = true
		m.saveToGroupCursor = 0
		m.saveToGroupNewMode = false
		m.saveToGroupInput = ""
		m.message = "Select group to save to (↑/↓ to navigate, Enter to select, Esc to cancel)"
		return m, nil

	case key.Matches(msg, m.keys.MoveToGroup):
		keysToMove := []string{}
		if len(m.selectedKeys) > 0 {
			for k := range m.selectedKeys {
				if !m.lockedKeys[k] {
					for _, s := range m.sessions {
						if s.Key == k && s.Path != "" {
							keysToMove = append(keysToMove, k)
							break
						}
					}
				}
			}
		} else if m.cursor < len(m.sessions) {
			currentSession := m.sessions[m.cursor]
			if currentSession.Path != "" && !m.lockedKeys[currentSession.Key] {
				keysToMove = append(keysToMove, currentSession.Key)
			}
		}
		if len(keysToMove) == 0 {
			m.message = "No eligible mappings to move (cannot move empty or locked keys)"
			return m, nil
		}
		m.moveToGroupOptions = []string{}
		currentGroup := m.store.GetActiveGroup()
		for _, g := range m.store.GetGroups() {
			if g != currentGroup {
				m.moveToGroupOptions = append(m.moveToGroupOptions, g)
			}
		}
		if len(m.moveToGroupOptions) == 0 {
			m.message = "No other groups to move to"
			return m, nil
		}
		m.moveToGroupMode = true
		m.moveToGroupCursor = 0
		m.moveToGroupKeys = keysToMove
		m.message = fmt.Sprintf("Move %d items to group (↑/↓, Enter, Esc)", len(keysToMove))
		return m, nil

	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.SetKey):
		if m.cwdProject == nil {
			m.message = "Current directory is not a valid workspace/project"
			return m, nil
		}
		m.setKeyMode = true
		m.message = "Enter key or number to map CWD to. (ESC to cancel)"
		return m, nil

	case key.Matches(msg, m.keys.Edit):
		return m.mapSelectedSlot()

	case key.Matches(msg, m.keys.Open):
		if m.pendingMapProject != nil {
			return m.mapSelectedSlot()
		}
		if m.cursor < len(m.sessions) {
			session := m.sessions[m.cursor]
			if session.Path != "" {
				return m, m.openSessionForPath(context.Background(), session.Path)
			}
			m.message = "No session mapped to this key"
		}

	case key.Matches(msg, m.keys.CopyPath):
		if m.cursor < len(m.sessions) {
			session := m.sessions[m.cursor]
			if session.Path != "" {
				if err := clipboard.WriteAll(session.Path); err != nil {
					m.message = fmt.Sprintf("Error copying path: %v", err)
				} else {
					m.message = fmt.Sprintf("Copied: %s", session.Path)
				}
			} else {
				m.message = "No path mapped to this key"
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Delete):
		clearedCount := 0
		if len(m.selectedKeys) > 0 {
			m.store.TakeSnapshot()
			for i := range m.sessions {
				if m.selectedKeys[m.sessions[i].Key] && !m.lockedKeys[m.sessions[i].Key] && m.sessions[i].Path != "" {
					m.sessions[i].Path = ""
					m.sessions[i].Repository = ""
					m.sessions[i].Description = ""
					clearedCount++
				}
			}
			m.selectedKeys = make(map[string]bool)
			m.message = fmt.Sprintf("Unmapped %d keys", clearedCount)
			m.saveChanges()
		} else if m.cursor < len(m.sessions) {
			session := &m.sessions[m.cursor]
			if session.Path != "" && !m.lockedKeys[session.Key] {
				m.store.TakeSnapshot()
				session.Path = ""
				session.Repository = ""
				session.Description = ""
				m.message = fmt.Sprintf("Unmapped key %s", session.Key)
				m.saveChanges()
			}
		}

	case key.Matches(msg, m.keys.GoToSessionize):
		if m.cursor < len(m.sessions) {
			session := m.sessions[m.cursor]
			if session.Path != "" {
				return m, func() tea.Msg {
					return JumpToSessionizeMsg{Path: session.Path, ApplyGroupFilter: true}
				}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.FocusCurrent):
		if m.cursor < len(m.sessions) {
			session := m.sessions[m.cursor]
			if session.Path != "" {
				return m, func() tea.Msg {
					return JumpToSessionizeMsg{Path: session.Path, ApplyGroupFilter: false}
				}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.justMappedKeys = make(map[string]bool)
		}

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
			m.justMappedKeys = make(map[string]bool)
		}

	case key.Matches(msg, m.keys.PageUp):
		m.cursor -= 5
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.justMappedKeys = make(map[string]bool)

	case key.Matches(msg, m.keys.PageDown):
		m.cursor += 5
		if m.cursor >= len(m.sessions) {
			m.cursor = len(m.sessions) - 1
		}
		m.justMappedKeys = make(map[string]bool)

	case key.Matches(msg, m.keys.Top):
		m.cursor = 0
		m.justMappedKeys = make(map[string]bool)

	case key.Matches(msg, m.keys.Bottom):
		m.cursor = len(m.sessions) - 1
		m.justMappedKeys = make(map[string]bool)

	case key.Matches(msg, m.keys.Undo):
		if err := m.store.Undo(); err != nil {
			m.message = fmt.Sprintf("Undo failed: %v", err)
		} else {
			m.refreshStateAfterUndoRedo()
			m.message = "Undo applied"
		}

	case key.Matches(msg, m.keys.Redo):
		if err := m.store.Redo(); err != nil {
			m.message = fmt.Sprintf("Redo failed: %v", err)
		} else {
			m.refreshStateAfterUndoRedo()
			m.message = "Redo applied"
		}
	}

	return m, nil
}

// swapSessionFields swaps only the path-related fields between two row
// indices, keeping the key column anchored.
func (m *Model) swapSessionFields(a, b int) {
	pa := m.sessions[a].Path
	ra := m.sessions[a].Repository
	da := m.sessions[a].Description

	m.sessions[a].Path = m.sessions[b].Path
	m.sessions[a].Repository = m.sessions[b].Repository
	m.sessions[a].Description = m.sessions[b].Description

	m.sessions[b].Path = pa
	m.sessions[b].Repository = ra
	m.sessions[b].Description = da
}
