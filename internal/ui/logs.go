package ui

import (
	"bufio"
	"context"
	"io"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sebastienheyd/dctop/internal/docker"
)

const (
	maxLogLines  = 5000
	maxLogPiece  = 1 << 20 // longer lines are delivered in several pieces
	maxLogBatch  = 500
	modeLogs     = modeHelp + 1
	logsChanSize = 64
)

// logLinesMsg carries a batch of log lines of one logs session.
type logLinesMsg struct {
	session int
	lines   []string
}

// logEndMsg reports that the log stream of a session ended.
type logEndMsg struct {
	session int
	err     error
}

// logsState is the logs view. session tells apart the messages of a view
// that was closed from those of the current one.
type logsState struct {
	session int
	title   string
	lines   []string
	follow  bool
	ended   bool
	cancel  context.CancelFunc
	closer  io.Closer
	ch      chan []string
	vp      viewport.Model
}

// readLines calls emit with batches of lines until r ends. A line longer
// than 1 MiB is delivered in several pieces.
func readLines(r io.Reader, emit func([]string)) {
	br := bufio.NewReaderSize(r, maxLogPiece)
	var batch []string
	for {
		line, _, err := br.ReadLine()
		if len(line) > 0 || err == nil {
			batch = append(batch, string(line))
		}
		if err != nil {
			if len(batch) > 0 {
				emit(batch)
			}
			return
		}
		if br.Buffered() == 0 || len(batch) >= maxLogBatch {
			emit(batch)
			batch = nil
		}
	}
}

// sanitize removes escape sequences and carriage returns and expands tabs.
func sanitize(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "\t", "    ")
}

// logsTarget returns the target of the logs view for the cursor row.
func (m Model) logsTarget() (docker.Target, bool) {
	r, ok := m.current()
	switch {
	case !ok:
		return docker.Target{}, false
	case r.container != nil:
		return docker.Target{Kind: docker.KindContainer, ID: r.container.ID, Name: r.container.Name}, true
	case r.project != nil && !r.project.Standalone():
		return docker.Target{Kind: docker.KindProject, ID: r.project.Name, Name: r.project.Name}, true
	}
	return docker.Target{}, false
}

// openLogs starts following the logs of the row under the cursor.
func (m Model) openLogs() (Model, tea.Cmd) {
	if m.tab != tabProjects {
		return m, nil
	}
	t, ok := m.logsTarget()
	if !ok {
		m.setInfo(m.tr.T("msg.logs_need_target"))
		return m, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	rc, err := m.backend.Logs(ctx, t)
	if err != nil {
		cancel()
		m.setError(m.tr.T("fail.refresh", "error", err.Error()))
		return m, nil
	}
	ch := make(chan []string, logsChanSize)
	go func() {
		defer close(ch)
		readLines(rc, func(b []string) {
			select {
			case ch <- b:
			case <-ctx.Done():
			}
		})
	}()
	vp := viewport.New()
	vp.SoftWrap = true
	m.logs = logsState{
		session: m.logs.session + 1,
		title:   t.Name,
		follow:  true,
		cancel:  cancel,
		closer:  rc,
		ch:      ch,
		vp:      vp,
	}
	m.mode = modeLogs
	m = m.sizeLogs()
	return m, m.listenLogs()
}

func (m Model) listenLogs() tea.Cmd {
	session, ch := m.logs.session, m.logs.ch
	return func() tea.Msg {
		b, ok := <-ch
		if !ok {
			return logEndMsg{session: session}
		}
		return logLinesMsg{session: session, lines: b}
	}
}

// closeLogs stops the stream and returns to the list.
func (m Model) closeLogs() Model {
	if m.logs.cancel != nil {
		m.logs.cancel()
	}
	if m.logs.closer != nil {
		m.logs.closer.Close()
	}
	m.logs.lines, m.logs.ch, m.logs.cancel, m.logs.closer = nil, nil, nil, nil
	m.mode = modeMain
	return m
}

func (m Model) sizeLogs() Model {
	m.logs.vp.SetWidth(max(1, m.width))
	m.logs.vp.SetHeight(max(1, m.height-2))
	if m.logs.follow {
		m.logs.vp.GotoBottom()
	}
	return m
}

func (m Model) appendLogs(msg logLinesMsg) (Model, tea.Cmd) {
	if m.mode != modeLogs || msg.session != m.logs.session {
		return m, nil
	}
	for _, l := range msg.lines {
		m.logs.lines = append(m.logs.lines, sanitize(l))
	}
	if extra := len(m.logs.lines) - maxLogLines; extra > 0 {
		m.logs.lines = append([]string(nil), m.logs.lines[extra:]...)
	}
	m.logs.vp.SetContentLines(m.logs.lines)
	if m.logs.follow {
		m.logs.vp.GotoBottom()
	}
	return m, m.listenLogs()
}

func (m Model) endLogs(msg logEndMsg) Model {
	if m.mode == modeLogs && msg.session == m.logs.session {
		m.logs.ended = true
	}
	return m
}

// logsKey handles a key in the logs view: Esc or q return, f or End resume
// following, anything else scrolls and pauses following away from the end.
func (m Model) logsKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m.closeLogs(), nil
	case "f", "end":
		m.logs.follow = true
		m.logs.vp.GotoBottom()
		return m, nil
	case "home":
		m.logs.vp.GotoTop()
		m.logs.follow = m.logs.vp.AtBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.logs.vp, cmd = m.logs.vp.Update(msg)
	m.logs.follow = m.logs.vp.AtBottom()
	return m, cmd
}

// logsLines renders the logs view: title, log lines, key bar.
func (m Model) logsLines() []string {
	state := m.tr.T("logs.following")
	switch {
	case m.logs.ended:
		state = m.tr.T("logs.ended")
	case !m.logs.follow:
		state = m.tr.T("logs.paused")
	}
	title := styleKeyName.Render(m.tr.T("logs.title", "name", m.logs.title)) + "  [" + state + "]"
	lines := []string{title}
	body := strings.Split(m.logs.vp.View(), "\n")
	h := max(1, m.height-2)
	for i := 0; i < h; i++ {
		if i < len(body) {
			lines = append(lines, body[i])
		} else {
			lines = append(lines, "")
		}
	}
	bar := renderKeyBar(m.tr, []keyBinding{{"Esc", "key.back"}, {"F", "key.follow"}})
	return append(lines, bar)
}
