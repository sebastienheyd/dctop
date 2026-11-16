package project

import (
	"reflect"
	"strings"
	"testing"

	"github.com/sebastienheyd/dctop/internal/docker"
)

func ctr(project, name, service, state string) docker.Container {
	return docker.Container{
		ID:      name + strings.Repeat("0", 64-len(name)),
		Name:    name,
		Service: service,
		State:   state,
		Project: project,
	}
}

func sample() []docker.Container {
	b2 := ctr("beta", "beta-worker-1", "worker", "exited")
	a1 := ctr("alpha", "alpha-web-1", "web", "running")
	a1.WorkingDir = "/srv/alpha"
	a1.ConfigFiles = []string{"/srv/alpha/compose.yml"}
	return []docker.Container{
		b2,
		a1,
		ctr("", "lonely", "", "running"),
		ctr("beta", "beta-app-1", "app", "running"),
		ctr("alpha", "alpha-db-1", "db", "running"),
		ctr("gamma", "gamma-app-1", "app", "exited"),
	}
}

func names(ps []Project) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func containerNames(p Project) []string {
	out := []string{}
	for _, c := range p.Containers {
		out = append(out, c.Name)
	}
	return out
}

func TestGroup(t *testing.T) {
	ps := Group(sample())
	if got := names(ps); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma", ""}) {
		t.Fatalf("order: %q", got)
	}
	alpha, beta, gamma, lone := ps[0], ps[1], ps[2], ps[3]
	if alpha.Running != 2 || alpha.State() != Running {
		t.Errorf("alpha: %d %v", alpha.Running, alpha.State())
	}
	if alpha.WorkingDir != "/srv/alpha" || !reflect.DeepEqual(alpha.ConfigFiles, []string{"/srv/alpha/compose.yml"}) {
		t.Errorf("alpha dir: %q %q", alpha.WorkingDir, alpha.ConfigFiles)
	}
	if got := containerNames(beta); !reflect.DeepEqual(got, []string{"beta-app-1", "beta-worker-1"}) {
		t.Errorf("beta containers: %q", got)
	}
	if beta.Running != 1 || beta.State() != Partial {
		t.Errorf("beta: %d %v", beta.Running, beta.State())
	}
	if gamma.State() != Stopped {
		t.Errorf("gamma: %v", gamma.State())
	}
	if !lone.Standalone() || alpha.Standalone() {
		t.Errorf("standalone flags")
	}
	if len(Group(nil)) != 0 {
		t.Errorf("Group(nil) not empty")
	}
}

func TestUsage(t *testing.T) {
	cs := sample()
	var webID, dbID string
	for _, c := range cs {
		switch c.Name {
		case "alpha-web-1":
			webID = c.ID
		case "alpha-db-1":
			dbID = c.ID
		}
	}
	stats := []docker.Stat{
		{ID: webID[:12], CPU: 1.5, Mem: 100 << 20},
		{ID: dbID[:12], CPU: 0.5, Mem: 50 << 20},
		{ID: "ffffffffffff", CPU: 9, Mem: 9},
	}
	u := UsageByContainer(stats, cs)
	if len(u) != 2 {
		t.Errorf("usage map: %v", u)
	}
	alpha := Group(cs)[0]
	if got := alpha.Usage(u); got != (Usage{CPU: 2.0, Mem: 150 << 20}) {
		t.Errorf("alpha usage: %+v", got)
	}
}

func TestSort(t *testing.T) {
	cs := sample()
	ps := Group(cs)
	idOf := func(name string) string {
		for _, c := range cs {
			if c.Name == name {
				return c.ID
			}
		}
		return ""
	}
	u := map[string]Usage{
		idOf("alpha-web-1"): {CPU: 1, Mem: 10},
		idOf("beta-app-1"):  {CPU: 5, Mem: 5},
		idOf("lonely"):      {CPU: 50, Mem: 500},
	}
	cases := []struct {
		key  SortKey
		want []string
	}{
		{SortName, []string{"alpha", "beta", "gamma", ""}},
		{SortState, []string{"alpha", "beta", "gamma", ""}},
		{SortCPU, []string{"beta", "alpha", "gamma", ""}},
		{SortMem, []string{"alpha", "beta", "gamma", ""}},
	}
	for _, c := range cases {
		cp := append([]Project(nil), ps...)
		// Start from the reverse order so that the sort really has work to do.
		for i, j := 0, len(cp)-1; i < j; i, j = i+1, j-1 {
			cp[i], cp[j] = cp[j], cp[i]
		}
		Sort(cp, c.key, u)
		if got := names(cp); !reflect.DeepEqual(got, c.want) {
			t.Errorf("key %d: %q", c.key, got)
		}
	}
	// State order with a tie on state broken by name.
	extra := append(Group(cs), Project{Name: "aaa", Running: 0, Containers: []docker.Container{ctr("aaa", "aaa-1", "x", "exited")}})
	Sort(extra, SortState, u)
	if got := names(extra); !reflect.DeepEqual(got, []string{"alpha", "beta", "aaa", "gamma", ""}) {
		t.Errorf("state ties: %q", got)
	}
}

func TestFilter(t *testing.T) {
	ps := Group(sample())
	before := make([][]string, len(ps))
	for i, p := range ps {
		before[i] = containerNames(p)
	}

	got := Filter(ps, "BET", false)
	if names(got)[0] != "beta" || len(got) != 1 || len(got[0].Containers) != 2 {
		t.Errorf("BET: %q", names(got))
	}
	got = Filter(ps, "worker", false)
	if len(got) != 1 || got[0].Name != "beta" || !reflect.DeepEqual(containerNames(got[0]), []string{"beta-worker-1"}) {
		t.Errorf("worker: %+v", got)
	}
	if got := Filter(ps, "zzz", false); len(got) != 0 {
		t.Errorf("zzz: %q", names(got))
	}
	if got := Filter(ps, "", true); !reflect.DeepEqual(names(got), []string{"alpha", "beta", ""}) {
		t.Errorf("hide: %q", names(got))
	}
	if got := Filter(ps, "", false); !reflect.DeepEqual(names(got), names(ps)) {
		t.Errorf("unchanged: %q", names(got))
	}
	for i, p := range ps {
		if !reflect.DeepEqual(containerNames(p), before[i]) {
			t.Errorf("input modified: %q", containerNames(p))
		}
	}
}

func TestMatches(t *testing.T) {
	if !Matches("Alpha-Web", "web") || Matches("alpha", "x") || !Matches("anything", "") {
		t.Error("Matches")
	}
}

func TestFilterKeepsProjectState(t *testing.T) {
	got := Filter(Group(sample()), "worker", false)
	if len(got) != 1 {
		t.Fatalf("worker: %q", names(got))
	}
	beta := got[0]
	if beta.Running != 1 || beta.Total() != 2 || beta.State() != Partial {
		t.Errorf("beta filtered on worker: %d/%d %v", beta.Running, beta.Total(), beta.State())
	}
	if all := Group(sample()); all[1].Total() != 2 {
		t.Errorf("unfiltered total: %d", all[1].Total())
	}
}
