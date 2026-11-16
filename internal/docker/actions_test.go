package docker

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestActionArgs(t *testing.T) {
	p := Target{Kind: KindProject, ID: "web", Name: "web"}
	c := Target{Kind: KindContainer, ID: "abc123", Name: "web-php-1"}
	v := Target{Kind: KindVolume, ID: "web_db", Name: "web_db"}
	i := Target{Kind: KindImage, ID: "sha256:aaa", Name: "nginx:latest"}
	cases := []struct {
		a    Action
		want []string
	}{
		{Action{OpStart, p}, []string{"compose", "-p", "web", "start"}},
		{Action{OpStop, p}, []string{"compose", "-p", "web", "stop"}},
		{Action{OpRestart, p}, []string{"compose", "-p", "web", "restart"}},
		{Action{OpRemove, p}, []string{"compose", "-p", "web", "down"}},
		{Action{OpStart, c}, []string{"start", "abc123"}},
		{Action{OpStop, c}, []string{"stop", "abc123"}},
		{Action{OpRestart, c}, []string{"restart", "abc123"}},
		{Action{OpRemove, c}, []string{"rm", "-f", "abc123"}},
		{Action{OpRemove, v}, []string{"volume", "rm", "web_db"}},
		{Action{OpRemove, i}, []string{"image", "rm", "sha256:aaa"}},
	}
	for _, tc := range cases {
		got, err := tc.a.Args()
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%+v: got %q %v", tc.a, got, err)
		}
	}
	unsupported := []Action{{OpStart, v}, {OpStop, i}, {OpRestart, v}}
	for _, op := range []Op{OpStart, OpStop, OpRestart, OpRemove} {
		unsupported = append(unsupported, Action{op, Target{Kind: KindProject, ID: ""}})
	}
	for _, a := range unsupported {
		if _, err := a.Args(); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%+v: %v", a, err)
		}
	}
}

func TestLogsArgs(t *testing.T) {
	got, err := LogsArgs(Target{Kind: KindProject, ID: "web"})
	if err != nil || !reflect.DeepEqual(got, []string{"compose", "-p", "web", "logs", "--follow", "--no-color", "--tail", "200"}) {
		t.Errorf("project: %q %v", got, err)
	}
	got, err = LogsArgs(Target{Kind: KindContainer, ID: "abc123"})
	if err != nil || !reflect.DeepEqual(got, []string{"logs", "--follow", "--tail", "200", "abc123"}) {
		t.Errorf("container: %q %v", got, err)
	}
	if _, err := LogsArgs(Target{Kind: KindVolume, ID: "v"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("volume: %v", err)
	}
}

func TestShellArgs(t *testing.T) {
	want := []string{"exec", "-it", "abc123", "sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}
	if got := ShellArgs("abc123"); !reflect.DeepEqual(got, want) {
		t.Errorf("%q", got)
	}
}

func TestShellCommandUsesRunner(t *testing.T) {
	fr := &fakeRunner{}
	cmd := NewClient(fr).ShellCommand("abc123")
	if cmd == nil || !reflect.DeepEqual(fr.Calls(), [][]string{ShellArgs("abc123")}) {
		t.Errorf("%q", fr.Calls())
	}
}

func TestLogsUsesStream(t *testing.T) {
	fr := &fakeRunner{streams: []fakeStream{{body: "line\n"}}}
	r, err := NewClient(fr).Logs(context.Background(), Target{Kind: KindContainer, ID: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	want, _ := LogsArgs(Target{Kind: KindContainer, ID: "abc123"})
	if !reflect.DeepEqual(fr.Calls(), [][]string{want}) {
		t.Errorf("%q", fr.Calls())
	}
	if _, err := NewClient(fr).Logs(context.Background(), Target{Kind: KindImage, ID: "x"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("image logs: %v", err)
	}
}

func TestRunUsesArgsAndReturnsError(t *testing.T) {
	boom := errors.New("boom")
	fr := &fakeRunner{outputs: []fakeOutput{{}, {err: boom}}}
	c := NewClient(fr)
	stop := Action{OpStop, Target{Kind: KindProject, ID: "web", Name: "web"}}
	if err := c.Run(context.Background(), stop); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fr.Calls(), [][]string{{"compose", "-p", "web", "stop"}}) {
		t.Errorf("%q", fr.Calls())
	}
	if err := c.Run(context.Background(), stop); err != boom {
		t.Errorf("runner error: %v", err)
	}
	if err := c.Run(context.Background(), Action{OpStart, Target{Kind: KindVolume, ID: "v"}}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unsupported: %v", err)
	}
	if n := len(fr.Calls()); n != 2 {
		t.Errorf("runner called %d times", n)
	}
}
