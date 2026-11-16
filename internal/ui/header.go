package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const wideHeader = 80 // below this width, meters stack one per line

// headerLines renders the three header lines.
func (m Model) headerLines() []string {
	ps := m.projects()
	var nProjects, upProjects, nContainers, upContainers int
	for _, p := range ps {
		nContainers += len(p.Containers)
		upContainers += p.Running
		if p.Standalone() {
			continue
		}
		nProjects++
		if p.Running > 0 {
			upProjects++
		}
	}
	var cpu float64
	var mem int64
	for _, u := range m.usage() {
		cpu += u.CPU
		mem += u.Mem
	}
	cpuFrac := 0.0
	if m.info.NCPU > 0 {
		cpuFrac = cpu / float64(m.info.NCPU*100)
		cpu /= float64(m.info.NCPU)
	}
	memFrac := 0.0
	if m.info.MemTotal > 0 {
		memFrac = float64(mem) / float64(m.info.MemTotal)
	}

	lp, lc := m.tr.T("header.projects"), m.tr.T("header.containers")
	lcpu, lmem := m.tr.T("header.cpu"), m.tr.T("header.mem")
	lp, lc = alignLabels(lp, lc)
	lcpu, lmem = alignLabels(lcpu, lmem)
	ratio := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b)
	}
	projects := func(w int) string {
		return meter(lp, ratio(upProjects, nProjects), fmt.Sprintf("%d/%d", upProjects, nProjects), w)
	}
	containers := func(w int) string {
		return meter(lc, ratio(upContainers, nContainers), fmt.Sprintf("%d/%d", upContainers, nContainers), w)
	}
	cpuMeter := func(w int) string { return meter(lcpu, cpuFrac, formatCPU(cpu), w) }
	memMeter := func(w int) string {
		return meter(lmem, memFrac, formatBytes(mem)+"/"+formatBytes(m.info.MemTotal), w)
	}

	w := m.width
	if w < wideHeader {
		return []string{projects(w), containers(w), cpuMeter(w)}
	}
	left := (w - 1) / 2
	right := w - 1 - left
	counts := m.tr.T("header.volumes", "count", fmt.Sprint(len(m.volumes))) + "   " +
		m.tr.T("header.images", "count", fmt.Sprint(len(m.images)))
	version := "dctop " + m.version + " · Docker " + m.info.ServerVersion
	gap := w - ansi.StringWidth(counts) - ansi.StringWidth(version)
	third := counts + strings.Repeat(" ", max(1, gap)) + version
	return []string{
		projects(left) + " " + cpuMeter(right),
		containers(left) + " " + memMeter(right),
		third,
	}
}

// alignLabels pads two labels to the same width so that their meters line up.
func alignLabels(a, b string) (string, string) {
	w := max(ansi.StringWidth(a), ansi.StringWidth(b))
	return pad(a, w), pad(b, w)
}

// tabLine renders the tab titles, the current one highlighted.
func (m Model) tabLine() string {
	titles := []string{m.tr.T("tab.projects"), m.tr.T("tab.volumes"), m.tr.T("tab.images")}
	var b strings.Builder
	for i, title := range titles {
		cell := " " + title + " "
		if tab(i) == m.tab {
			cell = styleTabActive.Render(cell)
		}
		b.WriteString(cell)
		b.WriteString(" ")
	}
	return b.String()
}
