package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Go templates passed to --format. Each one emits one JSON object per line.
// Labels are read one by one: the default {{json .}} output joins them with
// commas, and label values (config_files) may contain commas themselves.
// .HealthStatus is avoided because older Docker versions lack it.
const (
	ContainerFormat = `{"id":{{json .ID}},"name":{{json .Names}},"image":{{json .Image}},"state":{{json .State}},"status":{{json .Status}},"ports":{{json .Ports}},"mounts":{{json .Mounts}},"project":{{json (.Label "com.docker.compose.project")}},"service":{{json (.Label "com.docker.compose.service")}},"workdir":{{json (.Label "com.docker.compose.project.working_dir")}},"files":{{json (.Label "com.docker.compose.project.config_files")}}}`
	VolumeFormat    = `{"name":{{json .Name}},"driver":{{json .Driver}},"project":{{json (.Label "com.docker.compose.project")}},"labels":{{json .Labels}}}`
	ImageFormat     = `{"id":{{json .ID}},"repository":{{json .Repository}},"tag":{{json .Tag}},"size":{{json .Size}},"created":{{json .CreatedSince}}}`
	InfoFormat      = `{"version":{{json .ServerVersion}},"ncpu":{{json .NCPU}},"mem":{{json .MemTotal}}}`
)

// parseLines decodes every line starting with "{" into a T. Other lines
// (warnings, blank lines) are skipped; a line that starts with "{" but does
// not decode is an error.
func parseLines[T any](out []byte) ([]T, error) {
	var items []T
	for _, line := range bytes.Split(out, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("cannot decode docker output %q: %w", line, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// clean removes escape sequences and control characters from a text Docker
// printed. Labels are inherited from images, so a pulled image can put
// anything in them; nothing that could move the cursor, add a line or talk
// to the terminal may reach the screen.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

// splitList splits a comma-separated value, dropping empty entries.
func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type rawContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	Mounts  string `json:"mounts"`
	Project string `json:"project"`
	Service string `json:"service"`
	Workdir string `json:"workdir"`
	Files   string `json:"files"`
}

// ParseContainers parses the output of `docker ps --format ContainerFormat`.
func ParseContainers(out []byte) ([]Container, error) {
	raws, err := parseLines[rawContainer](out)
	if err != nil {
		return nil, err
	}
	cs := make([]Container, 0, len(raws))
	for _, r := range raws {
		health, code := parseStatus(r.Status)
		if r.State != "exited" {
			code = 0
		}
		cs = append(cs, Container{
			ID:          r.ID,
			Name:        clean(r.Name),
			Image:       clean(r.Image),
			State:       clean(r.State),
			Status:      clean(r.Status),
			Health:      health,
			ExitCode:    code,
			Ports:       CompactPorts(clean(r.Ports)),
			Mounts:      splitList(clean(r.Mounts)),
			Project:     clean(r.Project),
			Service:     clean(r.Service),
			WorkingDir:  clean(r.Workdir),
			ConfigFiles: splitList(clean(r.Files)),
		})
	}
	return cs, nil
}

var exitRe = regexp.MustCompile(`^Exited \((-?\d+)\)`)

// parseStatus reads the health and the exit code from Docker's status text,
// e.g. "Up 2 days (healthy)" or "Exited (137) 3 minutes ago".
func parseStatus(status string) (health string, exitCode int) {
	switch {
	case strings.Contains(status, "(unhealthy)"):
		health = "unhealthy"
	case strings.Contains(status, "(healthy)"):
		health = "healthy"
	case strings.Contains(status, "(health: starting)"):
		health = "starting"
	}
	if m := exitRe.FindStringSubmatch(status); m != nil {
		exitCode, _ = strconv.Atoi(m[1])
	}
	return health, exitCode
}

// CompactPorts keeps the published ports of Docker's port text, as
// "host→container". The /tcp suffix is dropped, other protocols are kept,
// and duplicates (IPv4 and IPv6 bindings) are removed.
func CompactPorts(s string) string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		host, cont, ok := strings.Cut(p, "->")
		if !ok {
			continue
		}
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[i+1:]
		}
		cont = strings.TrimSuffix(cont, "/tcp")
		entry := host + "→" + cont
		if !seen[entry] {
			seen[entry] = true
			out = append(out, entry)
		}
	}
	return strings.Join(out, ", ")
}

var sizeUnits = map[string]float64{
	"B":   1,
	"kB":  1e3,
	"KB":  1e3,
	"MB":  1e6,
	"GB":  1e9,
	"TB":  1e12,
	"KiB": 1 << 10,
	"MiB": 1 << 20,
	"GiB": 1 << 30,
	"TiB": 1 << 40,
}

// ParseSize reads a size such as "30.3MB" or "512MiB" as bytes, rounded to
// the nearest byte. ok is false for "N/A", "--" and anything unreadable.
func ParseSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	mult, ok := sizeUnits[strings.TrimSpace(s[i:])]
	if !ok {
		return 0, false
	}
	return int64(math.Round(n * mult)), true
}

