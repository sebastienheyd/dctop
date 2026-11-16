package docker

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

// Op is an action the user can launch.
type Op int

const (
	OpStart Op = iota
	OpStop
	OpRestart
	OpRemove
)

// TargetKind tells what an action applies to.
type TargetKind int

const (
	KindProject TargetKind = iota
	KindContainer
	KindVolume
	KindImage
)

// Target is the object of an action. ID is the project name, the full
// container ID, the volume name or the full image ID. Name is what the
// interface shows.
type Target struct {
	Kind TargetKind
	ID   string
	Name string
}

// Action is one operation on one target.
type Action struct {
	Op     Op
	Target Target
}

// ErrUnsupported is returned for a combination that has no docker command,
// such as starting a volume.
var ErrUnsupported = errors.New("unsupported action")

var projectVerbs = map[Op]string{OpStart: "start", OpStop: "stop", OpRestart: "restart", OpRemove: "down"}

// Args returns the docker arguments that perform the action. Removing a
// project runs `compose down`, which keeps its volumes.
func (a Action) Args() ([]string, error) {
	t := a.Target
	if t.ID == "" {
		return nil, ErrUnsupported
	}
	switch t.Kind {
	case KindProject:
		if verb, ok := projectVerbs[a.Op]; ok {
			return []string{"compose", "-p", t.ID, verb}, nil
		}
	case KindContainer:
		switch a.Op {
		case OpStart:
			return []string{"start", t.ID}, nil
		case OpStop:
			return []string{"stop", t.ID}, nil
		case OpRestart:
			return []string{"restart", t.ID}, nil
		case OpRemove:
			return []string{"rm", "-f", t.ID}, nil
		}
	case KindVolume:
		if a.Op == OpRemove {
			return []string{"volume", "rm", t.ID}, nil
		}
	case KindImage:
		if a.Op == OpRemove {
			return []string{"image", "rm", t.ID}, nil
		}
	}
	return nil, ErrUnsupported
}

// LogsArgs returns the arguments that follow the logs of a project or a
// container, starting from the last 200 lines.
func LogsArgs(t Target) ([]string, error) {
	if t.ID == "" {
		return nil, ErrUnsupported
	}
	switch t.Kind {
	case KindProject:
		return []string{"compose", "-p", t.ID, "logs", "--follow", "--no-color", "--tail", "200"}, nil
	case KindContainer:
		return []string{"logs", "--follow", "--tail", "200", t.ID}, nil
	}
	return nil, ErrUnsupported
}

// ShellArgs returns the arguments that open bash in a container, or sh when
// bash is not installed.
func ShellArgs(containerID string) []string {
	return []string{"exec", "-it", containerID, "sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh"}
}

// Run performs the action and returns Docker's error, if any.
func (c *Client) Run(ctx context.Context, a Action) error {
	args, err := a.Args()
	if err != nil {
		return err
	}
	_, err = c.runner.Output(ctx, args...)
	return err
}

// Logs starts following the logs of a project or a container. Closing the
// reader stops the process.
func (c *Client) Logs(ctx context.Context, t Target) (io.ReadCloser, error) {
	args, err := LogsArgs(t)
	if err != nil {
		return nil, err
	}
	return c.runner.Stream(ctx, args...)
}

// ShellCommand builds the interactive shell command for a running container.
func (c *Client) ShellCommand(containerID string) *exec.Cmd {
	return c.runner.Command(ShellArgs(containerID)...)
}
