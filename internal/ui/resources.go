package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/project"
)

const (
	diskUsageMaxAge  = 30 * time.Second
	diskUsageTimeout = 2 * time.Minute
)

// diskUsageMsg carries the result of `docker system df -v`.
type diskUsageMsg struct {
	du  docker.DiskUsage
	err error
}

// switchTab shows tab t, loading disk usage for the resource tabs when it
// was never loaded or is older than 30 s.
func (m Model) switchTab(t tab) (Model, tea.Cmd) {
	m.tab = t
	if t == tabProjects {
		return m, nil
	}
	if m.duLoaded && m.now.Sub(m.duAt) <= diskUsageMaxAge {
		return m, nil
	}
	return m.loadDiskUsage()
}

// loadDiskUsage starts a background load unless one is already running.
func (m Model) loadDiskUsage() (Model, tea.Cmd) {
	if m.duLoading {
		return m, nil
	}
	m.duLoading = true
	b, parent := m.backend, m.ctx
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, diskUsageTimeout)
		defer cancel()
		du, err := b.DiskUsage(ctx)
		return diskUsageMsg{du: du, err: err}
	}
}

func (m Model) applyDiskUsage(msg diskUsageMsg) Model {
	m.duLoading = false
	if msg.err != nil {
		m.setError(m.tr.T("fail.refresh", "error", msg.err.Error()))
		return m
	}
	m.du, m.duLoaded, m.duAt = msg.du, true, m.now
	return m.reconcile()
}

// volumeUsed tells whether a container mounts the volume.
func (m Model) volumeUsed(v *docker.Volume) bool {
	if m.du.VolumeLinks[v.Name] > 0 {
		return true
	}
	for _, c := range m.containers {
		for _, mount := range c.Mounts {
			if mount == v.Name {
				return true
			}
		}
	}
	return false
}

// volumeRows applies the filter and the sort; anonymous volumes stay last.
func (m Model) volumeRows() []row {
	q := m.filter[tabVolumes]
	vs := make([]*docker.Volume, 0, len(m.volumes))
	for i := range m.volumes {
		v := &m.volumes[i]
		if project.Matches(v.Name, q) || (v.Project != "" && project.Matches(v.Project, q)) {
			vs = append(vs, v)
		}
	}
	key := m.sortKey[tabVolumes]
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := vs[i], vs[j]
		if a.Anonymous != b.Anonymous {
			return b.Anonymous
		}
		switch key {
		case 1:
			if a.Project != b.Project {
				return a.Project < b.Project
			}
		case 2:
			if sa, sb := m.du.VolumeSize[a.Name], m.du.VolumeSize[b.Name]; sa != sb {
				return sa > sb
			}
		}
		return a.Name < b.Name
	})
	out := make([]row, 0, len(vs))
	for _, v := range vs {
		out = append(out, row{key: rowKey{kind: docker.KindVolume, id: v.Name}, volume: v})
	}
	return out
}

// imageRows applies the filter and the sort.
func (m Model) imageRows() []row {
	q := m.filter[tabImages]
	is := make([]*docker.Image, 0, len(m.images))
	for i := range m.images {
		if img := &m.images[i]; project.Matches(imageName(img), q) {
			is = append(is, img)
		}
	}
	bySize := m.sortKey[tabImages] == 1
	sort.SliceStable(is, func(i, j int) bool {
		a, b := is[i], is[j]
		if bySize && a.Size != b.Size {
			return a.Size > b.Size
		}
		return imageName(a) < imageName(b)
	})
	out := make([]row, 0, len(is))
	for _, img := range is {
		out = append(out, row{key: rowKey{kind: docker.KindImage, id: img.ID}, image: img})
	}
	return out
}

func (m Model) volumeColumns() []column {
	return layout([]column{
		{title: m.tr.T("col.name"), rest: true},
		{title: m.tr.T("col.project"), width: 20},
		{title: m.tr.T("col.driver"), width: 8},
		{title: m.tr.T("col.used"), width: 7},
		{title: m.tr.T("col.size"), width: 8, right: true},
	}, m.width-2)
}

func (m Model) imageColumns() []column {
	return layout([]column{
		{title: m.tr.T("col.name"), rest: true},
		{title: m.tr.T("col.id"), width: 12},
		{title: m.tr.T("col.size"), width: 8, right: true},
		{title: m.tr.T("col.age"), width: 14},
		{title: m.tr.T("col.used"), width: 7, right: true},
	}, m.width-2)
}

const pending = "…" // shown until disk usage has been loaded

func (m Model) volumeCells(v *docker.Volume) []cell {
	name := v.Name
	if v.Anonymous && len(name) > 12 {
		name = name[:12] + "…"
	}
	used := m.tr.T("used.no")
	if m.volumeUsed(v) {
		used = m.tr.T("used.yes")
	}
	size := pending
	if m.duLoaded {
		size = ""
		if n, ok := m.du.VolumeSize[v.Name]; ok {
			size = formatBytes(n)
		}
	}
	return []cell{{text: name}, {text: v.Project}, {text: v.Driver}, {text: used}, {text: size}}
}

func (m Model) imageCells(img *docker.Image) []cell {
	id := strings.TrimPrefix(img.ID, "sha256:")
	if len(id) > 12 {
		id = id[:12]
	}
	used := pending
	if m.duLoaded {
		used = fmt.Sprint(m.du.ImageContainers[img.ID])
	}
	return []cell{{text: imageName(img)}, {text: id}, {text: formatBytes(img.Size)}, {text: img.Created}, {text: used}}
}
