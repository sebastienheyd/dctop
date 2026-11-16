package main

import (
	"bytes"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--version"}, &out, &errOut, func(string) string { return "" })
	if code != 0 || out.String() != "dctop dev\n" || errOut.Len() != 0 {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--nope"}, &out, &errOut, func(string) string { return "" }); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
