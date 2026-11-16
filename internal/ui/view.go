package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/project"
)

const (
	minWidth  = 60
	minHeight = 12
)

// View renders the screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render returns exactly m.height lines, none wider than m.width.
func (m Model) render() string {
	if m.width < minWidth || m.height < minHeight {
		msg := m.tr.T("err.too_small", "min", fmt.Sprintf("%d×%d", minWidth, minHeight))
		return m.fit([]string{msg})
	}
	if m.mode == modeHelp {
		return m.fit(m.helpLines())
	}
	lines := m.headerLines()
	lines = append(lines, m.tabLine(), m.columnHeader())
	lines = append(lines, m.overlayConfirm(m.bodyLines())...)
	bar := m.keyBar()
	if m.filtering {
		bar = m.filterBar()
	}
	lines = append(lines, m.statusText(), bar)
	return m.fit(lines)
}

// fit truncates every line to the width and pads or cuts the list to the
// height.
func (m Model) fit(lines []string) string {
	h := max(m.height, 1)
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = truncate(lines[i], m.width)
		}
	}
	return strings.Join(out, "\n")
}

// column is one column of a list. The column with flex set takes the space
// left over, never less than its width; the last column takes the rest.
type column struct {
	title string
	width int
	right bool
	flex  bool
}

// cell is the content of one column for one row, with its colour.
type cell struct {
	text  string
	style *lipgloss.Style
}

// columns returns the columns of the current tab, sized for the screen.
func (m Model) columns() []column {
	cols := []column{
		{title: m.tr.T("col.name"), width: 20, flex: true},
		{title: m.tr.T("col.state"), width: 14},
		{title: m.tr.T("col.cpu"), width: 7, right: true},
		{title: m.tr.T("col.mem"), width: 7, right: true},
		{title: m.tr.T("col.info")},
	}
	return layout(cols, m.width-2)
}

// layout sizes the columns for avail cells (separators included): the flex
// column grows up to 40 cells while leaving the last column at least 24.
func layout(cols []column, avail int) []column {
	used := len(cols) - 1 // separators
	for _, c := range cols[:len(cols)-1] {
		used += c.width
	}
	for i := range cols {
		if cols[i].flex {
			extra := clamp(avail-used-24, 0, 40-cols[i].width)
			cols[i].width += extra
			used += extra
		}
	}
	cols[len(cols)-1].width = max(0, avail-used)
	return cols
}

// columnHeader renders the column titles in reverse video.
func (m Model) columnHeader() string {
	cols := m.columns()
	sorted := m.sortColumn()
	parts := make([]string, len(cols))
	for i, c := range cols {
		title := c.title
		if i == sorted {
			title += "▼"
		}
		parts[i] = align(title, c)
	}
	return styleColHeader.Render(pad("  "+strings.Join(parts, " "), m.width))
}

func align(s string, c column) string {
	if c.right {
		return padLeft(s, c.width)
	}
	return pad(s, c.width)
}

// bodyLines renders exactly bodyHeight lines of rows.
func (m Model) bodyLines() []string {
	rows := m.rows()
	cols := m.columns()
	u := m.usage()
	cur := m.cursorIndex(m.tab, rows)
	off := m.offset[m.tab]
	out := make([]string, m.bodyHeight())
	for i := range out {
		j := off + i
		if j >= len(rows) {
			break
		}
		out[i] = m.renderRow(rows[j], cols, u, j == cur)
	}
	return out
}

func (m Model) renderRow(r row, cols []column, u map[string]project.Usage, isCursor bool) string {
	cells := m.cells(r, u)
	tagged := m.tagged[m.tab][r.key]
	parts := make([]string, len(cols))
	for i, c := range cols {
		var ce cell
		if i < len(cells) {
			ce = cells[i]
		}
		text := align(ce.text, c)
		if ce.style != nil && !isCursor && !tagged {
			text = ce.style.Render(text)
		}
		parts[i] = text
	}
	line := m.marker(r) + " " + strings.Join(parts, " ")
	switch {
	case isCursor:
		return styleCursor.Render(pad(line, m.width))
	case tagged:
		return styleTagged.Render(line)
	}
	return line
}

