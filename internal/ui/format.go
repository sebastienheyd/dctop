package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// formatBytes prints a size with binary units: two decimals below 10, one
// below 100, none above. Negative sizes print as "".
func formatBytes(n int64) string {
	if n < 0 {
		return ""
	}
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	f := float64(n)
	units := "KMGT"
	i := -1
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	switch {
	case f < 10:
		return fmt.Sprintf("%.2f%c", f, units[i])
	case f < 100:
		return fmt.Sprintf("%.1f%c", f, units[i])
	}
	return fmt.Sprintf("%.0f%c", f, units[i])
}

// formatCPU prints a CPU percentage, 100 being one core.
func formatCPU(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}

// truncate cuts s to width display cells, ending with "…" when cut. Escape
// sequences are kept and do not count.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// pad truncates s to width and fills it with spaces up to width.
func pad(s string, width int) string {
	s = truncate(s, width)
	if w := ansi.StringWidth(s); w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// padLeft truncates s to width and right-aligns it.
func padLeft(s string, width int) string {
	s = truncate(s, width)
	if w := ansi.StringWidth(s); w < width {
		s = strings.Repeat(" ", width-w) + s
	}
	return s
}

// shortenHome replaces the home directory at the start of path with "~".
func shortenHome(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

// meter draws an htop-like gauge: label, a space, then brackets holding
// floor(frac × inner) bars, with text right-aligned over the end.
func meter(label string, frac float64, text string, width int) string {
	if width <= 0 {
		return ""
	}
	inner := width - ansi.StringWidth(label) - 3
	if inner < 1 {
		return truncate(label, width)
	}
	if math.IsNaN(frac) || frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	bars := int(math.Floor(frac * float64(inner)))
	text = truncate(text, inner)
	textWidth := ansi.StringWidth(text)
	free := inner - textWidth // cells not covered by the text
	shown := min(bars, free)
	gauge := styleMeterBar.Render(strings.Repeat("|", shown)) +
		strings.Repeat(" ", free-shown) +
		styleMeterText.Render(text)
	return label + " [" + gauge + "]"
}
