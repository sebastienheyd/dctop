// Command dctop is an htop-like terminal dashboard for Docker Compose projects.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// run parses the command line and returns the process exit code.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("dctop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "dctop %s\n", version)
		return 0
	}
	return 0
}
