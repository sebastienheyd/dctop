package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/project"
)

func TestKeyHelperRoundTrip(t *testing.T) {
	for _, name := range []string{"down", "up", "f3", "f10", "space", "enter", "esc", "tab", "shift+tab", "pgdown", "home", "end", "ctrl+c", "x", "H", "/", "?"} {
		if got := key(name).(fmt.Stringer).String(); got != name {
			t.Errorf("key(%q).String() = %q", name, got)
		}
	}
}

// order returns the positions of each needle in s, failing when one is missing.
func assertOrder(t *testing.T, s string, needles ...string) {
	t.Helper()
	last := -1
	for _, n := range needles {
		i := strings.Index(s, n)
		if i < 0 {
			t.Fatalf("%q not found in:\n%s", n, s)
		}
		if i < last {
			t.Fatalf("%q out of order in:\n%s", n, s)
		}
		last = i
	}
}

func currentKey(t *testing.T, m Model) rowKey {
	t.Helper()
	r, ok := m.current()
	if !ok {
		t.Fatal("no current row")
	}
	return r.key
}

func projectKey(name string) rowKey { return rowKey{kind: docker.KindProject, id: name} }

func TestInitialRows(t *testing.T) {
	out := plain(newTestModel(t, fixture()))
	assertOrder(t, out, "alpha", "beta", "gamma", "Standalone containers")
	if strings.Contains(out, "alpha-web-1") {
		t.Error("projects should start collapsed")
	}
}

func TestExpandCollapse(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "right")
	out := plain(m)
	if !strings.Contains(out, "alpha-web-1") || !strings.Contains(out, "alpha-db-1") {
		t.Fatalf("not expanded:\n%s", out)
	}
	if !strings.HasPrefix(lineWith(m, "alpha"), "▼") {
		t.Errorf("alpha row: %q", lineWith(m, "alpha"))
	}
	m = keys(m, "left")
	if strings.Contains(plain(m), "alpha-web-1") || !strings.HasPrefix(lineWith(m, "alpha"), "▶") {
		t.Errorf("not collapsed:\n%s", plain(m))
	}
	m = keys(m, "enter")
	if !strings.Contains(plain(m), "alpha-web-1") {
		t.Error("enter does not expand")
	}
	m = keys(m, "enter")
	if strings.Contains(plain(m), "alpha-web-1") {
		t.Error("enter does not collapse")
	}
	m = keys(m, "right", "down", "down")
	if r, _ := m.current(); r.key.kind != docker.KindContainer {
		t.Fatalf("cursor not on a container: %+v", r.key)
	}
	m = keys(m, "left")
	if currentKey(t, m) != projectKey("alpha") {
		t.Errorf("left on a container: %+v", currentKey(t, m))
	}
}

func TestNavigationClamps(t *testing.T) {
	m := newTestModel(t, fixture())
	m = keys(m, "up")
	if currentKey(t, m) != projectKey("alpha") {
		t.Error("up on first row")
	}
	m = keys(m, "end")
	if currentKey(t, m) != projectKey("") {
		t.Errorf("end: %+v", currentKey(t, m))
	}
	m = keys(m, "down")
	if currentKey(t, m) != projectKey("") {
		t.Error("down on last row")
	}
	m = keys(m, "home")
	if currentKey(t, m) != projectKey("alpha") {
		t.Error("home")
	}
	m = keys(m, "j")
	if currentKey(t, m) != projectKey("beta") {
		t.Error("j")
	}
	m = keys(m, "k")
	if currentKey(t, m) != projectKey("alpha") {
		t.Error("k")
	}

	big := fixture()
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("p%02d", i)
		big.containers = append(big.containers, fixtureContainer(id, id+"-app-1", "e"+id, "running", "Up", "", 0))
	}
	m = newTestModel(t, big)
	m = keys(m, "pgdown")
	rows := m.rows()
	if want := rows[m.bodyHeight()].key; currentKey(t, m) != want {
		t.Errorf("pgdown: %+v want %+v", currentKey(t, m), want)
	}
	m = keys(m, "pgup")
	if currentKey(t, m) != rows[0].key {
		t.Errorf("pgup: %+v", currentKey(t, m))
	}
}