type rawStat struct {
	ID       string `json:"ID"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
}

// ParseStatLine reads one line of `docker stats --format '{{json .}}'`.
// The stream wraps every line in cursor sequences, which are stripped first.
// ok is false for anything that is not a complete sample, including the
// "--" values Docker prints for a container that just stopped.
func ParseStatLine(line string, now time.Time) (Stat, bool) {
	line = strings.TrimSpace(ansi.Strip(line))
	if !strings.HasPrefix(line, "{") {
		return Stat{}, false
	}
	var r rawStat
	if err := json.Unmarshal([]byte(line), &r); err != nil || r.ID == "" {
		return Stat{}, false
	}
	cpu, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(r.CPUPerc), "%"), 64)
	if err != nil {
		return Stat{}, false
	}
	used, _, _ := strings.Cut(r.MemUsage, " / ")
	mem, ok := ParseSize(used)
	if !ok {
		return Stat{}, false
	}
	id := r.ID
	if len(id) > 12 {
		id = id[:12]
	}
	return Stat{ID: id, CPU: cpu, Mem: mem, Seen: now}, true
}

// IsRelevantEvent tells whether a line of `docker events --format '{{json .}}'`
// should trigger a refresh. Exec events, emitted by every health check, and
// network events are ignored.
func IsRelevantEvent(line string) bool {
	var e struct {
		Type   string `json:"Type"`
		Action string `json:"Action"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &e); err != nil {
		return false
	}
	switch e.Type {
	case "container":
		return !strings.HasPrefix(e.Action, "exec_")
	case "volume", "image":
		return true
	}
	return false
}

type rawVolume struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Project string `json:"project"`
	Labels  string `json:"labels"`
}

var anonymousNameRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseVolumes parses the output of `docker volume ls --format VolumeFormat`.
func ParseVolumes(out []byte) ([]Volume, error) {
	raws, err := parseLines[rawVolume](out)
	if err != nil {
		return nil, err
	}
	vs := make([]Volume, 0, len(raws))
	for _, r := range raws {
		vs = append(vs, Volume{
			Name:      clean(r.Name),
			Driver:    clean(r.Driver),
			Project:   clean(r.Project),
			Anonymous: strings.Contains(r.Labels, "com.docker.volume.anonymous") || anonymousNameRe.MatchString(r.Name),
		})
	}
	return vs, nil
}

type rawImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

// ParseImages parses the output of `docker image ls --format ImageFormat`.
func ParseImages(out []byte) ([]Image, error) {
	raws, err := parseLines[rawImage](out)
	if err != nil {
		return nil, err
	}
	is := make([]Image, 0, len(raws))
	for _, r := range raws {
		size, _ := ParseSize(r.Size)
		is = append(is, Image{ID: r.ID, Repository: clean(r.Repository), Tag: clean(r.Tag), Created: clean(r.Created), Size: size})
	}
	return is, nil
}

// ParseInfo parses the output of `docker info --format InfoFormat`.
func ParseInfo(out []byte) (Info, error) {
	raws, err := parseLines[struct {
		Version string `json:"version"`
		NCPU    int    `json:"ncpu"`
		Mem     int64  `json:"mem"`
	}](out)
	if err != nil {
		return Info{}, err
	}
	if len(raws) == 0 {
		return Info{}, fmt.Errorf("no output from docker info")
	}
	r := raws[0]
	return Info{ServerVersion: r.Version, NCPU: r.NCPU, MemTotal: r.Mem}, nil
}

// flexInt decodes a count that Docker prints either as a number or as a string.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" || s == "N/A" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// ParseDiskUsage parses the output of `docker system df -v --format '{{json .}}'`.
func ParseDiskUsage(out []byte) (DiskUsage, error) {
	type raw struct {
		Images []struct {
			ID         string  `json:"ID"`
			Containers flexInt `json:"Containers"`
		} `json:"Images"`
		Volumes []struct {
			Name  string  `json:"Name"`
			Links flexInt `json:"Links"`
			Size  string  `json:"Size"`
		} `json:"Volumes"`
	}
	raws, err := parseLines[raw](out)
	if err != nil {
		return DiskUsage{}, err
	}
	du := DiskUsage{
		VolumeSize:      map[string]int64{},
		VolumeLinks:     map[string]int{},
		ImageContainers: map[string]int{},
	}
	for _, r := range raws {
		for _, img := range r.Images {
			du.ImageContainers[img.ID] = int(img.Containers)
		}
		for _, v := range r.Volumes {
			du.VolumeLinks[clean(v.Name)] = int(v.Links)
			if size, ok := ParseSize(v.Size); ok {
				du.VolumeSize[clean(v.Name)] = size
			}
		}
	}
	return du, nil
}
