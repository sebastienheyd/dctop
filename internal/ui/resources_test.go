package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sebastienheyd/dctop/internal/docker"
)

var nginxID = "sha256:" + strings.Repeat("1", 64)

func volumeNames(m Model) []string {
	out := []string{}
	for _, r := range m.rows() {
		out = append(out, r.key.id)
	}
	return out
}

func TestTabSwitching(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "tab")
	if m.tab != tabVolumes {
		t.Fatalf("tab: %d", m.tab)
	}
	assertContains(t, lineWith(m, "NAME"), "NAME", "PROJECT", "DRIVER", "USED", "SIZE")
	m = keys(m, "tab")
	assertContains(t, lineWith(m, "NAME"), "NAME", "ID", "SIZE", "CREATED", "USED")
	m = keys(m, "tab")
	if m.tab != tabProjects {
		t.Errorf("tab wraps: %d", m.tab)
	}
	m = keys(m, "shift+tab")
	if m.tab != tabImages {
		t.Errorf("shift+tab: %d", m.tab)
	}
	for k, want := range map[string]tab{"1": tabProjects, "2": tabVolumes, "3": tabImages} {
		if m = keys(m, k); m.tab != want {
			t.Errorf("%s: %d", k, m.tab)
		}
	}
	m = keys(m, "2", "down", "1", "down", "down", "2")
	if currentKey(t, m) != (rowKey{kind: docker.KindVolume, id: "orphan"}) {
		t.Errorf("volumes cursor: %+v", currentKey(t, m))
	}
	m = keys(m, "1")
	if currentKey(t, m) != projectKey("gamma") {
		t.Errorf("projects cursor: %+v", currentKey(t, m))
	}
}

func TestVolumeRows(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "2")
	if got := volumeNames(m); !reflect.DeepEqual(got, []string{"alpha_data", "orphan", strings.Repeat("f", 64)}) {
		t.Errorf("order: %q", got)
	}
	assertContains(t, plain(m), "ffffffffffff…")
	if l := lineWith(m, "alpha_data"); !strings.Contains(l, "alpha") || !strings.Contains(l, "local") || !strings.Contains(l, "yes") || !strings.Contains(l, "…") {
		t.Errorf("alpha_data: %q", l)
	}
	if l := lineWith(m, "orphan"); !strings.Contains(l, "no") {
		t.Errorf("orphan: %q", l)
	}
	m = send(m, diskUsageMsg{du: docker.DiskUsage{
		VolumeSize:  map[string]int64{"alpha_data": 1385000000},
		VolumeLinks: map[string]int{"alpha_data": 1, "orphan": 0},
	}})
	if l := lineWith(m, "alpha_data"); !strings.Contains(l, "1.29G") {
		t.Errorf("alpha_data size: %q", l)
	}
}

func TestImageRows(t *testing.T) {
	fb := fixture()
	fb.images = append(fb.images, docker.Image{ID: "sha256:" + strings.Repeat("3", 64), Repository: "<none>", Tag: "<none>", Size: 1})
	m := keys(newTestModel(t, fb), "3")
	l := lineWith(m, "nginx:latest")
	if !strings.Contains(l, "111111111111") || !strings.Contains(l, "47.7M") || !strings.HasSuffix(strings.TrimRight(l, " "), "…") {
		t.Errorf("nginx: %q", l)
	}
	m = send(m, diskUsageMsg{du: docker.DiskUsage{ImageContainers: map[string]int{nginxID: 2}}})
	if l := strings.TrimRight(lineWith(m, "nginx:latest"), " "); !strings.HasSuffix(l, "2") {
		t.Errorf("nginx used: %q", l)
	}
	assertContains(t, plain(m), "<none>:<none>")
}

func TestDiskUsageLoadedOnDemand(t *testing.T) {
	fb := fixture()
	m := send(newTestModel(t, fb), tickMsg(t0))
	m, cmd := step(m, key("2"))
	msgs := runCmd(t, cmd)
	if !hasMsg[diskUsageMsg](msgs) {
		t.Fatal("entering Volumes does not load disk usage")
	}
	for _, msg := range msgs {
		m = send(m, msg)
	}
	m, cmd = step(m, key("3"))
	if hasMsg[diskUsageMsg](runCmd(t, cmd)) {
		t.Error("loaded again within 30 s")
	}
	m = send(m, key("1"), tickMsg(t0.Add(31*time.Second)))
	m, cmd = step(m, key("3"))
	if !hasMsg[diskUsageMsg](runCmd(t, cmd)) {
		t.Error("not reloaded after 30 s")
	}
	m = keys(m, "1")
	m, cmd = step(m, key("2"))
	if hasMsg[diskUsageMsg](runCmd(t, cmd)) {
		t.Error("second load while one is running")
	}
	m = send(m, diskUsageMsg{err: errors.New("df failed")})
	assertContains(t, plain(m), "Refresh failed: df failed", "alpha_data")
}

func TestRemoveVolume(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "2", "d")
	assertContains(t, plain(m), "Remove volume alpha_data? Its data will be lost.")
	m, cmd := step(m, key("y"))
	var done actionDoneMsg
	for _, msg := range runCmd(t, cmd) {
		if d, ok := msg.(actionDoneMsg); ok {
			done = d
		}
	}
	want := []docker.Action{{Op: docker.OpRemove, Target: docker.Target{Kind: docker.KindVolume, ID: "alpha_data", Name: "alpha_data"}}}
	if !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
	m.duLoading = false
	_, cmd = step(m, done)
	if !hasMsg[diskUsageMsg](runCmd(t, cmd)) {
		t.Error("removal does not reload disk usage")
	}
}

