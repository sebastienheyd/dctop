package ui

import (
	"strings"
	"time"

	"github.com/sebastienheyd/dctop/internal/i18n"
)

const infoLifetime = 4 * time.Second

// statusText renders the status line: an informative message for 4 s, an
// error until the next key press.
func (m Model) statusText() string {
	s := m.status
	if s.text == "" {
		return ""
	}
	if s.isErr {
		return styleError.Render(s.text)
	}
	if m.now.Sub(s.at) >= infoLifetime {
		return ""
	}
	return s.text
}

// keyBinding is one entry of the key bar.
type keyBinding struct {
	key   string
	label string // catalog key
}

func (m Model) keyBindings() []keyBinding {
	return []keyBinding{
		{"F1", "key.help"}, {"F2", "key.start"}, {"F3", "key.stop"}, {"F4", "key.filter"},
		{"F5", "key.restart"}, {"F6", "key.sort"}, {"F7", "key.logs"}, {"F8", "key.shell"},
		{"F9", "key.remove"}, {"F10", "key.quit"},
	}
}

// keyBar renders the htop-like key bar.
func (m Model) keyBar() string {
	return renderKeyBar(m.tr, m.keyBindings())
}

func renderKeyBar(tr *i18n.Translator, bs []keyBinding) string {
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		parts = append(parts, styleKeyName.Render(b.key)+styleKeyLabel.Render(tr.T(b.label)))
	}
	return strings.Join(parts, " ")
}
