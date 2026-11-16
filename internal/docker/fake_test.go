package docker

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// fakeOutput is one queued answer of fakeRunner.Output.
type fakeOutput struct {
	out []byte
	err error
}

// fakeStream is one queued answer of fakeRunner.Stream. With block set, the
// reader delivers body then blocks until the context ends or it is closed.
type fakeStream struct {
	body  string
	block bool
	err   error
}

// fakeRunner records every call and replays queued answers. An empty queue
// gives empty output, or a stream that blocks until the context ends.
type fakeRunner struct {
	mu      sync.Mutex
	calls   [][]string
	outputs []fakeOutput
	streams []fakeStream
}

func (f *fakeRunner) record(args []string) {
	f.calls = append(f.calls, append([]string(nil), args...))
}

func (f *fakeRunner) Output(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(args)
	if len(f.outputs) == 0 {
		return nil, nil
	}
	o := f.outputs[0]
	f.outputs = f.outputs[1:]
	return o.out, o.err
}

func (f *fakeRunner) Stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(args)
	s := fakeStream{block: true}
	if len(f.streams) > 0 {
		s = f.streams[0]
		f.streams = f.streams[1:]
	}
	if s.err != nil {
		return nil, s.err
	}
	return newFakeReader(ctx, s.body, s.block), nil
}

func (f *fakeRunner) Command(args ...string) *exec.Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(args)
	return exec.Command("docker", args...)
}

func (f *fakeRunner) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.calls...)
}

type fakeReader struct {
	body   io.Reader
	block  bool
	ctx    context.Context
	closed chan struct{}
	once   sync.Once
}

func newFakeReader(ctx context.Context, body string, block bool) *fakeReader {
	return &fakeReader{body: strings.NewReader(body), block: block, ctx: ctx, closed: make(chan struct{})}
}

func (r *fakeReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 || err != io.EOF || !r.block {
		return n, err
	}
	select {
	case <-r.ctx.Done():
	case <-r.closed:
	}
	return 0, io.EOF
}

func (r *fakeReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}
