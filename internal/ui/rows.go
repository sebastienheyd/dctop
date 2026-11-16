package ui

import (
	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/project"
)

type tab int

const (
	tabProjects tab = iota
	tabVolumes
	tabImages
	tabCount
)

// rowKey identifies a row across refreshes.
type rowKey struct {
	kind docker.TargetKind
	id   string // project name ("" = standalone group), full container ID, volume name, image repository:tag or ID
}

// row is one line of a list.
type row struct {
	key       rowKey
	depth     int // 0 for projects, volumes and images; 1 for containers
	project   *project.Project
	container *docker.Container
	volume    *docker.Volume
	image     *docker.Image
}

// rows returns the visible rows of the current tab, in display order.
func (m Model) rows() []row { return m.rowsOf(m.tab) }

func (m Model) rowsOf(t tab) []row {
	switch t {
	case tabVolumes:
		return m.volumeRows()
	case tabImages:
		return m.imageRows()
	}
	return m.projectRows()
}

func (m Model) projects() []project.Project {
	return project.Group(m.containers)
}

// projectRows applies the filter, the hide-stopped toggle and the sort.
func (m Model) projectRows() []row {
	ps := project.Filter(m.projects(), m.filter[tabProjects], m.hideStopped)
	project.Sort(ps, project.SortKey(m.sortKey[tabProjects]), m.usage())
	var out []row
	for i := range ps {
		p := &ps[i]
		out = append(out, row{key: rowKey{kind: docker.KindProject, id: p.Name}, project: p})
		if !m.isExpanded(p) {
			continue
		}
		for j := range p.Containers {
			c := &p.Containers[j]
			out = append(out, row{key: rowKey{kind: docker.KindContainer, id: c.ID}, depth: 1, project: p, container: c})
		}
	}
	return out
}

// allKeys returns the keys of every row tab t can show, whatever the
// filter, the collapsed projects and the hide-stopped toggle.
func (m Model) allKeys(t tab) map[rowKey]bool {
	out := map[rowKey]bool{}
	switch t {
	case tabVolumes:
		for _, v := range m.volumes {
			out[rowKey{kind: docker.KindVolume, id: v.Name}] = true
		}
	case tabImages:
		for i := range m.images {
			out[rowKey{kind: docker.KindImage, id: imageRef(&m.images[i])}] = true
		}
	default:
		for _, p := range m.projects() {
			out[rowKey{kind: docker.KindProject, id: p.Name}] = true
			for _, c := range p.Containers {
				out[rowKey{kind: docker.KindContainer, id: c.ID}] = true
			}
		}
	}
	return out
}

// cursorIndex returns the index of the cursor row of tab t in rows: the row
// with the remembered key, otherwise the remembered index clamped. -1 when
// rows is empty.
func (m Model) cursorIndex(t tab, rows []row) int {
	if len(rows) == 0 {
		return -1
	}
	c := m.cursor[t]
	if c.valid {
		for i, r := range rows {
			if r.key == c.key {
				return i
			}
		}
	}
	return clamp(c.index, 0, len(rows)-1)
}

// current returns the row under the cursor.
func (m Model) current() (row, bool) {
	rows := m.rows()
	i := m.cursorIndex(m.tab, rows)
	if i < 0 {
		return row{}, false
	}
	return rows[i], true
}

// moveTo puts the cursor of the current tab on index i of rows.
func (m Model) moveTo(rows []row, i int) Model {
	if len(rows) == 0 {
		return m
	}
	i = clamp(i, 0, len(rows)-1)
	m.cursor[m.tab] = cursorPos{key: rows[i].key, index: i, valid: true}
	return m
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// scrolled adjusts the scroll offset of the current tab so that the cursor
// row is visible.
func (m Model) scrolled() Model {
	rows := m.rows()
	h := m.bodyHeight()
	i := m.cursorIndex(m.tab, rows)
	off := m.offset[m.tab]
	if i < off {
		off = i
	}
	if i >= off+h {
		off = i - h + 1
	}
	off = clamp(off, 0, max(0, len(rows)-h))
	m.offset[m.tab] = off
	return m
}

// bodyHeight is the number of rows that fit between the column header and
// the status line.
func (m Model) bodyHeight() int {
	return max(1, m.height-7)
}
