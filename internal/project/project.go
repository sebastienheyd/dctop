// Package project groups containers into Docker Compose projects. Every
// function here is pure: no Docker access, no clock.
package project

import (
	"sort"
	"strings"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// State summarises the containers of a project.
type State int

const (
	Stopped State = iota // no container running
	Partial              // some containers running
	Running              // every container running
)

// Project is the set of containers sharing a com.docker.compose.project
// label. The project with an empty Name groups the standalone containers.
type Project struct {
	Name        string // "" for the standalone group
	WorkingDir  string
	ConfigFiles []string
	Containers  []docker.Container // sorted by Name; only the matching ones after Filter
	Running     int                // containers of the whole project whose State is "running"
	Hidden      int                // containers left out of Containers by Filter
}

// Total is the number of containers of the whole project, filtered or not.
func (p Project) Total() int { return len(p.Containers) + p.Hidden }

// State tells whether all, some or none of the project's containers run. A
// filtered project keeps the state of the whole project.
func (p Project) State() State {
	switch {
	case p.Running == 0:
		return Stopped
	case p.Running == p.Total():
		return Running
	default:
		return Partial
	}
}

// Standalone tells whether p is the group of containers without a project.
func (p Project) Standalone() bool { return p.Name == "" }

// Usage is a CPU and memory figure.
type Usage struct {
	CPU float64
	Mem int64
}

// SortKey selects the order of the project list.
type SortKey int

const (
	SortName SortKey = iota
	SortState
	SortCPU
	SortMem
)

func countRunning(cs []docker.Container) int {
	n := 0
	for _, c := range cs {
		if c.State == "running" {
			n++
		}
	}
	return n
}

// Group gathers containers by project, sorted by name, with the standalone
// group last.
func Group(cs []docker.Container) []Project {
	byName := map[string]*Project{}
	var order []string
	for _, c := range cs {
		p, ok := byName[c.Project]
		if !ok {
			p = &Project{Name: c.Project}
			byName[c.Project] = p
			order = append(order, c.Project)
		}
		if p.WorkingDir == "" && c.WorkingDir != "" {
			p.WorkingDir = c.WorkingDir
		}
		if len(p.ConfigFiles) == 0 && len(c.ConfigFiles) > 0 {
			p.ConfigFiles = c.ConfigFiles
		}
		p.Containers = append(p.Containers, c)
	}
	out := make([]Project, 0, len(order))
	for _, name := range order {
		p := byName[name]
		sort.SliceStable(p.Containers, func(i, j int) bool { return p.Containers[i].Name < p.Containers[j].Name })
		p.Running = countRunning(p.Containers)
		out = append(out, *p)
	}
	Sort(out, SortName, nil)
	return out
}

// UsageByContainer keys the result by full container ID; a Stat matches the
// container whose ID starts with Stat.ID.
func UsageByContainer(stats []docker.Stat, cs []docker.Container) map[string]Usage {
	out := map[string]Usage{}
	for _, s := range stats {
		if s.ID == "" {
			continue
		}
		for _, c := range cs {
			if strings.HasPrefix(c.ID, s.ID) {
				out[c.ID] = Usage{CPU: s.CPU, Mem: s.Mem}
				break
			}
		}
	}
	return out
}

// Usage sums the usage of the project's containers.
func (p Project) Usage(u map[string]Usage) Usage {
	var total Usage
	for _, c := range p.Containers {
		cu := u[c.ID]
		total.CPU += cu.CPU
		total.Mem += cu.Mem
	}
	return total
}

// Sort orders projects by key; ties are broken by name and the standalone
// group always stays last. CPU and memory sort in descending order.
func Sort(ps []Project, key SortKey, u map[string]Usage) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.Standalone() != b.Standalone() {
			return b.Standalone()
		}
		switch key {
		case SortState:
			if a.State() != b.State() {
				return a.State() > b.State()
			}
		case SortCPU:
			if ua, ub := a.Usage(u).CPU, b.Usage(u).CPU; ua != ub {
				return ua > ub
			}
		case SortMem:
			if ua, ub := a.Usage(u).Mem, b.Usage(u).Mem; ua != ub {
				return ua > ub
			}
		}
		return a.Name < b.Name
	})
}

// Matches tells whether text contains query, ignoring case. An empty query
// matches everything.
func Matches(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

// Filter keeps the projects matching query and, with hideStopped, drops the
// projects with no running container. A project matching by its own name
// keeps all its containers; otherwise only the containers whose name or
// service matches are kept, and the project keeps its state and counts. The
// input is never modified.
func Filter(ps []Project, query string, hideStopped bool) []Project {
	out := []Project{}
	for _, p := range ps {
		if hideStopped && p.Running == 0 {
			continue
		}
		if query == "" || (!p.Standalone() && Matches(p.Name, query)) {
			out = append(out, p)
			continue
		}
		var kept []docker.Container
		for _, c := range p.Containers {
			if Matches(c.Name, query) || Matches(c.Service, query) {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			continue
		}
		p.Hidden += len(p.Containers) - len(kept)
		p.Containers = kept
		out = append(out, p)
	}
	return out
}
