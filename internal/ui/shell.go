package ui

import (
	tea "charm.land/bubbletea/v2"
)

// shellDoneMsg reports the end of an interactive shell.
type shellDoneMsg struct{ err error }

// openShell suspends the interface and runs a shell in the running
// container under the cursor.
func (m Model) openShell() (Model, tea.Cmd) {
	if m.tab != tabProjects {
		return m, nil
	}
	r, ok := m.current()
	if !ok || r.container == nil || r.container.State != "running" {
		m.setInfo(m.tr.T("msg.shell_needs_running"))
		return m, nil
	}
	cmd := m.backend.ShellCommand(r.container.ID)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return shellDoneMsg{err: err} })
}

// shellDone reports a failed shell and refreshes the lists.
func (m Model) shellDone(msg shellDoneMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.setError(m.tr.T("fail.shell", "error", msg.err.Error()))
	}
	return m, m.refresh()
}
