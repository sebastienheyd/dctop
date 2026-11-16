// Package docker drives the docker command line and turns its output into
// typed values.
package docker

import "time"

// Container is one row of `docker ps -a`.
type Container struct {
	ID, Name, Image string
	State           string // created, running, paused, restarting, exited, removing, dead
	Status          string // Docker's own text, e.g. "Up 2 days (healthy)"
	Health          string // "", "healthy", "unhealthy", "starting"
	ExitCode        int    // parsed from Status when State is "exited", otherwise 0
	Ports           string // CompactPorts of the raw ports
	Mounts          []string
	Project         string // "" when the container has no Compose label
	Service         string
	WorkingDir      string
	ConfigFiles     []string
}

// Volume is one row of `docker volume ls`.
type Volume struct {
	Name, Driver, Project string
	Anonymous             bool
}

// Image is one row of `docker image ls`.
type Image struct {
	ID, Repository, Tag, Created string // ID is "sha256:…" in full
	Size                         int64
}

// Info holds the daemon facts read once at start-up.
type Info struct {
	ServerVersion string
	NCPU          int
	MemTotal      int64
}

// Stat is one usage sample from `docker stats`.
type Stat struct {
	ID   string  // 12-character short container ID
	CPU  float64 // percent, 100 = one core
	Mem  int64   // bytes
	Seen time.Time
}

// DiskUsage holds the figures of `docker system df -v`.
type DiskUsage struct {
	VolumeSize      map[string]int64 // by volume name
	VolumeLinks     map[string]int   // by volume name
	ImageContainers map[string]int   // by full image ID
}
