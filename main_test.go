package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/sebastienheyd/dctop/internal/ui"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--version"}, &out, &errOut, func(string) string { return "" })
	if code != 0 || out.String() != "dctop dev\n" || errOut.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--nope"}, &out, &errOut, func(string) string { return "" }); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

// stubRunner answers every Output call with out or err.
type stubRunner struct {
	out []byte
	err error
}

func (s stubRunner) Output(context.Context, ...string) ([]byte, error) { return s.out, s.err }
func (s stubRunner) Stream(context.Context, ...string) (io.ReadCloser, error) {
	return nil, errors.New("no stream")
}
func (s stubRunner) Command(args ...string) *exec.Cmd { return exec.Command("true") }

type harness struct {
	deps   deps
	calls  int
	runErr error
}

func newHarness(r stubRunner) *harness {
	h := &harness{}
	h.deps = deps{
		lookPath:   func(string) (string, error) { return "/usr/bin/docker", nil },
		runner:     r,
		runProgram: func(ui.Model) error { h.calls++; return h.runErr },
		home:       "/home/me",
	}
	return h
}

func envWith(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestHelpIsTranslated(t *testing.T) {
	fr := envWith(map[string]string{"DCTOP_LANG": "fr"})
	var out, errOut bytes.Buffer
	h := newHarness(stubRunner{})
	if code := runWith([]string{"--help"}, &out, &errOut, fr, h.deps); code != 0 || !strings.HasPrefix(out.String(), "Utilisation : dctop") {
		t.Errorf("fr: %d %q", code, out.String())
	}
	out.Reset()
	if code := runWith([]string{"--help", "--lang", "en"}, &out, &errOut, fr, h.deps); code != 0 || !strings.HasPrefix(out.String(), "Usage: dctop") {
		t.Errorf("en: %d %q", code, out.String())
	}
	out.Reset()
	if code := runWith([]string{"-h"}, &out, &errOut, envWith(nil), h.deps); code != 0 || !strings.HasPrefix(out.String(), "Usage: dctop") {
		t.Errorf("-h: %d %q", code, out.String())
	}
	if h.calls != 0 {
		t.Error("program started")
	}
}

func TestDockerMissing(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "docker command not found. Install Docker first.\n"},
		{[]string{"--lang", "fr"}, "Commande docker introuvable. Installez d'abord Docker.\n"},
	} {
		h := newHarness(stubRunner{})
		h.deps.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		var out, errOut bytes.Buffer
		code := runWith(c.args, &out, &errOut, envWith(nil), h.deps)
		if code != 1 || errOut.String() != c.want || h.calls != 0 {
			t.Errorf("%v: code=%d err=%q calls=%d", c.args, code, errOut.String(), h.calls)
		}
	}
}

func TestDaemonUnreachable(t *testing.T) {
	h := newHarness(stubRunner{err: errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock")})
	var out, errOut bytes.Buffer
	code := runWith(nil, &out, &errOut, envWith(nil), h.deps)
	want := "Cannot reach the Docker daemon: Cannot connect to the Docker daemon at unix:///var/run/docker.sock\n"
	if code != 1 || errOut.String() != want || h.calls != 0 {
		t.Errorf("code=%d err=%q calls=%d", code, errOut.String(), h.calls)
	}
}

func TestStartsTheProgram(t *testing.T) {
	info := stubRunner{out: []byte(`{"version":"29.7.2","ncpu":20,"mem":16597549056}` + "\n")}
	h := newHarness(info)
	var out, errOut bytes.Buffer
	if code := runWith(nil, &out, &errOut, envWith(nil), h.deps); code != 0 || h.calls != 1 {
		t.Errorf("code=%d calls=%d err=%q", code, h.calls, errOut.String())
	}
	h = newHarness(info)
	h.runErr = errors.New("terminal lost")
	errOut.Reset()
	if code := runWith(nil, &out, &errOut, envWith(nil), h.deps); code != 1 || !strings.Contains(errOut.String(), "terminal lost") {
		t.Errorf("code=%d err=%q", code, errOut.String())
	}
}
