package docker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const statA = `{"CPUPerc":"1.00%","ID":"aaaaaaaaaaaa","MemUsage":"1MiB / 2GiB"}`
const statB = `{"CPUPerc":"2.00%","ID":"bbbbbbbbbbbb","MemUsage":"2MiB / 2GiB"}`

func receiveStat(t *testing.T, ch <-chan Stat) Stat {
	t.Helper()
	select {
	case s := <-ch:
		return s
	case <-time.After(2 * time.Second):
		t.Fatal("no stat received")
		return Stat{}
	}
}

func waitReturn(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not return after cancel")
	}
}

func TestWatchStatsRestartsAfterStreamEnds(t *testing.T) {
	fr := &fakeRunner{streams: []fakeStream{
		{body: "\x1b[H" + statA + "\x1b[K\n"},
		{body: statB + "\n", block: true},
	}}
	c := NewClient(fr)
	c.retryDelay = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Stat, 4)
	done := make(chan struct{})
	go func() { c.WatchStats(ctx, ch); close(done) }()

	if s := receiveStat(t, ch); s.ID != "aaaaaaaaaaaa" {
		t.Errorf("first: %+v", s)
	}
	if s := receiveStat(t, ch); s.ID != "bbbbbbbbbbbb" {
		t.Errorf("second: %+v", s)
	}
	cancel()
	waitReturn(t, done)
	want := []string{"stats", "--format", "{{json .}}"}
	calls := fr.Calls()
	if len(calls) < 2 || !reflect.DeepEqual(calls[0], want) || !reflect.DeepEqual(calls[1], want) {
		t.Errorf("calls: %q", calls)
	}
}

func TestWatchStatsSurvivesStartFailure(t *testing.T) {
	fr := &fakeRunner{streams: []fakeStream{
		{err: errors.New("daemon down")},
		{body: statA + "\n", block: true},
	}}
	c := NewClient(fr)
	c.retryDelay = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan Stat, 4)
	done := make(chan struct{})
	go func() { c.WatchStats(ctx, ch); close(done) }()
	if s := receiveStat(t, ch); s.ID != "aaaaaaaaaaaa" {
		t.Errorf("%+v", s)
	}
	cancel()
	waitReturn(t, done)
}

func TestWatchEventsFiltersAndNeverBlocks(t *testing.T) {
	body := `{"Type":"container","Action":"exec_start: sh"}
{"Type":"container","Action":"die"}
{"Type":"container","Action":"die"}
{"Type":"container","Action":"die"}
{"Type":"container","Action":"die"}
`
	fr := &fakeRunner{streams: []fakeStream{{body: body, block: true}}}
	c := NewClient(fr)
	c.retryDelay = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() { c.WatchEvents(ctx, out); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for len(out) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // let the remaining lines be read
	cancel()
	waitReturn(t, done)
	if len(out) != 1 {
		t.Errorf("tokens: %d", len(out))
	}
	if calls := fr.Calls(); !reflect.DeepEqual(calls[0], []string{"events", "--format", "{{json .}}"}) {
		t.Errorf("calls: %q", calls)
	}
}

func TestWatchEventsIgnoresExecOnly(t *testing.T) {
	fr := &fakeRunner{streams: []fakeStream{{body: `{"Type":"container","Action":"exec_start: sh"}` + "\n", block: true}}}
	c := NewClient(fr)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() { c.WatchEvents(ctx, out); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	waitReturn(t, done)
	if len(out) != 0 {
		t.Errorf("exec event produced a token")
	}
}
