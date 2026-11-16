package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// handleKey dispatches a key press of the main screen.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.mode == modeHelp {
		m.mode = modeMain
		return m, nil
	}
	if m.confirm != nil {
		return m.confirmKey(msg)
	}
	if m.filtering {
		return m.filterKey(msg)
	}
	switch msg.String() {
	case "f10", "q":
		mm, cmd := m.quit()
		return mm.(Model), cmd
	case "f1", "?":
		m.mode = modeHelp
		return m, nil
	case "f4", "/":
		return m.startFilter()
	case "f6", "o":
		return m.cycleSort(), nil
	case "H":
		return m.toggleHide(), nil
	case "f2", "s":
		return m.startAction(docker.OpStart)
	case "f3", "x":
		return m.startAction(docker.OpStop)
	case "f5", "r":
		return m.startAction(docker.OpRestart)
	case "f9", "d":
		return m.askRemove(), nil
	}
	rows := m.rows()
	i := m.cursorIndex(m.tab, rows)
	switch msg.String() {
	case "up", "k":
		return m.moveTo(rows, i-1), nil
	case "down", "j":
		return m.moveTo(rows, i+1), nil
	case "pgup":
		return m.moveTo(rows, i-m.bodyHeight()), nil
	case "pgdown":
		return m.moveTo(rows, i+m.bodyHeight()), nil
	case "home":
		return m.moveTo(rows, 0), nil
	case "end":
		return m.moveTo(rows, len(rows)-1), nil
	case "right":
		return m.setExpanded(rows, i, true), nil
	case "left":
		return m.collapse(rows, i), nil
	case "enter":
		if i >= 0 && rows[i].key.kind == docker.KindProject {
			return m.setExpanded(rows, i, !m.isExpanded(rows[i].project)), nil
		}
		return m, nil
	case "space":
		return m.toggleTag(rows, i), nil
	case "U":
		delete(m.tagged, m.tab)
		return m, nil
	}
	return m, nil
}

func (m Model) setExpanded(rows []row, i int, open bool) Model {
	if m.tab != tabProjects || i < 0 || rows[i].key.kind != docker.KindProject {
		return m
	}
	m.expanded[rows[i].key.id] = open
	return m
}

// collapse folds the project under the cursor, or moves from a container
// to its project.
func (m Model) collapse(rows []row, i int) Model {
	if m.tab != tabProjects || i < 0 {
		return m
	}
	r := rows[i]
	if r.key.kind == docker.KindContainer {
		for j := i - 1; j >= 0; j-- {
			if rows[j].key.kind == docker.KindProject {
				return m.moveTo(rows, j)
			}
		}
		return m
	}
	m.expanded[r.key.id] = false
	return m
}

func (m Model) toggleTag(rows []row, i int) Model {
	if i < 0 {
		return m
	}
	k := rows[i].key
	if m.tagged[m.tab] == nil {
		m.tagged[m.tab] = map[rowKey]bool{}
	}
	if m.tagged[m.tab][k] {
		delete(m.tagged[m.tab], k)
	} else {
		m.tagged[m.tab][k] = true
	}
	return m.moveTo(rows, i+1)
}
