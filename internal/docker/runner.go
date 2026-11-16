package docker

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner runs the docker command line. Every Docker call goes through it,
// which keeps the rest of the program testable with recorded output.
type Runner interface {
	// Output runs `docker args…` and returns its standard output.
	Output(ctx context.Context, args ...string) ([]byte, error)
	// Stream starts `docker args…` and returns its standard output and error merged.
	// Closing the reader kills the process.
	Stream(ctx context.Context, args ...string) (io.ReadCloser, error)
	// Command builds an interactive command the caller will attach to the terminal.
	Command(args ...string) *exec.Cmd
}

// ExecRunner is the Runner that starts real processes.
type ExecRunner struct {
	Bin string // defaults to "docker" when empty
}

func (r ExecRunner) bin() string {
	if r.Bin == "" {
		return "docker"
	}
	return r.Bin
}

// CmdError is a failed command, carrying what it printed on standard error.
type CmdError struct {
	Args   []string
	Stderr string
	Err    error
}

// Error returns the last non-empty line of the command's standard error,
// which is where Docker puts its own message, or the underlying error.
func (e *CmdError) Error() string {
	lines := strings.Split(strings.TrimSpace(e.Stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return e.Err.Error()
}

func (e *CmdError) Unwrap() error { return e.Err }

// Output runs the command and returns its standard output.
func (r ExecRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, &CmdError{Args: args, Stderr: stderr.String(), Err: err}
	}
	return out, nil
}

// Stream starts the command with standard output and error sent to one pipe.
func (r ExecRunner) Stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, r.bin(), args...)
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		cancel()
		pr.Close()
		pw.Close()
		return nil, &CmdError{Args: args, Err: err}
	}
	// The child holds its own copy of the write end; closing ours lets the
	// reader see end of file when the process exits.
	pw.Close()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	return &streamReader{File: pr, cancel: cancel, done: done}, nil
}

// Command builds an interactive command; the caller attaches it to the terminal.
func (r ExecRunner) Command(args ...string) *exec.Cmd {
	return exec.Command(r.bin(), args...)
}

type streamReader struct {
	*os.File
	cancel context.CancelFunc
	done   chan struct{}
}

// Close kills the process, closes the pipe and waits for the process to end.
func (s *streamReader) Close() error {
	s.cancel()
	err := s.File.Close()
	<-s.done
	return err
}
