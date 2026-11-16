package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// blockingLog is a log stream that never delivers anything and records Close.
type blockingLog struct {
	pr     *io.PipeReader
	pw     *io.PipeWriter
	closed atomic.Bool
}

func newBlockingLog() *blockingLog {
	pr, pw := io.Pipe()
	return &blockingLog{pr: pr, pw: pw}
}

func (b *blockingLog) Read(p []byte) (int, error) { return b.pr.Read(p) }

func (b *blockingLog) Close() error {
	b.closed.Store(true)
	b.pw.Close()
	return b.pr.Close()
}

func logsFixture() (*fakeBackend, *blockingLog) {
	fb := fixture()
	bl := newBlockingLog()
	fb.logs = bl
	return fb, bl
}

func TestOpenProjectLogs(t *testing.T) {
	for _, k := range []string{"l", "f7"} {
		fb, _ := logsFixture()
		m := keys(newTestModel(t, fb), k)
		if want := []docker.Target{projectTarget("alpha")}; !reflect.DeepEqual(fb.logTargets, want) {
			t.Errorf("%s: targets %+v", k, fb.logTargets)
		}
		assertContains(t, plain(m), "Logs — alpha", "following")
		m = send(m, logLinesMsg{session: m.logs.session, lines: []string{"one", "two"}})
		assertContains(t, plain(m), "one", "two", "Back", "Follow")
	}
}

func TestOpenContainerLogs(t *testing.T) {
	fb, _ := logsFixture()
	keys(newTestModel(t, fb), "right", "down", "down", "l")
	want := []docker.Target{{Kind: docker.KindContainer, ID: cid("a1"), Name: "alpha-web-1"}}
	if !reflect.DeepEqual(fb.logTargets, want) {
		t.Errorf("targets %+v", fb.logTargets)
	}
}

func TestLogsNeedATarget(t *testing.T) {
	fb, _ := logsFixture()
	m := keys(newTestModel(t, fb), "end", "l")
	if m.mode != modeMain {
		t.Errorf("mode: %d", m.mode)
	}
	assertContains(t, plain(m), "Logs need a project or a container")
}

func TestLogsOpenFailure(t *testing.T) {
	fb := fixture()
	fb.logsErr = errors.New("no such project")
	m := keys(newTestModel(t, fb), "l")
	if m.mode != modeMain {
		t.Errorf("mode: %d", m.mode)
	}
	assertContains(t, plain(m), "Refresh failed: no such project")
}

func TestLeavingLogsClosesTheStream(t *testing.T) {
	for _, k := range []string{"esc", "q"} {
		fb, bl := logsFixture()
		m := keys(newTestModel(t, fb), "l")
		session := m.logs.session
		m, cmd := step(m, key(k))
		if m.mode != modeMain || !strings.Contains(plain(m), "▶ alpha") {
			t.Errorf("%s: not back to the list", k)
		}
		if !bl.closed.Load() {
			t.Errorf("%s: stream not closed", k)
		}
		if cmd != nil {
			t.Errorf("%s: command returned (quit?)", k)
		}
		m = send(m, logLinesMsg{session: session, lines: []string{"late"}})
		if len(m.logs.lines) != 0 && strings.Contains(strings.Join(m.logs.lines, ""), "late") {
			t.Errorf("%s: old session accepted", k)
		}
	}
}

func TestLogBufferIsCapped(t *testing.T) {
	fb, _ := logsFixture()
	m := keys(newTestModel(t, fb), "l")
	lines := make([]string, 5200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	m = send(m, logLinesMsg{session: m.logs.session, lines: lines})
	if len(m.logs.lines) != 5000 || m.logs.lines[0] != "line 200" {
		t.Errorf("len %d, first %q", len(m.logs.lines), m.logs.lines[0])
	}
}

func TestFollowPausesAndResumes(t *testing.T) {
	fb, _ := logsFixture()
	m := keys(newTestModel(t, fb), "l")
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i)
	}
	m = send(m, logLinesMsg{session: m.logs.session, lines: lines})
	assertContains(t, plain(m), "line 099", "following")
	m = keys(m, "up")
	assertContains(t, plain(m), "paused")
	before := plain(m)
	m = send(m, logLinesMsg{session: m.logs.session, lines: []string{"line 100"}})
	if plain(m) != before {
		t.Error("new lines moved a paused view")
	}
	m = keys(m, "f")
	assertContains(t, plain(m), "following", "line 100")
	m = keys(m, "up", "end")
	assertContains(t, plain(m), "following")
}

