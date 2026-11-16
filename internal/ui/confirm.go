package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sebastienheyd/dctop/internal/docker"
)

// confirmState is an open removal confirmation. Its targets were resolved
// when it opened: the removal applies to them even if the list changed.
type confirmState struct {
	text    string
	targets []docker.Target
}

// confirmKey handles a key while the confirmation is open: "y", or the yes
// letter of the interface language, removes; anything else cancels.
func (m Model) confirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	c := m.confirm
	m.confirm = nil
	k := msg.String()
	if k == "y" || k == m.tr.T("confirm.yes_key") {
		return m.launch(docker.OpRemove, c.targets)
	}
	return m, nil
}

// overlayConfirm draws the confirmation box centred over the body lines.
func (m Model) overlayConfirm(body []string) []string {
	if m.confirm == nil || len(body) == 0 {
		return body
	}
	text := m.confirm.text
	hint := m.tr.T("confirm.keys")
	inner := max(ansi.StringWidth(text), ansi.StringWidth(hint))
	inner = max(1, min(inner, m.width-4))
	box := styleBox.Width(inner + 4).Render(text + "\n\n" + hint)
	lines := strings.Split(box, "\n")
	top := max(0, (len(body)-len(lines))/2)
	out := append([]string(nil), body...)
	for i, l := range lines {
		if top+i >= len(out) {
			break
		}
		left := max(0, (m.width-ansi.StringWidth(l))/2)
		out[top+i] = strings.Repeat(" ", left) + l
	}
	return out
}
