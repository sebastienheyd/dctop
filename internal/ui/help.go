package ui

import (
	"github.com/charmbracelet/x/ansi"
)

type mode int

const (
	modeMain mode = iota
	modeHelp
)

// helpEntry is one line of the help screen: the keys and a catalog key.
type helpEntry struct {
	keys  string
	label string
}

var helpEntries = []helpEntry{
	{"↑ ↓ k j PgUp PgDn Home End", "help.move"},
	{"→ ← Enter", "help.expand"},
	{"Space", "help.tag"},
	{"U", "help.untag_all"},
	{"Tab Shift+Tab 1 2 3", "help.tabs"},
	{"F1 ?", "key.help"},
	{"F2 s", "key.start"},
	{"F3 x", "key.stop"},
	{"F4 /", "key.filter"},
	{"F5 r", "key.restart"},
	{"F6 o", "key.sort"},
	{"F7 l", "key.logs"},
	{"F8 e", "key.shell"},
	{"F9 d", "key.remove"},
	{"H", "help.hide"},
	{"F10 q Ctrl+C", "key.quit"},
	{"Esc q", "key.back"},
	{"f End", "key.follow"},
}

// helpLines renders the help screen, which replaces the whole frame.
func (m Model) helpLines() []string {
	width := 0
	for _, e := range helpEntries {
		width = max(width, ansi.StringWidth(e.keys))
	}
	lines := []string{styleKeyName.Render(m.tr.T("help.title")), ""}
	for _, e := range helpEntries {
		lines = append(lines, "  "+styleKeyName.Render(pad(e.keys, width))+"  "+m.tr.T(e.label))
	}
	return append(lines, "", m.tr.T("help.close"))
}
