package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// View renders the screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.body() + "\n" + m.status.text)
	v.AltScreen = true
	return v
}

// body renders the visible rows.
func (m Model) body() string {
	rows := m.rows()
	off := m.offset[m.tab]
	end := min(len(rows), off+m.bodyHeight())
	var b strings.Builder
	for i := off; i < end; i++ {
		if i > off {
			b.WriteByte('\n')
		}
		b.WriteString(m.marker(rows[i]))
		b.WriteByte(' ')
		b.WriteString(m.rowName(rows[i]))
	}
	return b.String()
}

// marker is the first column: "*" for a tagged row, "▶"/"▼" for a
// collapsed/expanded project, a space otherwise.
func (m Model) marker(r row) string {
	switch {
	case m.tagged[m.tab][r.key]:
		return "*"
	case r.key.kind == docker.KindProject && m.expanded[r.key.id]:
		return "▼"
	case r.key.kind == docker.KindProject:
		return "▶"
	}
	return " "
}

func (m Model) rowName(r row) string {
	switch {
	case r.container != nil:
		return "  " + r.container.Name
	case r.project != nil && r.project.Standalone():
		return m.tr.T("group.standalone")
	case r.project != nil:
		return r.project.Name
	case r.volume != nil:
		return r.volume.Name
	case r.image != nil:
		return r.image.Repository + ":" + r.image.Tag
	}
	return ""
}
