package navapp

// View renders the pager (tab bar + active page body + outer padding).
func (m *Model) View() string {
	return m.pager.View()
}
