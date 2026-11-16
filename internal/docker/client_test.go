package docker

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestClientCommands(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		out  string
		call func(c *Client) (any, error)
		args []string
		want any
	}{
		{"Info", `{"version":"29.7.2","ncpu":20,"mem":100}`,
			func(c *Client) (any, error) { return c.Info(ctx) },
			[]string{"info", "--format", InfoFormat},
			Info{"29.7.2", 20, 100}},
		{"Containers", psLonely,
			func(c *Client) (any, error) {
				cs, err := c.Containers(ctx)
				if len(cs) != 1 {
					return nil, err
				}
				return cs[0].Name, err
			},
			[]string{"ps", "-a", "--no-trunc", "--format", ContainerFormat},
			"lonely"},
		{"Volumes", `{"name":"v","driver":"local","project":"p","labels":""}`,
			func(c *Client) (any, error) { return c.Volumes(ctx) },
			[]string{"volume", "ls", "--format", VolumeFormat},
			[]Volume{{Name: "v", Driver: "local", Project: "p"}}},
		{"Images", `{"id":"sha256:a","repository":"r","tag":"t","size":"1kB","created":"now"}`,
			func(c *Client) (any, error) { return c.Images(ctx) },
			[]string{"image", "ls", "--no-trunc", "--format", ImageFormat},
			[]Image{{ID: "sha256:a", Repository: "r", Tag: "t", Created: "now", Size: 1000}}},
		{"DiskUsage", `{"Images":[{"ID":"sha256:a","Containers":"1"}],"Volumes":[{"Name":"v","Links":"2","Size":"1kB"}]}`,
			func(c *Client) (any, error) { return c.DiskUsage(ctx) },
			[]string{"system", "df", "-v", "--format", "{{json .}}"},
			DiskUsage{VolumeSize: map[string]int64{"v": 1000}, VolumeLinks: map[string]int{"v": 2}, ImageContainers: map[string]int{"sha256:a": 1}}},
	}
	for _, c := range cases {
		fr := &fakeRunner{outputs: []fakeOutput{{out: []byte(c.out + "\n")}}}
		got, err := c.call(NewClient(fr))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
		if calls := fr.Calls(); len(calls) != 1 || !reflect.DeepEqual(calls[0], c.args) {
			t.Errorf("%s: args %q", c.name, calls)
		}
	}
}

func TestClientPropagatesErrors(t *testing.T) {
	down := errors.New("daemon down")
	fr := &fakeRunner{outputs: []fakeOutput{{err: down}}}
	if _, err := NewClient(fr).Containers(context.Background()); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}
