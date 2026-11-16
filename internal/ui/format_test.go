package ui

import (
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0: "0B", 1023: "1023B", 1536: "1.50K", 150 << 20: "150M", 5 << 30: "5.00G", 16 << 30: "16.0G", -1: "",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("%d: got %q want %q", in, got, want)
		}
	}
}

func TestFormatCPU(t *testing.T) {
	if got := formatCPU(2); got != "2.0%" {
		t.Errorf("%q", got)
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"abcdef", 4, "abc…"}, {"abc", 3, "abc"}, {"abc", 0, ""}, {"abc", -5, ""},
	}
	for _, c := range cases {
		if got := truncate(c.in, c.width); got != c.want {
			t.Errorf("(%q, %d): got %q want %q", c.in, c.width, got, c.want)
		}
	}
	got := truncate("日本語テキスト", 5)
	if lipgloss.Width(got) > 5 || !strings.HasSuffix(got, "…") {
		t.Errorf("wide: %q (%d)", got, lipgloss.Width(got))
	}
}

func TestShortenHome(t *testing.T) {
	cases := []struct{ path, home, want string }{
		{"/home/me/docker/x", "/home/me", "~/docker/x"},
		{"/home/me", "/home/me", "~"},
		{"/home/meow/x", "/home/me", "/home/meow/x"},
		{"/srv/x", "", "/srv/x"},
	}
	for _, c := range cases {
		if got := shortenHome(c.path, c.home); got != c.want {
			t.Errorf("(%q, %q): got %q", c.path, c.home, got)
		}
	}
}

func TestMeter(t *testing.T) {
	got := meter("CPU", 0.5, "50.0%", 20)
	p := ansi.Strip(got)
	if lipgloss.Width(got) != 20 || !strings.HasPrefix(p, "CPU [") || !strings.HasSuffix(p, "50.0%]") || strings.Count(p, "|") != 7 {
		t.Errorf("%q", p)
	}
	full := ansi.Strip(meter("CPU", 1.7, "", 20))
	if strings.Count(full, "|") != 14 {
		t.Errorf("frac > 1: %q", full)
	}
	for _, f := range []float64{-1, math.NaN()} {
		if s := ansi.Strip(meter("CPU", f, "", 20)); strings.Count(s, "|") != 0 {
			t.Errorf("frac %v: %q", f, s)
		}
	}
	for _, w := range []int{0, 2, 5, 6} {
		if s := meter("Containers", 0.5, "4/7", w); lipgloss.Width(s) > w {
			t.Errorf("width %d: %q", w, s)
		}
	}
}
