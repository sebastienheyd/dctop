// Package ui is the full-screen interface of dctop, a Bubble Tea model that
// talks to Docker only through the Backend interface.
package ui

import (
	"context"
	"io"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/i18n"
	"github.com/sebastienheyd/dctop/internal/project"
)

// Backend is everything the interface needs from Docker.
type Backend interface {
	Containers(ctx context.Context) ([]docker.Container, error)
	Volumes(ctx context.Context) ([]docker.Volume, error)
	Images(ctx context.Context) ([]docker.Image, error)
	DiskUsage(ctx context.Context) (docker.DiskUsage, error)
	Run(ctx context.Context, a docker.Action) error
	WatchStats(ctx context.Context, out chan<- docker.Stat)
	WatchEvents(ctx context.Context, out chan<- struct{})
	Logs(ctx context.Context, t docker.Target) (io.ReadCloser, error)
	ShellCommand(containerID string) *exec.Cmd
}

// Options configures a Model.
type Options struct {
	Backend Backend
	Tr      *i18n.Translator
	Info    docker.Info
	Version string
	Home    string // user home directory, used to shorten paths
}

const (
	refreshTimeout  = 30 * time.Second
	debounceDelay   = 200 * time.Millisecond
	statsLifetime   = 5 * time.Second
	refreshEveryNth = 5 // ticks between two safety-net refreshes
)

// cursorPos remembers the row under the cursor by key, and its index as a
// fallback when that row disappears.
type cursorPos struct {
	key   rowKey
	index int
	valid bool // false until the cursor has been placed on a row
}

// statusLine is the message shown above the key bar.
type statusLine struct {
	text  string
	isErr bool
	at    time.Time
}

// Model is the Bubble Tea model of dctop.
type Model struct {
	backend Backend
	tr      *i18n.Translator
	info    docker.Info
	version string
	home    string

	ctx      context.Context
	cancel   context.CancelFunc
	statsCh  chan docker.Stat
	eventsCh chan struct{}

	containers []docker.Container
	volumes    []docker.Volume
	images     []docker.Image
	stats      map[string]docker.Stat // by short container ID

	now            time.Time
	ticks          int
	refreshPending bool

	tab      tab
	cursor   map[tab]cursorPos
	offset   map[tab]int
	expanded map[string]bool
	tagged   map[tab]map[rowKey]bool
	status   statusLine

	width, height int
}

// New builds the model. Watchers start in Init.
func New(o Options) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		backend:  o.Backend,
		tr:       o.Tr,
		info:     o.Info,
		version:  o.Version,
		home:     o.Home,
		ctx:      ctx,
		cancel:   cancel,
		statsCh:  make(chan docker.Stat, 256),
		eventsCh: make(chan struct{}, 1),
		stats:    map[string]docker.Stat{},
		now:      time.Now(),
		cursor:   map[tab]cursorPos{},
		offset:   map[tab]int{},
		expanded: map[string]bool{},
		tagged:   map[tab]map[rowKey]bool{},
		width:    80,
		height:   24,
	}
}

// Init starts the Docker watchers and returns the first refresh, the clock
// and the listeners of the two streams.
func (m Model) Init() tea.Cmd {
	go m.backend.WatchStats(m.ctx, m.statsCh)
	go m.backend.WatchEvents(m.ctx, m.eventsCh)
	return tea.Batch(m.refresh(), tick(), m.listenStats(), m.listenEvents())
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refresh reads the three lists; the first error wins.
func (m Model) refresh() tea.Cmd {
	b, parent := m.backend, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, refreshTimeout)
		defer cancel()
		cs, err := b.Containers(ctx)
		if err != nil {
			return dataMsg{err: err}
		}
		vs, err := b.Volumes(ctx)
		if err != nil {
			return dataMsg{err: err}
		}
		is, err := b.Images(ctx)
		if err != nil {
			return dataMsg{err: err}
		}
		return dataMsg{containers: cs, volumes: vs, images: is}
	}
}

