package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

func projectTarget(name string) docker.Target {
	return docker.Target{Kind: docker.KindProject, ID: name, Name: name}
}

// deliver runs cmd and feeds every message it produced back to the model.
func deliver(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range runCmd(t, cmd) {
		m = send(m, msg)
	}
	return m
}

func TestStopProjectUnderCursor(t *testing.T) {
	fb := fixture()
	m := newTestModel(t, fb)
	m, cmd := step(m, key("x"))
	if cmd == nil {
		t.Fatal("no command")
	}
	if l := lineWith(m, "alpha"); !strings.Contains(l, "stopping…") {
		t.Errorf("alpha row: %q", l)
	}
	msgs := runCmd(t, cmd)
	var done []actionDoneMsg
	for _, msg := range msgs {
		if d, ok := msg.(actionDoneMsg); ok {
			done = append(done, d)
		}
	}
	if len(done) != 1 {
		t.Fatalf("done messages: %d", len(done))
	}
	if want := []docker.Action{{Op: docker.OpStop, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
	m = send(m, done[0])
	out := plain(m)
	if strings.Contains(out, "stopping…") || !strings.Contains(out, "Stopped alpha") {
		t.Errorf("after completion:\n%s", out)
	}
}

func TestFunctionKeysMatchLetters(t *testing.T) {
	pairs := []struct {
		fn, letter string
		op         docker.Op
	}{{"f2", "s", docker.OpStart}, {"f3", "x", docker.OpStop}, {"f5", "r", docker.OpRestart}}
	for _, p := range pairs {
		for _, k := range []string{p.fn, p.letter} {
			fb := fixture()
			m := newTestModel(t, fb)
			_, cmd := step(m, key(k))
			runCmd(t, cmd)
			if want := []docker.Action{{Op: p.op, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
				t.Errorf("%s: %+v", k, fb.Ran())
			}
		}
	}
	for _, k := range []string{"f9", "d"} {
		fb := fixture()
		m := keys(newTestModel(t, fb), k)
		_, cmd := step(m, key("y"))
		runCmd(t, cmd)
		if want := []docker.Action{{Op: docker.OpRemove, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
			t.Errorf("%s: %+v", k, fb.Ran())
		}
	}
}

func TestTaggedRowsAreTheTargets(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "space", "down", "space", "up", "up")
	if currentKey(t, m) != projectKey("beta") {
		t.Fatalf("cursor: %+v", currentKey(t, m))
	}
	m, cmd := step(m, key("s"))
	runCmd(t, cmd)
	want := []docker.Action{{Op: docker.OpStart, Target: projectTarget("alpha")}, {Op: docker.OpStart, Target: projectTarget("gamma")}}
	got := fb.Ran()
	if len(got) != 2 || !containsAll(got, want) {
		t.Errorf("ran: %+v", got)
	}
	if len(m.tagged[tabProjects]) != 0 {
		t.Errorf("tags not cleared: %v", m.tagged[tabProjects])
	}
}

func containsAll(got, want []docker.Action) bool {
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestContainerSkippedWhenItsProjectIsTargeted(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "right", "space", "space")
	if len(m.tagged[tabProjects]) != 2 {
		t.Fatalf("tags: %v", m.tagged[tabProjects])
	}
	_, cmd := step(m, key("r"))
	runCmd(t, cmd)
	if want := []docker.Action{{Op: docker.OpRestart, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

func TestStandaloneGroupExpandsToContainers(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "end")
	_, cmd := step(m, key("x"))
	runCmd(t, cmd)
	want := []docker.Action{{Op: docker.OpStop, Target: docker.Target{Kind: docker.KindContainer, ID: cid("d1"), Name: "lonely"}}}
	if !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

func TestBusyRowIsSkipped(t *testing.T) {
	fb := fixture()
	m := newTestModel(t, fb)
	m, cmd1 := step(m, key("x"))
	_, cmd2 := step(m, key("x"))
	if cmd2 != nil {
		t.Error("second press returned a command")
	}
	runCmd(t, cmd1)
	if len(fb.Ran()) != 1 {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

func TestRemoveAsksFirst(t *testing.T) {
	fb := fixture()
	m := newTestModel(t, fb)
	m, cmd := step(m, key("d"))
	if cmd != nil {
		t.Error("remove ran without confirmation")
	}
	assertContains(t, plain(m),
		"Remove project alpha? Its containers and networks will be removed. Volumes are kept.",
		"y: confirm · any other key: cancel")
	for _, cancel := range []string{"n", "esc", "q"} {
		mm := keys(newTestModel(t, fb), "d")
		mm, cmd := step(mm, key(cancel))
		if strings.Contains(plain(mm), "Remove project alpha?") {
			t.Errorf("%s: dialog still open", cancel)
		}
		if hasMsg[tea.QuitMsg](runCmd(t, cmd)) {
			t.Errorf("%s: program quits", cancel)
		}
	}
	if len(fb.Ran()) != 0 {
		t.Fatalf("ran: %+v", fb.Ran())
	}
	m, cmd = step(m, key("y"))
	runCmd(t, cmd)
	if want := []docker.Action{{Op: docker.OpRemove, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

func TestRemoveContainerAndMany(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "right", "down", "down", "d")
	assertContains(t, plain(m), "Remove container alpha-web-1?")

	m = keys(newTestModel(t, fixture()), "space", "down", "space", "d")
	want := "Remove these 2 items: alpha, gamma? Containers and networks of the projects will be removed. Volumes are kept."
	if m.confirm == nil || m.confirm.text != want {
		t.Errorf("projects: %+v", m.confirm)
	}
	assertContains(t, plain(m), "Volumes are kept.")

	fb := fixture()
	for _, n := range []string{"v1", "v2", "v3", "v4"} {
		fb.volumes = append(fb.volumes, docker.Volume{Name: n, Driver: "local"})
	}
	m = newTestModel(t, fb)
	m = keys(m, "2")
	for range fb.volumes {
		m = keys(m, "space")
	}
	m = keys(m, "d")
	want = "Remove these 7 items: alpha_data, orphan, v1, v2, v3 and 2 more? Volume data will be lost."
	if m.confirm == nil || m.confirm.text != want {
		t.Errorf("volumes: %+v", m.confirm)
	}
}

func TestFrenchYesKey(t *testing.T) {
	for _, yes := range []string{"o", "y"} {
		fb := fixture()
		m := keys(newTestModelLang(t, fb, "fr"), "d")
		_, cmd := step(m, key(yes))
		runCmd(t, cmd)
		if len(fb.Ran()) != 1 {
			t.Errorf("%s: ran %+v", yes, fb.Ran())
		}
	}
}

func TestConfirmKeepsItsTargets(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "d")
	m = send(m, without(fb, "alpha"))
	if currentKey(t, m) != projectKey("beta") {
		t.Fatalf("cursor: %+v", currentKey(t, m))
	}
	_, cmd := step(m, key("y"))
	runCmd(t, cmd)
	if want := []docker.Action{{Op: docker.OpRemove, Target: projectTarget("alpha")}}; !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

func TestFailureShowsDockerMessage(t *testing.T) {
	fb := fixture()
	fb.runErr["alpha"] = errors.New("no such service")
	m := send(newTestModel(t, fb), tickMsg(t0))
	m, cmd := step(m, key("x"))
	for _, msg := range runCmd(t, cmd) {
		if d, ok := msg.(actionDoneMsg); ok {
			m = send(m, d)
		}
	}
	if !strings.Contains(plain(m), "Failed to stop alpha: no such service") {
		t.Errorf("view:\n%s", plain(m))
	}
	if strings.Contains(lineWith(m, "alpha"), "stopping…") {
		t.Error("row still busy")
	}
	m = send(m, tickMsg(t0.Add(60*time.Second)))
	if !strings.Contains(plain(m), "Failed to stop alpha") {
		t.Error("error expired")
	}
	m = keys(m, "down")
	if strings.Contains(plain(m), "Failed to stop alpha") {
		t.Error("error kept after a key press")
	}
}

func TestActionCompletionRefreshes(t *testing.T) {
	m := newTestModel(t, fixture())
	_, cmd := step(m, actionDoneMsg{action: docker.Action{Op: docker.OpStop, Target: projectTarget("alpha")}})
	if !hasMsg[dataMsg](runCmd(t, cmd)) {
		t.Error("no refresh")
	}
}

func TestAtMostFourActionsAtOnce(t *testing.T) {
	fb := fixture()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("extra%d", i)
		fb.containers = append(fb.containers, fixtureContainer(id, id+"-app-1", fmt.Sprintf("e%d", i), "running", "Up", "", 0))
	}
	gate := make(chan struct{})
	fb.gate = gate
	m := newTestModel(t, fb)
	m = keys(m, "space", "space", "space", "space", "space", "space")
	if len(m.tagged[tabProjects]) != 6 {
		t.Fatalf("tags: %d", len(m.tagged[tabProjects]))
	}
	_, cmd := step(m, key("x"))
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 6 {
		t.Fatalf("batch: %T %d", cmd(), len(batch))
	}
	var wg sync.WaitGroup
	results := make(chan tea.Msg, 6)
	for _, c := range batch {
		wg.Add(1)
		go func(c tea.Cmd) { defer wg.Done(); results <- c() }(c)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		fb.mu.Lock()
		blocked := fb.blocked
		fb.mu.Unlock()
		if blocked >= 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	fb.mu.Lock()
	if fb.blocked != 4 {
		t.Errorf("blocked calls: %d", fb.blocked)
	}
	fb.mu.Unlock()
	close(gate)
	wg.Wait()
	close(results)
	n := 0
	for msg := range results {
		if _, ok := msg.(actionDoneMsg); ok {
			n++
		}
	}
	if n != 6 || fb.maxActive != 4 {
		t.Errorf("done %d, max concurrency %d", n, fb.maxActive)
	}
}