func TestLogLinesAreSanitised(t *testing.T) {
	fb, _ := logsFixture()
	m := keys(newTestModel(t, fb), "l")
	m = send(m, logLinesMsg{session: m.logs.session, lines: []string{"\x1b[31mred\x1b[0m\tdone\r"}})
	if got := m.logs.lines[0]; got != "red    done" {
		t.Errorf("%q", got)
	}
}

func TestLogStreamEnd(t *testing.T) {
	fb, _ := logsFixture()
	m := keys(newTestModel(t, fb), "l")
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %03d", i)
	}
	m = send(m, logLinesMsg{session: m.logs.session, lines: lines}, logEndMsg{session: m.logs.session})
	assertContains(t, plain(m), "Log stream ended")
	if m.mode != modeLogs {
		t.Fatal("view closed")
	}
	m = keys(m, "home")
	assertContains(t, plain(m), "line 000")
}

func TestLogsStreamThroughListener(t *testing.T) {
	fb := fixture()
	fb.logs = io.NopCloser(strings.NewReader("first\nsecond\n"))
	m, cmd := step(newTestModel(t, fb), key("l"))
	for i := 0; i < 10 && cmd != nil; i++ {
		var next []tea.Cmd
		for _, msg := range runCmd(t, cmd) {
			var c tea.Cmd
			m, c = step(m, msg)
			next = append(next, c)
		}
		cmd = tea.Batch(next...)
	}
	assertContains(t, plain(m), "first", "second", "Log stream ended")
}

func TestReadLinesHandlesHugeLines(t *testing.T) {
	const big = 3 << 20
	input := "a\n" + strings.Repeat("x", big) + "\ntail\n"
	var got []string
	readLines(bytes.NewReader([]byte(input)), func(b []string) { got = append(got, b...) })
	if len(got) < 5 || got[0] != "a" || got[len(got)-1] != "tail" {
		t.Fatalf("pieces: %d, first %q", len(got), got[0])
	}
	total := 0
	for _, p := range got[1 : len(got)-1] {
		if strings.Trim(p, "x") != "" {
			t.Fatalf("unexpected piece")
		}
		total += len(p)
	}
	if total != big || len(got)-2 < 3 {
		t.Errorf("x pieces: %d, total %d", len(got)-2, total)
	}
}

func TestShell(t *testing.T) {
	for _, k := range []string{"e", "f8"} {
		fb := fixture()
		m := keys(newTestModel(t, fb), "right", "down", "down")
		_, cmd := step(m, key(k))
		if cmd == nil || !reflect.DeepEqual(fb.shells, []string{cid("a1")}) {
			t.Errorf("%s: cmd %v shells %q", k, cmd != nil, fb.shells)
		}
	}
	fb := fixture()
	m := keys(newTestModel(t, fb), "down", "right", "down", "down")
	if r, _ := m.current(); r.container == nil || r.container.Name != "beta-worker-1" {
		t.Fatalf("cursor: %+v", r.key)
	}
	m, cmd := step(m, key("e"))
	if cmd != nil || len(fb.shells) != 0 {
		t.Error("shell on an exited container")
	}
	assertContains(t, plain(m), "A shell needs a running container")
	m, cmd = step(keys(m, "home"), key("e"))
	if cmd != nil || len(fb.shells) != 0 {
		t.Error("shell on a project row")
	}
	m = send(m, shellDoneMsg{err: errors.New("exit status 126")})
	assertContains(t, plain(m), "Shell failed: exit status 126")
	_, cmd = step(m, shellDoneMsg{})
	if !hasMsg[dataMsg](runCmd(t, cmd)) {
		t.Error("no refresh after the shell")
	}
}
