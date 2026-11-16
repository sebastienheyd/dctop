package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func assertContains(t *testing.T, s string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Errorf("%q not found in:\n%s", n, s)
		}
	}
}

func assertNotContains(t *testing.T, s string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(s, n) {
			t.Errorf("%q unexpectedly found in:\n%s", n, s)
		}
	}
}

func TestHeaderCounts(t *testing.T) {
	out := plain(newTestModel(t, fixture()))
	assertContains(t, out, "Projects", "2/3", "Containers", "4/7", "Volumes: 3", "Images: 2", "dctop test · Docker 29.7.2")
}

func TestHeaderUsage(t *testing.T) {
	m := send(newTestModel(t, fixture()),
		statMsg{ID: cid("a1")[:12], CPU: 150, Mem: 3 << 30, Seen: t0},
		statMsg{ID: cid("a2")[:12], CPU: 50, Mem: 1 << 30, Seen: t0},
		tickMsg(t0),
	)
	assertContains(t, plain(m), "10.0%", "4.00G/16.0G")
}

func TestProjectRowContent(t *testing.T) {
	m := newTestModel(t, fixture())
	if l := lineWith(m, "▶ alpha"); !strings.Contains(l, "running 2/2") || !strings.Contains(l, "~/docker/alpha") {
		t.Errorf("alpha: %q", l)
	}
	if l := lineWith(m, "▶ beta"); !strings.Contains(l, "partial 1/2") {
		t.Errorf("beta: %q", l)
	}
	if l := lineWith(m, "▶ gamma"); !strings.Contains(l, "stopped 0/2") {
		t.Errorf("gamma: %q", l)
	}
	mb := keys(newTestModel(t, fixture()), "down", "right")
	if l := lineWith(mb, "beta-worker-1"); !strings.Contains(l, "exited (1)") || !strings.Contains(l, "Exited (1) 5 minutes ago") {
		t.Errorf("beta-worker-1: %q", l)
	}
	ma := keys(newTestModel(t, fixture()), "right")
	if l := lineWith(ma, "alpha-web-1"); !strings.Contains(l, "running") || !strings.Contains(l, "Up 2 days (healthy)") || !strings.Contains(l, "8080→80") {
		t.Errorf("alpha-web-1: %q", l)
	}
	ma = send(ma, alphaStats()...)
	if l := lineWith(ma, "alpha "); !strings.Contains(l, "2.0%") || !strings.Contains(l, "150M") {
		t.Errorf("alpha usage: %q", l)
	}
	if l := lineWith(ma, "alpha-web-1"); !strings.Contains(l, "1.5%") || !strings.Contains(l, "100M") {
		t.Errorf("alpha-web-1 usage: %q", l)
	}
	ma = send(ma, tickMsg(t0.Add(6*time.Second)))
	for _, name := range []string{"alpha ", "alpha-web-1"} {
		l := lineWith(ma, name)
		for _, v := range []string{"2.0%", "150M", "1.5%", "100M"} {
			if strings.Contains(l, v) {
				t.Errorf("stale %s in %q", v, l)
			}
		}
	}
}

func TestKeyBarAndTabs(t *testing.T) {
	m := newTestModel(t, fixture())
	out := plain(m)
	assertContains(t, out, "F1Help", "F2Start", "F3Stop", "F4Filter", "F5Restart", "F6Sort", "F7Logs", "F8Shell", "F9Remove", "F10Quit")
	assertContains(t, out, "Projects", "Volumes", "Images")
	assertContains(t, lineWith(m, "NAME"), "NAME", "STATE", "CPU%", "MEM", "INFO")
}

func TestFrench(t *testing.T) {
	out := plain(newTestModelLang(t, fixture(), "fr"))
	assertContains(t, out, "Projets", "F10Quitter", "démarré 2/2", "Conteneurs hors projet", "ÉTAT")
}

func TestTooSmall(t *testing.T) {
	m := send(newTestModel(t, fixture()), tea.WindowSizeMsg{Width: 50, Height: 10})
	out := plain(m)
	assertContains(t, out, "Terminal too small (minimum 60×12)")
	assertNotContains(t, out, "alpha")
}

func TestFrameFitsTerminal(t *testing.T) {
	fb := fixture()
	fb.containers = append(fb.containers,
		fixtureContainer(strings.Repeat("x", 90), "x-app-1", "f1", "running", "Up", "", 0),
		fixtureContainer("プロジェクト-日本語", "日本語-app-1", "f2", "running", strings.Repeat("Up a long time ", 20), "1→1, 2→2", 0),
	)
	m := newTestModel(t, fb)
	for _, p := range m.projects() {
		m.expanded[p.Name] = true
	}
	for _, size := range [][2]int{{60, 12}, {79, 20}, {80, 24}, {120, 30}, {200, 50}} {
		w, h := size[0], size[1]
		mm := send(m, tea.WindowSizeMsg{Width: w, Height: h})
		lines := strings.Split(mm.View().Content, "\n")
		if len(lines) != h {
			t.Errorf("%dx%d: %d lines", w, h, len(lines))
		}
		for i, l := range lines {
			if lipgloss.Width(l) > w {
				t.Errorf("%dx%d: line %d is %d wide: %q", w, h, i, lipgloss.Width(l), l)
			}
		}
	}
	for _, size := range [][2]int{{1, 1}, {0, 0}, {10, 3}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			mm := send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			_ = mm.View()
		})
	}
}

func TestStatusLineExpires(t *testing.T) {
	m := send(newTestModel(t, fixture()), tickMsg(t0))
	m.setInfo("hello there")
	if !strings.Contains(plain(send(m, tickMsg(t0.Add(3*time.Second)))), "hello there") {
		t.Error("info gone too early")
	}
	if strings.Contains(plain(send(m, tickMsg(t0.Add(5*time.Second)))), "hello there") {
		t.Error("info still shown after 5 s")
	}
	m.setError("it broke")
	m = send(m, tickMsg(t0.Add(60*time.Second)))
	if !strings.Contains(plain(m), "it broke") {
		t.Error("error gone without a key press")
	}
	m = keys(m, "down")
	if strings.Contains(plain(m), "it broke") {
		t.Error("error still shown after a key press")
	}
}
