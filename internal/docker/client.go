package docker

import (
	"context"
	"time"
)

// Client reads from and acts on the Docker daemon through a Runner.
type Client struct {
	runner     Runner
	now        func() time.Time
	retryDelay time.Duration // pause before a finished stream is restarted
}

// NewClient returns a client that runs every command through r.
func NewClient(r Runner) *Client {
	return &Client{runner: r, now: time.Now, retryDelay: 2 * time.Second}
}

// Info reads the daemon version, CPU count and total memory.
func (c *Client) Info(ctx context.Context) (Info, error) {
	out, err := c.runner.Output(ctx, "info", "--format", InfoFormat)
	if err != nil {
		return Info{}, err
	}
	return ParseInfo(out)
}

// Containers lists every container, running or not.
func (c *Client) Containers(ctx context.Context) ([]Container, error) {
	out, err := c.runner.Output(ctx, "ps", "-a", "--no-trunc", "--format", ContainerFormat)
	if err != nil {
		return nil, err
	}
	return ParseContainers(out)
}

// Volumes lists every volume.
func (c *Client) Volumes(ctx context.Context) ([]Volume, error) {
	out, err := c.runner.Output(ctx, "volume", "ls", "--format", VolumeFormat)
	if err != nil {
		return nil, err
	}
	return ParseVolumes(out)
}

// Images lists every image.
func (c *Client) Images(ctx context.Context) ([]Image, error) {
	out, err := c.runner.Output(ctx, "image", "ls", "--no-trunc", "--format", ImageFormat)
	if err != nil {
		return nil, err
	}
	return ParseImages(out)
}

// DiskUsage reads volume sizes and usage counts. It takes seconds on a busy
// machine: callers run it in the background.
func (c *Client) DiskUsage(ctx context.Context) (DiskUsage, error) {
	out, err := c.runner.Output(ctx, "system", "df", "-v", "--format", "{{json .}}")
	if err != nil {
		return DiskUsage{}, err
	}
	return ParseDiskUsage(out)
}