func TestRemoveImage(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "3", "down", "d")
	assertContains(t, plain(m), "Remove image nginx:latest?")
	_, cmd := step(m, key("y"))
	runCmd(t, cmd)
	want := []docker.Action{{Op: docker.OpRemove, Target: docker.Target{Kind: docker.KindImage, ID: "nginx:latest", Name: "nginx:latest"}}}
	if !reflect.DeepEqual(fb.Ran(), want) {
		t.Errorf("ran: %+v", fb.Ran())
	}
}

// docker image ls prints one row per tag, all with the same image ID.
func TestImageWithSeveralTags(t *testing.T) {
	fb := fixture()
	dangling := "sha256:" + strings.Repeat("3", 64)
	fb.images = append(fb.images,
		docker.Image{ID: nginxID, Repository: "nginx", Tag: "1.27", Created: "2 weeks ago", Size: 50_000_000},
		docker.Image{ID: dangling, Repository: "<none>", Tag: "<none>", Created: "1 year ago", Size: 1_000},
	)
	m := keys(newTestModel(t, fb), "3")
	var seen []string
	for range 4 {
		r, _ := m.current()
		seen = append(seen, imageName(r.image))
		m = keys(m, "down")
	}
	if want := []string{"<none>:<none>", "busybox:1.36", "nginx:1.27", "nginx:latest"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("cursor went through %q", seen)
	}
	m = keys(m, "home", "space")
	if n := len(m.tagged[tabImages]); n != 1 {
		t.Errorf("space tagged %d rows", n)
	}
	m = keys(m, "U", "end", "d")
	m, cmd := step(m, key("y"))
	runCmd(t, cmd)
	m = keys(m, "home", "d")
	_, cmd = step(m, key("y"))
	runCmd(t, cmd)
	ran := fb.Ran()
	if len(ran) != 2 || ran[0].Target.ID != "nginx:latest" || ran[1].Target.ID != dangling {
		t.Errorf("ran: %+v", ran)
	}
}

func TestOnlyRemoveAppliesToResources(t *testing.T) {
	for _, tabKey := range []string{"2", "3"} {
		fb := fixture()
		m := keys(newTestModel(t, fb), tabKey)
		m.duLoading = true // keep disk usage loads out of the way
		before := plain(m)
		for _, k := range []string{"s", "x", "r", "l", "e", "right"} {
			var cmd = func() bool { _, c := step(m, key(k)); return c != nil }()
			if cmd {
				t.Errorf("tab %s, %s: command returned", tabKey, k)
			}
			m = keys(m, k)
		}
		if plain(m) != before || len(fb.Ran()) != 0 || len(fb.logTargets) != 0 || len(fb.shells) != 0 {
			t.Errorf("tab %s: something changed", tabKey)
		}
		bar := lastLine(m)
		assertContains(t, bar, "F1", "F4", "F6", "F9", "F10")
		assertNotContains(t, bar, "F2", "F3", "F5", "F7", "F8")
	}
}

func TestTagsAreKeptPerTab(t *testing.T) {
	fb := fixture()
	m := keys(newTestModel(t, fb), "space", "2", "d")
	assertContains(t, plain(m), "Remove volume alpha_data?")
	m = keys(m, "esc", "1")
	if !m.tagged[tabProjects][projectKey("alpha")] {
		t.Error("projects tag lost")
	}
}

func TestResourceFilterAndSort(t *testing.T) {
	m := keys(newTestModel(t, fixture()), "2", "/", "a", "l", "p", "h", "a", "enter")
	if got := volumeNames(m); !reflect.DeepEqual(got, []string{"alpha_data"}) {
		t.Errorf("volume filter: %q", got)
	}
	m = keys(m, "3", "/", "b", "u", "s", "y", "enter")
	if got := volumeNames(m); !reflect.DeepEqual(got, []string{"busybox:1.36"}) {
		t.Errorf("image filter: %q", got)
	}

	fb := fixture()
	fb.volumes = append(fb.volumes, docker.Volume{Name: "zeta", Driver: "local", Project: "aaa"})
	m = keys(newTestModel(t, fb), "2")
	m = send(m, diskUsageMsg{du: docker.DiskUsage{VolumeSize: map[string]int64{"orphan": 500, "alpha_data": 100, strings.Repeat("f", 64): 900}}})
	anon := strings.Repeat("f", 64)
	steps := []struct {
		want  []string
		title string
	}{
		{[]string{"alpha_data", "orphan", "zeta", anon}, "NAME▼"},
		{[]string{"orphan", "zeta", "alpha_data", anon}, "PROJECT▼"},
		{[]string{"orphan", "alpha_data", "zeta", anon}, "SIZE▼"},
		{[]string{"alpha_data", "orphan", "zeta", anon}, "NAME▼"},
	}
	for i, s := range steps {
		if i > 0 {
			m = keys(m, "o")
		}
		if got := volumeNames(m); !reflect.DeepEqual(got, s.want) {
			t.Errorf("volume sort %d: %q", i, got)
		}
		assertContains(t, lineWith(m, "NAME"), s.title)
	}

	m = keys(newTestModel(t, fixture()), "3")
	img1, img2 := "nginx:latest", "busybox:1.36"
	for i, want := range [][]string{{img2, img1}, {img1, img2}, {img2, img1}} {
		if i > 0 {
			m = keys(m, "o")
		}
		if got := volumeNames(m); !reflect.DeepEqual(got, want) {
			t.Errorf("image sort %d: %q", i, got)
		}
	}
}
