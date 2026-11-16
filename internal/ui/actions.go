package ui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sebastienheyd/dctop/internal/docker"
)

const (
	maxConcurrentActions = 4
	actionTimeout        = 5 * time.Minute
)

// actionDoneMsg reports the end of one action.
type actionDoneMsg struct {
	action docker.Action
	err    error
}

var opNames = map[docker.Op]string{
	docker.OpStart:   "start",
	docker.OpStop:    "stop",
	docker.OpRestart: "restart",
	docker.OpRemove:  "remove",
}

func targetKey(t docker.Target) rowKey { return rowKey{kind: t.Kind, id: t.ID} }

// targets returns what an action started now would apply to: the tagged
// rows of the current tab, otherwise the row under the cursor. The
// standalone group stands for its containers, a container whose project is
// also targeted is dropped, and rows with an action in progress are skipped.
func (m Model) targets() []docker.Target {
	rows := m.rows()
	var picked []row
	if tags := m.tagged[m.tab]; len(tags) > 0 {
		for _, r := range rows {
			if tags[r.key] {
				picked = append(picked, r)
			}
		}
	} else if r, ok := m.current(); ok {
		picked = append(picked, r)
	}

	var out []docker.Target
	projects := map[string]bool{}
	for _, r := range picked {
		if r.key.kind == docker.KindProject && r.project != nil && !r.project.Standalone() {
			projects[r.project.Name] = true
		}
	}
	seen := map[rowKey]bool{}
	add := func(t docker.Target) {
		k := targetKey(t)
		if seen[k] {
			return
		}
		if _, busy := m.busy[k]; busy {
			return
		}
		seen[k] = true
		out = append(out, t)
	}
	for _, r := range picked {
		switch {
		case r.container != nil:
			if !projects[r.container.Project] || r.container.Project == "" {
				add(docker.Target{Kind: docker.KindContainer, ID: r.container.ID, Name: r.container.Name})
			}
		case r.project != nil && r.project.Standalone():
			for _, c := range r.project.Containers {
				add(docker.Target{Kind: docker.KindContainer, ID: c.ID, Name: c.Name})
			}
		case r.project != nil:
			add(docker.Target{Kind: docker.KindProject, ID: r.project.Name, Name: r.project.Name})
		case r.volume != nil:
			add(docker.Target{Kind: docker.KindVolume, ID: r.volume.Name, Name: r.volume.Name})
		case r.image != nil:
			add(docker.Target{Kind: docker.KindImage, ID: r.image.ID, Name: imageName(r.image)})
		}
	}
	return out
}

// launch marks the targets busy, clears the tags of the current tab and
// returns one command per target. At most four run at the same time.
func (m Model) launch(op docker.Op, ts []docker.Target) (Model, tea.Cmd) {
	if len(ts) == 0 {
		return m, nil
	}
	delete(m.tagged, m.tab)
	cmds := make([]tea.Cmd, 0, len(ts))
	for _, t := range ts {
		m.busy[targetKey(t)] = op
		cmds = append(cmds, m.runAction(docker.Action{Op: op, Target: t}))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) runAction(a docker.Action) tea.Cmd {
	b, sem, parent := m.backend, m.sem, m.ctx
	return func() tea.Msg {
		select {
		case sem <- struct{}{}:
		case <-parent.Done():
			return actionDoneMsg{action: a, err: parent.Err()}
		}
		defer func() { <-sem }()
		ctx, cancel := context.WithTimeout(parent, actionTimeout)
		defer cancel()
		return actionDoneMsg{action: a, err: b.Run(ctx, a)}
	}
}

// actionDone clears the busy mark, reports the result and refreshes.
func (m Model) actionDone(msg actionDoneMsg) (Model, tea.Cmd) {
	delete(m.busy, targetKey(msg.action.Target))
	op := opNames[msg.action.Op]
	name := msg.action.Target.Name
	if msg.err != nil {
		failKey := "fail." + op
		m.setError(m.tr.T(failKey, "name", name, "error", msg.err.Error()))
	} else {
		doneKey := "done." + op
		m.setInfo(m.tr.T(doneKey, "name", name))
	}
	return m, m.refresh()
}

// startAction runs start, stop or restart on the current targets.
func (m Model) startAction(op docker.Op) (Model, tea.Cmd) {
	return m.launch(op, m.supported(op, m.targets()))
}

// supported keeps the targets that op applies to.
func (m Model) supported(op docker.Op, ts []docker.Target) []docker.Target {
	var out []docker.Target
	for _, t := range ts {
		if _, err := (docker.Action{Op: op, Target: t}).Args(); err == nil {
			out = append(out, t)
		}
	}
	return out
}

// askRemove opens the confirmation box for the current targets.
func (m Model) askRemove() Model {
	ts := m.supported(docker.OpRemove, m.targets())
	if len(ts) == 0 {
		return m
	}
	var text string
	if len(ts) > 1 {
		text = m.tr.T("confirm.remove.many", "count", fmt.Sprint(len(ts)))
	} else {
		t := ts[0]
		switch t.Kind {
		case docker.KindProject:
			text = m.tr.T("confirm.remove.project", "name", t.Name)
		case docker.KindContainer:
			text = m.tr.T("confirm.remove.container", "name", t.Name)
		case docker.KindVolume:
			text = m.tr.T("confirm.remove.volume", "name", t.Name)
		case docker.KindImage:
			text = m.tr.T("confirm.remove.image", "name", t.Name)
		}
	}
	m.confirm = &confirmState{text: text, targets: ts}
	return m
}

// imageName is how an image is shown: repository:tag.
func imageName(img *docker.Image) string {
	return img.Repository + ":" + img.Tag
}