func (m Model) listenStats() tea.Cmd {
	ch, ctx := m.statsCh, m.ctx
	return func() tea.Msg {
		select {
		case s := <-ch:
			return statMsg(s)
		case <-ctx.Done():
			return nil
		}
	}
}

func (m Model) listenEvents() tea.Cmd {
	ch, ctx := m.eventsCh, m.ctx
	return func() tea.Msg {
		select {
		case <-ch:
			return eventMsg{}
		case <-ctx.Done():
			return nil
		}
	}
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.scrolled(), nil
	case tea.KeyPressMsg:
		if m.status.isErr {
			m.status = statusLine{}
		}
		m, cmd := m.handleKey(msg)
		return m.scrolled(), cmd
	case dataMsg:
		return m.applyData(msg), nil
	case statMsg:
		m.stats[msg.ID] = docker.Stat(msg)
		return m, m.listenStats()
	case eventMsg:
		// The event listener is re-armed by refreshNowMsg: events arriving in
		// the meantime wait in the channel and are covered by that refresh.
		if m.refreshPending {
			return m, nil
		}
		m.refreshPending = true
		return m, tea.Tick(debounceDelay, func(time.Time) tea.Msg { return refreshNowMsg{} })
	case refreshNowMsg:
		m.refreshPending = false
		return m, tea.Batch(m.refresh(), m.listenEvents())
	case tickMsg:
		return m.onTick(time.Time(msg))
	}
	return m, nil
}

func (m Model) onTick(now time.Time) (tea.Model, tea.Cmd) {
	m.now = now
	m.ticks++
	for id, s := range m.stats {
		if now.Sub(s.Seen) > statsLifetime {
			delete(m.stats, id)
		}
	}
	cmds := []tea.Cmd{tick()}
	if m.ticks%refreshEveryNth == 0 {
		cmds = append(cmds, m.refresh())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) applyData(msg dataMsg) Model {
	if msg.err != nil {
		m.setError(m.tr.T("fail.refresh", "error", msg.err.Error()))
		return m
	}
	m.containers, m.volumes, m.images = msg.containers, msg.volumes, msg.images
	return m.reconcile()
}

// reconcile puts the cursor of every tab back on its row, or on the same
// index when the row is gone, and drops the tags of vanished rows.
func (m Model) reconcile() Model {
	for _, t := range []tab{tabProjects, tabVolumes, tabImages} {
		rows := m.rowsOf(t)
		present := make(map[rowKey]bool, len(rows))
		for _, r := range rows {
			present[r.key] = true
		}
		for k := range m.tagged[t] {
			if !present[k] {
				delete(m.tagged[t], k)
			}
		}
		if i := m.cursorIndex(t, rows); i >= 0 {
			m.cursor[t] = cursorPos{key: rows[i].key, index: i, valid: true}
		}
	}
	return m.scrolled()
}

func (m *Model) setInfo(text string) {
	m.status = statusLine{text: text, at: m.now}
}

func (m *Model) setError(text string) {
	m.status = statusLine{text: text, isErr: true, at: m.now}
}

// freshStats returns the samples seen during the last statsLifetime.
func (m Model) freshStats() []docker.Stat {
	out := make([]docker.Stat, 0, len(m.stats))
	for _, s := range m.stats {
		if m.now.Sub(s.Seen) <= statsLifetime {
			out = append(out, s)
		}
	}
	return out
}

// usage returns the recent usage of every container, by full ID.
func (m Model) usage() map[string]project.Usage {
	return project.UsageByContainer(m.freshStats(), m.containers)
}

// projectUsage sums the recent usage of a project's containers.
func (m Model) projectUsage(name string) project.Usage {
	for _, p := range project.Group(m.containers) {
		if p.Name == name {
			return p.Usage(m.usage())
		}
	}
	return project.Usage{}
}
