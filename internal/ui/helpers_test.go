package ui

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/i18n"
)

// fakeBackend serves fixed lists and records every action, log and shell request.
type fakeBackend struct {
	mu         sync.Mutex
	containers []docker.Container
	volumes    []docker.Volume
	images     []docker.Image
	du         docker.DiskUsage
	duCalls    int
	listErr    error
	duErr      error
	runErr     map[string]error // by Target.ID
	ran        []docker.Action
	gate       chan struct{} // when set, Run waits for it to be closed
	active     int
	maxActive  int
	blocked    int
	logs       io.ReadCloser
	logsErr    error
	logTargets []docker.Target
	shells     []string
}

func (f *fakeBackend) Containers(context.Context) ([]docker.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]docker.Container(nil), f.containers...), nil
}

func (f *fakeBackend) Volumes(context.Context) ([]docker.Volume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]docker.Volume(nil), f.volumes...), nil
}

func (f *fakeBackend) Images(context.Context) ([]docker.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]docker.Image(nil), f.images...), nil
}

func (f *fakeBackend) DiskUsage(context.Context) (docker.DiskUsage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.duCalls++
	return f.du, f.duErr
}

func (f *fakeBackend) Run(ctx context.Context, a docker.Action) error {
	f.mu.Lock()
	f.ran = append(f.ran, a)
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	gate := f.gate
	if gate != nil {
		f.blocked++
	}
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active--
	return f.runErr[a.Target.ID]
}

func (f *fakeBackend) WatchStats(ctx context.Context, _ chan<- docker.Stat) { <-ctx.Done() }

func (f *fakeBackend) WatchEvents(ctx context.Context, _ chan<- struct{}) { <-ctx.Done() }

func (f *fakeBackend) Logs(_ context.Context, t docker.Target) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logTargets = append(f.logTargets, t)
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	return f.logs, nil
}

func (f *fakeBackend) ShellCommand(id string) *exec.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shells = append(f.shells, id)
	return exec.Command("true")
}

func (f *fakeBackend) Ran() []docker.Action {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]docker.Action(nil), f.ran...)
}

func (f *fakeBackend) set(fn func(f *fakeBackend)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// cid pads prefix with "0" to a 64-character container ID.
func cid(prefix string) string { return prefix + strings.Repeat("0", 64-len(prefix)) }

func fixtureContainer(project, name, id, state, status, ports string, code int) docker.Container {
	c := docker.Container{
		ID: cid(id), Name: name, Image: "img", State: state, Status: status,
		ExitCode: code, Ports: ports, Project: project,
	}
	if project != "" {
		c.WorkingDir = "/home/me/docker/" + project
		c.Service = strings.TrimSuffix(strings.TrimPrefix(name, project+"-"), "-1")
	}
	return c
}

func fixture() *fakeBackend {
	db := fixtureContainer("alpha", "alpha-db-1", "a2", "running", "Up 2 days", "", 0)
	db.Mounts = []string{"alpha_data"}
	web := fixtureContainer("alpha", "alpha-web-1", "a1", "running", "Up 2 days (healthy)", "8080→80", 0)
	web.Health = "healthy"
	return &fakeBackend{
		containers: []docker.Container{
			web,
			db,
			fixtureContainer("beta", "beta-app-1", "b1", "running", "Up 3 hours", "", 0),
			fixtureContainer("beta", "beta-worker-1", "b2", "exited", "Exited (1) 5 minutes ago", "", 1),
			fixtureContainer("gamma", "gamma-app-1", "c1", "exited", "Exited (0) 2 days ago", "", 0),
			fixtureContainer("gamma", "gamma-db-1", "c2", "exited", "Exited (0) 2 days ago", "", 0),
			fixtureContainer("", "lonely", "d1", "running", "Up 1 hour", "", 0),
		},
		volumes: []docker.Volume{
			{Name: "alpha_data", Driver: "local", Project: "alpha"},
			{Name: "orphan", Driver: "local"},
			{Name: strings.Repeat("f", 64), Driver: "local", Anonymous: true},
		},
		images: []docker.Image{
			{ID: "sha256:" + strings.Repeat("1", 64), Repository: "nginx", Tag: "latest", Created: "2 weeks ago", Size: 50 << 20},
			{ID: "sha256:" + strings.Repeat("2", 64), Repository: "busybox", Tag: "1.36", Created: "3 months ago", Size: 4 << 20},
		},
		runErr: map[string]error{},
	}
}

func newTestModelLang(t *testing.T, fb *fakeBackend, lang string) Model {
	t.Helper()
	m := New(Options{
		Backend: fb,
		Tr:      i18n.New(lang),
		Info:    docker.Info{ServerVersion: "29.7.2", NCPU: 20, MemTotal: 16 << 30},
		Version: "test",
		Home:    "/home/me",
	})
	t.Cleanup(m.cancel)
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return send(m, m.refresh()())
}

// newTestModel returns an English model, 120x30, with the fixture loaded.
func newTestModel(t *testing.T, fb *fakeBackend) Model {
	return newTestModelLang(t, fb, "en")
}

func step(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		m, _ = step(m, msg)
	}
	return m
}

