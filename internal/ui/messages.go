package ui

import (
	"time"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// dataMsg carries the result of a refresh. On error the lists are empty and
// the previous data is kept.
type dataMsg struct {
	containers []docker.Container
	volumes    []docker.Volume
	images     []docker.Image
	err        error
}

// statMsg is one usage sample from the stats stream.
type statMsg docker.Stat

// eventMsg reports a Docker event worth a refresh.
type eventMsg struct{}

// refreshNowMsg fires once the event debounce delay has passed.
type refreshNowMsg struct{}

// tickMsg fires every second.
type tickMsg time.Time
