package docker

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const psKafka = `{"id":"e61bf90d0c117237fa267706aaf81f1e8e75d442cc6f92e1bd2a6cfbae963a22","name":"kafka-connect","image":"redpanda/connect:latest","state":"exited","status":"Exited (255) 2 weeks ago","ports":"4195/tcp, 0.0.0.0:8083->8083/tcp, [::]:8083->8083/tcp","mounts":"/home/me/My Projects/kafka,kafka_data","project":"kafka","service":"connect","workdir":"/home/me/My Projects/kafka","files":"/home/me/My Projects/kafka/compose.yml,/home/me/My Projects/kafka/compose.override.yml"}`
const psLonely = `{"id":"a373cbbaea609087074a2f66232b82467c8c9e7aab181d0425c5252104405f23","name":"lonely","image":"busybox","state":"running","status":"Up 2 days (healthy)","ports":"","mounts":"","project":"","service":"","workdir":"","files":""}`

func TestParseContainersKeepsCommasAndSpaces(t *testing.T) {
	cs, err := ParseContainers([]byte(psKafka + "\n" + psLonely + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d containers", len(cs))
	}
	k := cs[0]
	if k.ID != "e61bf90d0c117237fa267706aaf81f1e8e75d442cc6f92e1bd2a6cfbae963a22" || k.Name != "kafka-connect" || k.Image != "redpanda/connect:latest" {
		t.Errorf("identity: %+v", k)
	}
	if k.Project != "kafka" || k.Service != "connect" || k.State != "exited" || k.ExitCode != 255 || k.Health != "" {
		t.Errorf("state: %+v", k)
	}
	if k.Status != "Exited (255) 2 weeks ago" {
		t.Errorf("status: %q", k.Status)
	}
	if k.WorkingDir != "/home/me/My Projects/kafka" {
		t.Errorf("workdir: %q", k.WorkingDir)
	}
	wantFiles := []string{"/home/me/My Projects/kafka/compose.yml", "/home/me/My Projects/kafka/compose.override.yml"}
	if !reflect.DeepEqual(k.ConfigFiles, wantFiles) {
		t.Errorf("files: %q", k.ConfigFiles)
	}
	if k.Ports != "8083→8083" {
		t.Errorf("ports: %q", k.Ports)
	}
	if !reflect.DeepEqual(k.Mounts, []string{"/home/me/My Projects/kafka", "kafka_data"}) {
		t.Errorf("mounts: %q", k.Mounts)
	}
	l := cs[1]
	if l.Project != "" || l.Health != "healthy" || l.ExitCode != 0 || len(l.ConfigFiles) != 0 || len(l.Mounts) != 0 {
		t.Errorf("lonely: %+v", l)
	}
}

func TestParseContainersSkipsNoise(t *testing.T) {
	cs, err := ParseContainers([]byte("WARNING: something\n\n" + psLonely + "\n"))
	if err != nil || len(cs) != 1 {
		t.Errorf("noise: %d %v", len(cs), err)
	}
	if _, err := ParseContainers([]byte(`{"id": broken`)); err == nil {
		t.Error("broken JSON: no error")
	}
	cs, err = ParseContainers([]byte(""))
	if err != nil || len(cs) != 0 {
		t.Errorf("empty: %d %v", len(cs), err)
	}
}

func TestParseStatus(t *testing.T) {
	cases := []struct {
		status string
		health string
		code   int
	}{
		{"Up 2 days (healthy)", "healthy", 0},
		{"Up 5 seconds (health: starting)", "starting", 0},
		{"Up 3 hours (unhealthy)", "unhealthy", 0},
		{"Up 2 hours (Paused)", "", 0},
		{"Exited (137) 3 minutes ago", "", 137},
		{"Created", "", 0},
	}
	for _, c := range cases {
		h, code := parseStatus(c.status)
		if h != c.health || code != c.code {
			t.Errorf("%q: got %q %d", c.status, h, code)
		}
	}
}

func TestCompactPorts(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"4195/tcp": "",
		"0.0.0.0:8083->8083/tcp, [::]:8083->8083/tcp, 4195/tcp": "8083→8083",
		"0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp":              "80→80, 443→443",
		"127.0.0.1:5432->5432/tcp":                              "5432→5432",
		"0.0.0.0:8000-8002->8000-8002/tcp":                      "8000-8002→8000-8002",
		"0.0.0.0:53->53/udp":                                    "53→53/udp",
	}
	for in, want := range cases {
		if got := CompactPorts(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestParseSize(t *testing.T) {
	ok := map[string]int64{
		"0B": 0, "2kB": 2000, "30.3MB": 30300000, "1.385GB": 1385000000,
		"512MiB": 536870912, "1.5GiB": 1610612736, "19.89kB": 19890,
	}
	for in, want := range ok {
		got, valid := ParseSize(in)
		if !valid || got != want {
			t.Errorf("%q: got %d %v want %d", in, got, valid, want)
		}
	}
	for _, in := range []string{"N/A", "--", "", "abc"} {
		if _, valid := ParseSize(in); valid {
			t.Errorf("%q: valid", in)
		}
	}
}

func TestParseStatLine(t *testing.T) {
	now := time.Unix(1000, 0)
	body := `{"BlockIO":"13.1MB / 0B","CPUPerc":"0.28%","Container":"ad367e148cdcdf89","ID":"ad367e148cdc","MemPerc":"0.81%","MemUsage":"512MiB / 15.46GiB","Name":"x","NetIO":"1MB / 2MB","PIDs":"3"}`
	line := "\x1b[H" + body + "\x1b[K"
	s, ok := ParseStatLine(line, now)
	want := Stat{ID: "ad367e148cdc", CPU: 0.28, Mem: 536870912, Seen: now}
	if !ok || s != want {
		t.Errorf("got %+v %v", s, ok)
	}
	dashes := strings.NewReplacer(`"CPUPerc":"0.28%"`, `"CPUPerc":"--"`, `"MemUsage":"512MiB / 15.46GiB"`, `"MemUsage":"-- / --"`).Replace(line)
	if _, ok := ParseStatLine(dashes, now); ok {
		t.Error("dashes accepted")
	}
	for _, l := range []string{"\x1b[2J\x1b[H", "", "not json"} {
		if _, ok := ParseStatLine(l, now); ok {
			t.Errorf("%q accepted", l)
		}
	}
}

func TestIsRelevantEvent(t *testing.T) {
	cases := map[string]bool{
		`{"Type":"container","Action":"die"}`:                       true,
		`{"Type":"container","Action":"health_status: healthy"}`:    true,
		`{"Type":"container","Action":"exec_start: node check.js"}`: false,
		`{"Type":"container","Action":"exec_create: sh"}`:           false,
		`{"Type":"container","Action":"exec_die"}`:                  false,
		`{"Type":"volume","Action":"create"}`:                       true,
		`{"Type":"image","Action":"delete"}`:                        true,
		`{"Type":"network","Action":"connect"}`:                     false,
		"garbage":                                                   false,
	}
	for in, want := range cases {
		if got := IsRelevantEvent(in); got != want {
			t.Errorf("%s: got %v", in, got)
		}
	}
}

func TestParseVolumes(t *testing.T) {
	hex := strings.Repeat("ab", 32)
	in := `{"name":"bacv3_db","driver":"local","project":"bacv3","labels":"com.docker.compose.project=bacv3,com.docker.compose.volume=db"}
{"name":"` + hex + `","driver":"local","project":"","labels":"com.docker.volume.anonymous="}
{"name":"` + hex + `","driver":"local","project":"","labels":""}
{"name":"cafe","driver":"local","project":"","labels":""}
`
	vs, err := ParseVolumes([]byte(in))
	if err != nil || len(vs) != 4 {
		t.Fatalf("%d %v", len(vs), err)
	}
	if vs[0] != (Volume{Name: "bacv3_db", Driver: "local", Project: "bacv3"}) {
		t.Errorf("named: %+v", vs[0])
	}
	if !vs[1].Anonymous || !vs[2].Anonymous || vs[3].Anonymous {
		t.Errorf("anonymous: %v %v %v", vs[1].Anonymous, vs[2].Anonymous, vs[3].Anonymous)
	}
}

func TestParseImages(t *testing.T) {
	in := `{"id":"sha256:b49369bbdf90aa","repository":"me/proxy","tag":"1.1.1","size":"30.3MB","created":"2 days ago"}
{"id":"sha256:cc","repository":"<none>","tag":"<none>","size":"1kB","created":"3 weeks ago"}
`
	is, err := ParseImages([]byte(in))
	if err != nil || len(is) != 2 {
		t.Fatalf("%d %v", len(is), err)
	}
	want := Image{ID: "sha256:b49369bbdf90aa", Repository: "me/proxy", Tag: "1.1.1", Created: "2 days ago", Size: 30300000}
	if is[0] != want {
		t.Errorf("got %+v", is[0])
	}
	if is[1].Repository != "<none>" || is[1].Tag != "<none>" {
		t.Errorf("none: %+v", is[1])
	}
}

func TestParseInfo(t *testing.T) {
	info, err := ParseInfo([]byte(`{"version":"29.7.2","ncpu":20,"mem":16597549056}` + "\n"))
	if err != nil || info != (Info{"29.7.2", 20, 16597549056}) {
		t.Errorf("%+v %v", info, err)
	}
}

func TestParseDiskUsage(t *testing.T) {
	in := `{"Images":[{"ID":"sha256:aaa","Containers":"2","Size":"30.3MB"}],"Containers":[],"Volumes":[{"Name":"bacv3_db","Links":"1","Size":"1.385GB"},{"Name":"tmp","Links":"0","Size":"N/A"}],"BuildCache":[]}`
	du, err := ParseDiskUsage([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(du.VolumeSize, map[string]int64{"bacv3_db": 1385000000}) {
		t.Errorf("sizes: %v", du.VolumeSize)
	}
	if !reflect.DeepEqual(du.VolumeLinks, map[string]int{"bacv3_db": 1, "tmp": 0}) {
		t.Errorf("links: %v", du.VolumeLinks)
	}
	if !reflect.DeepEqual(du.ImageContainers, map[string]int{"sha256:aaa": 2}) {
		t.Errorf("images: %v", du.ImageContainers)
	}
}