func TestTagging(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "space")
	if !m.tagged[tabProjects][projectKey("alpha")] || !strings.HasPrefix(lineWith(m, "alpha"), "*") {
		t.Errorf("alpha not tagged: %q", lineWith(m, "alpha"))
	}
	if currentKey(t, m) != projectKey("beta") {
		t.Error("cursor did not move down")
	}
	m = keys(m, "up", "space")
	if m.tagged[tabProjects][projectKey("alpha")] {
		t.Error("space did not untag")
	}
	m = keys(m, "home", "space", "space")
	if len(m.tagged[tabProjects]) != 2 {
		t.Fatalf("tags: %v", m.tagged[tabProjects])
	}
	m = keys(m, "U")
	if len(m.tagged[tabProjects]) != 0 {
		t.Errorf("U: %v", m.tagged[tabProjects])
	}
}

func without(fb *fakeBackend, project string) dataMsg {
	var cs []docker.Container
	for _, c := range fb.containers {
		if c.Project != project {
			cs = append(cs, c)
		}
	}
	return dataMsg{containers: cs, volumes: fb.volumes, images: fb.images}
}

func TestCursorFollowsRowAcrossRefresh(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "down")
	m = send(m, without(fb, "alpha"))
	if currentKey(t, m) != projectKey("beta") {
		t.Errorf("cursor: %+v", currentKey(t, m))
	}

	m = keys(newTestModel(t, fb), "down", "down")
	m = send(m, without(fb, "gamma"))
	if _, ok := m.current(); !ok {
		t.Error("cursor lost")
	}

	m = send(m, dataMsg{})
	if len(m.rows()) != 0 {
		t.Errorf("rows: %d", len(m.rows()))
	}
	if _, ok := m.current(); ok {
		t.Error("current on empty list")
	}
	m = keys(m, "down", "up", "end", "space", "right", "left")
	_ = plain(m)
}

func TestTagsDroppedWhenRowVanishes(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "space")
	m = send(m, without(fb, "alpha"))
	if len(m.tagged[tabProjects]) != 0 {
		t.Errorf("tags: %v", m.tagged[tabProjects])
	}
}

var t0 = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

func alphaStats() []tea.Msg {
	return []tea.Msg{
		statMsg{ID: cid("a1")[:12], CPU: 1.5, Mem: 100 << 20, Seen: t0},
		statMsg{ID: cid("a2")[:12], CPU: 0.5, Mem: 50 << 20, Seen: t0},
		tickMsg(t0),
	}
}

func TestStatsFeedTheProjects(t *testing.T) {
	m := send(newTestModel(t, fixture()), alphaStats()...)
	if got := m.projectUsage("alpha"); got != (project.Usage{CPU: 2.0, Mem: 150 << 20}) {
		t.Errorf("%+v", got)
	}
}

func TestStaleStatsAreDropped(t *testing.T) {
	m := send(newTestModel(t, fixture()), alphaStats()...)
	m = send(m, tickMsg(t0.Add(6*time.Second)))
	if got := m.projectUsage("alpha"); got != (project.Usage{}) {
		t.Errorf("%+v", got)
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	fb := &fakeBackend{runErr: map[string]error{}}
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("proj%02d", i)
		fb.containers = append(fb.containers, fixtureContainer(id, id+"-app-1", fmt.Sprintf("e%02d", i), "running", "Up", "", 0))
	}
	m := send(newTestModel(t, fb), tea.WindowSizeMsg{Width: 120, Height: 14})
	m = keys(m, "end")
	if !strings.Contains(plain(m), "proj39") {
		t.Errorf("last project not visible:\n%s", plain(m))
	}
	m = keys(m, "home")
	if !strings.Contains(plain(m), "proj00") {
		t.Errorf("first project not visible:\n%s", plain(m))
	}
}

func TestRefreshErrorKeepsData(t *testing.T) {
	m := send(newTestModel(t, fixture()), dataMsg{err: errors.New("daemon down")})
	out := plain(m)
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "Refresh failed: daemon down") {
		t.Errorf("view:\n%s", out)
	}
}

func TestEventTriggersOneRefresh(t *testing.T) {
	m := newTestModel(t, fixture())
	m, cmd := step(m, eventMsg{})
	if cmd == nil {
		t.Fatal("first event: no command")
	}
	m, cmd = step(m, eventMsg{})
	if cmd != nil {
		t.Fatal("second event: command")
	}
	_, cmd = step(m, refreshNowMsg{})
	if !hasMsg[dataMsg](runCmd(t, cmd)) {
		t.Error("refreshNowMsg: no dataMsg")
	}
}

func TestInitStartsWatchersAndRefresh(t *testing.T) {
	m := newTestModel(t, fixture())
	if !hasMsg[dataMsg](runCmd(t, m.Init())) {
		t.Error("Init: no dataMsg")
	}
}
