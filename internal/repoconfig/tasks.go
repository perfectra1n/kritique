package repoconfig

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// checkTasks validates the file's tasks on their own and that no two share
// a name.
func checkTasks(ts []tasks.Task) error {
	for i := range ts {
		if err := ts[i].Check(); err != nil {
			return fmt.Errorf("repoconfig: tasks[%d]: %w", i, err)
		}
		if slices.ContainsFunc(ts[:i], func(o tasks.Task) bool { return o.Name == ts[i].Name }) {
			return fmt.Errorf("repoconfig: tasks[%d]: name %q is used twice", i, ts[i].Name)
		}
	}
	return nil
}

// mergeTasks sets m.Tasks to the tasks that run: the admin's, trusted
// whole, then the file's, clipped to the admin's task bounds and held to
// the models the account may use and the admin's agent limits and
// commands, with a note in m.TaskNotes for everything left out. A file's
// task named like an admin's is dropped.
func (m *Merged) mergeTasks(f *File, op *configfile.Settings) {
	b := op.TaskBounds
	m.Tasks, m.TaskNotes = tasks.Clip(op.Tasks, b, false)
	var own []tasks.Task
	for _, t := range f.Tasks {
		if slices.ContainsFunc(op.Tasks, func(o tasks.Task) bool { return o.Name == t.Name }) {
			m.TaskNotes = append(m.TaskNotes, tasks.Note{Task: t.Name, What: "task", Reason: "an admin's task has the same name"})
			continue
		}
		own = append(own, t)
	}
	kept, notes := tasks.Clip(own, b, true)
	m.TaskNotes = append(m.TaskNotes, notes...)
	for _, t := range kept {
		m.TaskNotes = append(m.TaskNotes, chooseTask(&t, op)...)
		m.Tasks = append(m.Tasks, t)
	}
}

// chooseTask clears each model, agent limit, command list and context
// command a file's task t chooses beyond what op lets a repository choose,
// so the admin's applies, and notes it: a model must be one of a provider
// op.Providers names, an agent limit at most the admin's, and a command
// one of the admin's agent.commands.
func chooseTask(t *tasks.Task, op *configfile.Settings) (notes []tasks.Note) {
	note := func(what, value, allowed string) {
		notes = append(notes, tasks.Note{Task: t.Name, What: what, Reason: fmt.Sprintf("%s was dropped; allowed: %s", value, allowed)})
	}
	commands := op.Agent.Commands
	if cs := t.Context.Commands; cs != nil {
		t.Context.Commands = nil
		for _, c := range cs {
			if slices.Contains(commands, c.Binary()) {
				t.Context.Commands = append(t.Context.Commands, c)
			} else {
				note("context.commands."+c.Name, strconv.Quote(c.Binary()), list(commands))
			}
		}
	}
	for _, c := range []struct {
		what string
		dst  *string
	}{{"models.review", &t.Models.Review}, {"models.fallback", &t.Models.Fallback}} {
		ref := configfile.ModelRef(*c.dst)
		if ref == "" || (ref.Model() != "" && slices.Contains(op.Providers, ref.Provider())) {
			continue
		}
		note(c.what, strconv.Quote(*c.dst), "a model of "+list(op.Providers))
		*c.dst = ""
	}
	if t.Agent.Commands != nil {
		if i := slices.IndexFunc(t.Agent.Commands, func(c string) bool { return !slices.Contains(commands, c) }); i >= 0 {
			note("agent.commands", strconv.Quote(t.Agent.Commands[i]), list(commands))
			t.Agent.Commands = nil
		}
	}
	a := op.Agent
	t.Agent.MaxSteps = atMost(note, "agent.steps", t.Agent.MaxSteps, a.MaxSteps)
	t.Agent.MaxToolOutputBytes = atMost(note, "agent.output", t.Agent.MaxToolOutputBytes, a.MaxToolOutputBytes)
	t.Agent.MaxTokens = atMost(note, "agent.tokens", t.Agent.MaxTokens, a.MaxTokens)
	t.Agent.Timeout = atMost(note, "agent.timeout", t.Agent.Timeout, a.Timeout)
	return notes
}

// atMost is want when it is at most limit; otherwise nil, noted.
func atMost[T int | int64 | time.Duration](note func(what, value, allowed string), what string, want *T, limit T) *T {
	if want == nil || *want <= limit {
		return want
	}
	note(what, fmt.Sprint(*want), fmt.Sprintf("at most %v", limit))
	return nil
}