// cells returns the column contents of a row of the current tab.
func (m Model) cells(r row, u map[string]project.Usage) []cell {
	switch {
	case r.container != nil:
		return m.containerCells(r.container, u)
	case r.project != nil:
		return m.projectCells(r.project, u)
	}
	return []cell{{text: m.rowName(r)}}
}

func (m Model) projectCells(p *project.Project, u map[string]project.Usage) []cell {
	name := p.Name
	if p.Standalone() {
		name = m.tr.T("group.standalone")
	}
	var stateKey string
	var style *lipgloss.Style
	switch p.State() {
	case project.Running:
		stateKey, style = "state.running", &styleRunning
	case project.Partial:
		stateKey, style = "state.partial", &stylePartial
	default:
		stateKey, style = "state.stopped", &styleStopped
	}
	state := fmt.Sprintf("%s %d/%d", m.tr.T(stateKey), p.Running, len(p.Containers))
	if busy, ok := m.busyText(rowKey{kind: docker.KindProject, id: p.Name}); ok {
		state, style = busy, &stylePartial
	}
	cpu, mem := usageCells(p.Containers, u)
	return []cell{
		{text: name},
		{text: state, style: style},
		{text: cpu},
		{text: mem},
		{text: shortenHome(p.WorkingDir, m.home)},
	}
}

func (m Model) containerCells(c *docker.Container, u map[string]project.Usage) []cell {
	stateKey := "state." + c.State
	state := m.tr.T(stateKey)
	if c.State == "exited" {
		state += fmt.Sprintf(" (%d)", c.ExitCode)
	}
	style := &stylePartial
	switch {
	case c.Health == "unhealthy" || (c.State == "exited" && c.ExitCode != 0) || c.State == "dead":
		style = &styleFailed
	case c.State == "running":
		style = &styleRunning
	case c.State == "exited":
		style = &styleStopped
	}
	if busy, ok := m.busyText(rowKey{kind: docker.KindContainer, id: c.ID}); ok {
		state, style = busy, &stylePartial
	}
	info := c.Status
	if c.Ports != "" {
		info += " · " + c.Ports
	}
	cpu, mem := usageCells([]docker.Container{*c}, u)
	return []cell{
		{text: "  " + c.Name},
		{text: state, style: style},
		{text: cpu},
		{text: mem},
		{text: info},
	}
}

// usageCells sums the usage of containers; both cells are blank when none
// of them has a recent sample.
func usageCells(cs []docker.Container, u map[string]project.Usage) (string, string) {
	var total project.Usage
	seen := false
	for _, c := range cs {
		if cu, ok := u[c.ID]; ok {
			seen = true
			total.CPU += cu.CPU
			total.Mem += cu.Mem
		}
	}
	if !seen {
		return "", ""
	}
	return formatCPU(total.CPU), formatBytes(total.Mem)
}

// marker is the first column: "*" for a tagged row, "▶"/"▼" for a
// collapsed/expanded project, a space otherwise.
func (m Model) marker(r row) string {
	switch {
	case m.tagged[m.tab][r.key]:
		return "*"
	case r.key.kind == docker.KindProject && r.project != nil && m.isExpanded(r.project):
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

var busyKeys = map[docker.Op]string{
	docker.OpStart:   "busy.start",
	docker.OpStop:    "busy.stop",
	docker.OpRestart: "busy.restart",
	docker.OpRemove:  "busy.remove",
}

// busyText returns the progress text of a row with an action in progress.
func (m Model) busyText(k rowKey) (string, bool) {
	op, ok := m.busy[k]
	if !ok {
		return "", false
	}
	return m.tr.T(busyKeys[op]), true
}

// sortColumn is the index of the column the current tab is sorted by.
func (m Model) sortColumn() int {
	return m.sortKey[m.tab]
}