var namedKeys = map[string]rune{
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace,
	"backspace": tea.KeyBackspace, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	"home": tea.KeyHome, "end": tea.KeyEnd,
	"f1": tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5,
	"f6": tea.KeyF6, "f7": tea.KeyF7, "f8": tea.KeyF8, "f9": tea.KeyF9, "f10": tea.KeyF10,
}

// key builds the key press whose String() is s.
func key(s string) tea.Msg {
	var mod tea.KeyMod
	name := s
	for _, p := range []struct {
		prefix string
		mod    tea.KeyMod
	}{{"ctrl+", tea.ModCtrl}, {"alt+", tea.ModAlt}, {"shift+", tea.ModShift}} {
		if len(name) > len(p.prefix) && strings.HasPrefix(name, p.prefix) {
			mod |= p.mod
			name = name[len(p.prefix):]
		}
	}
	if code, ok := namedKeys[name]; ok {
		k := tea.Key{Code: code, Mod: mod}
		if code == tea.KeySpace {
			k.Text = " "
		}
		return tea.KeyPressMsg(k)
	}
	r, _ := utf8.DecodeRuneInString(name)
	k := tea.Key{Code: unicode.ToLower(r), Mod: mod}
	if mod == 0 {
		k.Text = name
		if unicode.IsUpper(r) {
			k.ShiftedCode = r
			k.Mod = tea.ModShift
		}
	}
	return tea.KeyPressMsg(k)
}

func keys(m Model, names ...string) Model {
	for _, n := range names {
		m = send(m, key(n))
	}
	return m
}

func plain(m Model) string { return ansi.Strip(m.View().Content) }

// runCmd runs cmd and returns the messages it produced, unwrapping batches.
// Each leaf runs in its own goroutine and is dropped if it has not returned
// after 500 ms (listeners waiting on a channel, ticks).
func runCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	var leaves []tea.Cmd
	var flatten func(c tea.Cmd)
	flatten = func(c tea.Cmd) {
		if c == nil {
			return
		}
		leaves = append(leaves, c)
	}
	flatten(cmd)
	var out []tea.Msg
	for len(leaves) > 0 {
		results := make([]chan tea.Msg, len(leaves))
		for i, c := range leaves {
			ch := make(chan tea.Msg, 1)
			results[i] = ch
			go func(c tea.Cmd) { ch <- c() }(c)
		}
		leaves = nil
		timer := time.NewTimer(500 * time.Millisecond)
		expired := false
		for _, ch := range results {
			var msg tea.Msg
			if !expired {
				select {
				case msg = <-ch:
				case <-timer.C:
					expired = true
				}
			}
			if expired {
				select {
				case msg = <-ch:
				default:
				}
			}
			if b, ok := msg.(tea.BatchMsg); ok {
				for _, c := range b {
					flatten(c)
				}
			} else if msg != nil {
				out = append(out, msg)
			}
		}
		timer.Stop()
	}
	return out
}

// lineWith returns the first line of the view containing s.
func lineWith(m Model, s string) string {
	for _, l := range strings.Split(plain(m), "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

func hasMsg[T any](msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}
