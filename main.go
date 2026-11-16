// Command dctop is an htop-like terminal dashboard for Docker Compose projects.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
	"github.com/sebastienheyd/dctop/internal/i18n"
	"github.com/sebastienheyd/dctop/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

const infoTimeout = 5 * time.Second

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// deps holds what runWith needs from the outside world, so that tests can
// replace it.
type deps struct {
	lookPath   func(string) (string, error)
	runner     docker.Runner
	runProgram func(ui.Model) error
	home       string
}

// run parses the command line and returns the process exit code.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	home, _ := os.UserHomeDir()
	return runWith(args, stdout, stderr, getenv, deps{
		lookPath: exec.LookPath,
		runner:   docker.ExecRunner{},
		runProgram: func(m ui.Model) error {
			_, err := tea.NewProgram(m).Run()
			return err
		},
		home: home,
	})
}

// runWith parses flags, picks the language, checks that Docker answers, and
// runs the interface. Errors found before the screen is taken over are
// printed on stderr.
func runWith(args []string, stdout, stderr io.Writer, getenv func(string) string, d deps) int {
	fs := flag.NewFlagSet("dctop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "interface language (en, fr)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	help := fs.Bool("help", false, "print this help and exit")
	fs.BoolVar(help, "h", false, "print this help and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	tr := i18n.New(i18n.Detect(*lang, getenv))
	if *showVersion {
		fmt.Fprintf(stdout, "dctop %s\n", version)
		return 0
	}
	if *help {
		fmt.Fprintln(stdout, tr.T("app.usage"))
		return 0
	}
	if _, err := d.lookPath("docker"); err != nil {
		fmt.Fprintln(stderr, tr.T("err.docker_missing"))
		return 1
	}
	client := docker.NewClient(d.runner)
	ctx, cancel := context.WithTimeout(context.Background(), infoTimeout)
	info, err := client.Info(ctx)
	cancel()
	if err != nil {
		fmt.Fprintln(stderr, tr.T("err.daemon", "error", err.Error()))
		return 1
	}
	m := ui.New(ui.Options{Backend: client, Tr: tr, Info: info, Version: version, Home: d.home})
	if err := d.runProgram(m); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
