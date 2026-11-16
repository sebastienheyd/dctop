package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/project"
)

func newFilterInput() textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	return in
}

// startFilter opens the filter prompt on the current tab's query.
func (m Model) startFilter() (Model, tea.Cmd) {
	m.filtering = true
	m.input.Prompt = m.tr.T("filter.prompt")
	m.input.SetValue(m.filter[m.tab])
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// filterKey handles a key while the filter prompt is open: Enter keeps the
// query, Esc clears it, anything else edits it. No other binding runs.
func (m Model) filterKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.input.Blur()
		return m, nil
	case "esc":
		m.filtering = false
		m.input.Blur()
		m.input.Reset()
		delete(m.filter, m.tab)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if v := m.input.Value(); v != "" {
		m.filter[m.tab] = v
	} else {
		delete(m.filter, m.tab)
	}
	return m, cmd
}

// filterBar replaces the key bar while the filter prompt is open.
func (m Model) filterBar() string {
	return m.input.View() + "   " + m.tr.T("filter.hint")
}

// isExpanded tells whether a project shows its containers: when the user
// expanded it, or when the filter matches it only through its containers.
func (m Model) isExpanded(p *project.Project) bool {
	if m.expanded[p.Name] {
		return true
	}
	q := m.filter[tabProjects]
	return q != "" && (p.Standalone() || !project.Matches(p.Name, q))
}

// sortCount is the number of sort keys of a tab.
func sortCount(t tab) int {
	switch t {
	case tabVolumes:
		return 3
	case tabImages:
		return 2
	}
	return 4
}

func (m Model) cycleSort() Model {
	m.sortKey[m.tab] = (m.sortKey[m.tab] + 1) % sortCount(m.tab)
	return m
}

func (m Model) toggleHide() Model {
	m.hideStopped = !m.hideStopped
	if m.hideStopped {
		m.setInfo(m.tr.T("msg.hidden_on"))
	} else {
		m.setInfo(m.tr.T("msg.hidden_off"))
	}
	return m
}
