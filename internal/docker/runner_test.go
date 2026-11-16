package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerOutput(t *testing.T) {
	out, err := ExecRunner{Bin: "sh"}.Output(context.Background(), "-c", "echo hi")
	if err != nil || string(out) != "hi\n" {
		t.Errorf("%q %v", out, err)
	}
}

func TestExecRunnerOutputError(t *testing.T) {
	_, err := ExecRunner{Bin: "sh"}.Output(context.Background(), "-c", "echo first >&2; echo boom >&2; exit 3")
	var cmdErr *CmdError
	if !errors.As(err, &cmdErr) || err.Error() != "boom" {
		t.Errorf("%v", err)
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	_, err := ExecRunner{Bin: "definitely-not-a-binary"}.Output(context.Background())
	if err == nil || err.Error() == "" {
		t.Errorf("%v", err)
	}
}

func TestExecRunnerStreamMergesAndCloses(t *testing.T) {
	r, err := ExecRunner{Bin: "sh"}.Stream(context.Background(), "-c", "echo out; echo err >&2")
	if err != nil {
		t.Fatal(err)
	}
	all, _ := io.ReadAll(r)
	r.Close()
	if !strings.Contains(string(all), "out") || !strings.Contains(string(all), "err") {
		t.Errorf("merged: %q", all)
	}

	r2, err := ExecRunner{Bin: "sleep"}.Stream(context.Background(), "30")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { r2.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Close did not return within 2 seconds")
	}
}

func TestExecRunnerDefaultsToDocker(t *testing.T) {
	cmd := ExecRunner{}.Command("ps")
	if len(cmd.Args) != 2 || cmd.Args[0] != "docker" || cmd.Args[1] != "ps" {
		t.Errorf("%q", cmd.Args)
	}
}
