package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// projectNames lists the project rows shown, in order ("" for the standalone group).
func projectNames(m Model) []string {
	out := []string{}
	for _, r := range m.rows() {
		if r.key.kind == docker.KindProject {
			out = append(out, r.key.id)
		}
	}
	return out
}

// lastLine is the key bar line of the frame.
func lastLine(m Model) string {
	lines := strings.Split(plain(m), "\n")
	return lines[len(lines)-1]
}

func TestFilterNarrowsProjects(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "/", "b", "e", "t")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Errorf("projects: %q", got)
	}
	assertContains(t, lastLine(m), "Filter: bet", "Enter: keep · Esc: clear")

	m = keys(m, "enter")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"beta"}) {
		t.Errorf("after enter: %q", got)
	}
	assertContains(t, lastLine(m), "F1Help")
	_, cmd := step(m, key("x"))
	runCmd(t, cmd)
	if want := []docker.Action{{Op: docker.OpStop, Target: projectTarget("beta")}}; !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}

	m = keys(m, "/", "esc")
	if got := projectNames(m); len(got) != 4 {
		t.Errorf("after esc: %q", got)
	}
}

func TestFilterInputSwallowsBindings(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "/")
	for _, k := range []string{"q", "x", "d", "H", "space"} {
		var cmd tea.Cmd
		m, cmd = step(m, key(k))
		if hasMsg[tea.QuitMsg](runCmd(t, cmd)) {
			t.Errorf("%s quits", k)
		}
	}
	if len(fb.Ran()) != 0 || len(m.tagged[tabProjects]) != 0 || m.confirm != nil || m.hideStopped {
		t.Errorf("a binding ran: ran=%v tags=%v confirm=%v hide=%v", fb.Ran(), m.tagged, m.confirm, m.hideStopped)
	}
	if got := m.filter[tabProjects]; got != "qxdH " {
		t.Errorf("query: %q", got)
	}
}

func TestFilterByContainerExpandsProject(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "/", "w", "o", "r", "k", "e", "r", "enter")
	out := plain(m)
	if !strings.Contains(out, "beta-worker-1") || strings.Contains(out, "beta-app-1") {
		t.Errorf("view:\n%s", out)
	}
	assertContains(t, out, "Filter: worker")
}

func TestSortCycles(t *testing.T) {
	m := newTestModel(t, fixture())
	m = send(m,
		statMsg{ID: cid("a1")[:12], CPU: 1, Mem: 10 << 20, Seen: t0},
		statMsg{ID: cid("b1")[:12], CPU: 5, Mem: 5 << 20, Seen: t0},
		tickMsg(t0),
	)
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma", ""}) {
		t.Errorf("default: %q", got)
	}
	assertContains(t, lineWith(m, "NAME"), "NAME▼")
	m = keys(m, "o")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma", ""}) {
		t.Errorf("state: %q", got)
	}
	assertContains(t, lineWith(m, "NAME"), "STATE▼")
	m = keys(m, "o")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"beta", "alpha", "gamma", ""}) {
		t.Errorf("cpu: %q", got)
	}
	m = keys(m, "o")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma", ""}) {
		t.Errorf("mem: %q", got)
	}
	assertContains(t, lineWith(m, "NAME"), "MEM▼")
	m = keys(m, "o")
	assertContains(t, lineWith(m, "NAME"), "NAME▼")
	m = keys(m, "f6", "f6")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"beta", "alpha", "gamma", ""}) {
		t.Errorf("f6: %q", got)
	}
}

func TestHideStopped(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "H")
	if got := projectNames(m); !reflect.DeepEqual(got, []string{"alpha", "beta", ""}) {
		t.Errorf("hidden: %q", got)
	}
	assertContains(t, plain(m), "Stopped projects hidden")
	m = keys(m, "H")
	if got := projectNames(m); len(got) != 4 {
		t.Errorf("shown: %q", got)
	}
	assertContains(t, plain(m), "Stopped projects shown")
}

func TestHelp(t *testing.T) {
	for _, open := range []string{"?", "f1"} {
		m := keys(newTestModel(t, fixture()), open)
		out := plain(m)
		assertContains(t, out, "dctop — keys", "Press any key to close")
		for _, k := range []string{"key.help", "key.start", "key.stop", "key.filter", "key.restart", "key.sort",
			"key.logs", "key.shell", "key.remove", "key.quit", "key.back", "key.follow"} {
			assertContains(t, out, m.tr.T(k))
		}
		m, cmd := step(m, key("q"))
		if hasMsg[tea.QuitMsg](runCmd(t, cmd)) {
			t.Error("q quits from help")
		}
		if strings.Contains(plain(m), "dctop — keys") {
			t.Error("help still open")
		}
	}
}

func TestQuit(t *testing.T) {
	for _, k := range []string{"q", "f10", "ctrl+c"} {
		m := newTestModel(t, fixture())
		_, cmd := step(m, key(k))
		if !hasMsg[tea.QuitMsg](runCmd(t, cmd)) {
			t.Errorf("%s does not quit", k)
		}
		if m.ctx.Err() == nil {
			t.Errorf("%s: context not cancelled", k)
		}
	}
}

func TestFilteredProjectKeepsItsState(t *testing.T) {
	m := newTestModel(t, fixture())
	m = keys(m, "/", "w", "o", "r", "k", "e", "r", "enter")
	if l := lineWith(m, "beta "); !strings.Contains(l, "partial 1/2") {
		t.Errorf("beta under filter: %q", l)
	}
}
