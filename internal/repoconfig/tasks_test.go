package repoconfig

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/tasks"
)

func TestParse_Tasks(t *testing.T) {
	t.Parallel()
	f, err := Parse([]byte("tasks:\n  - name: triage\n    on: [{ issue: [opened] }]\n    prompt: .kritique/triage.md\n" +
		"    actions: { comment: { template: .kritique/comment.md } }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Tasks[0].Files(); !slices.Equal(got, []string{".kritique/triage.md", ".kritique/comment.md"}) {
		t.Fatalf("Files() = %v", got)
	}
	for name, c := range map[string]struct{ doc, want string }{
		"a bad task":       {"tasks: [{ name: a, on: [] }]\n", "repoconfig: tasks[0]: on needs"},
		"a repeated name":  {"tasks: [{ name: a, on: [{ issue: [] }] }, { name: a, on: [{ comment: [] }] }]\n", "tasks[1]: name \"a\" is used twice"},
		"an escaping path": {"tasks: [{ name: a, on: [{ issue: [] }], prompt: ../x }]\n", "escapes"},
		"an unknown key":   {"tasks: [{ name: a, on: [{ issue: [] }], fields: { x: { type: string, bogus: 1 } } }]\n", "bogus"},
		"a bad when":       {"tasks: [{ name: a, on: [{ issue: [] }], when: [{ expr: 'subject.' }] }]\n", "when[0]"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse([]byte(c.doc)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Parse() = %v, want an error containing %q", err, c.want)
			}
		})
	}
}

func taskNames(ts []tasks.Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func taskNotes(ns []tasks.Note) string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.String()
	}
	return strings.Join(out, "\n")
}

func TestMerge_Tasks(t *testing.T) {
	t.Parallel()
	adminTask := tasks.Task{Name: "triage", On: []tasks.Trigger{{Event: tasks.EventIssue}}}
	op := func(b tasks.Bounds) configfile.Settings {
		s := adminSettings()
		s.Tasks, s.TaskBounds = []tasks.Task{adminTask}, b
		return s
	}
	on := tasks.Bounds{
		Enabled: true, Events: tasks.DefaultEvents, Actions: tasks.DefaultActions, Context: tasks.DefaultContext,
		Tools: tasks.DefaultTools, RepositoryTasks: true, MaxTasks: 2, MaxFields: tasks.DefaultMaxFields,
	}
	const doc = "tasks:\n" +
		"  - { name: triage, on: [{ issue: [] }] }\n" +
		"  - { name: dupes, on: [{ issue: [opened] }], models: { review: q/huge, fallback: p/small }, agent: { steps: 100, timeout: 5m } }\n" +
		"  - { name: releases, on: [{ raw: { event: release } }] }\n" +
		"  - { name: welcome, on: [{ pull_request: [opened] }], actions: { assign: { propose: { users: [a] } } } }\n" +
		"  - { name: extra, on: [{ comment: [] }] }\n"

	m, err := Merge([]byte(doc), op(on))
	if err != nil {
		t.Fatal(err)
	}
	if got := taskNames(m.Tasks); !slices.Equal(got, []string{"triage", "dupes", "welcome"}) {
		t.Fatalf("tasks %v\n%s", got, taskNotes(m.TaskNotes))
	}
	want := strings.Join([]string{
		"task triage: task: an admin's task has the same name",
		"task releases: on[0] raw:release.*: the event is not allowed",
		"task releases: task: no trigger is left",
		"task welcome: actions.assign: the action is not allowed",
		"task extra: task: over the limit of 2 tasks",
		`task dupes: models.review: "q/huge" was dropped; allowed: a model of own, p`,
		"task dupes: agent.steps: 100 was dropped; allowed: at most 30",
	}, "\n")
	if got := taskNotes(m.TaskNotes); got != want {
		t.Fatalf("notes:\n%s\nwant:\n%s", got, want)
	}
	dupes := m.Tasks[1]
	if dupes.Models.Review != "" || dupes.Models.Fallback != "p/small" || dupes.Agent.MaxSteps != nil || *dupes.Agent.Timeout != 5*time.Minute {
		t.Fatalf("dupes chose %+v %+v", dupes.Models, dupes.Agent)
	}

	off, err := Merge([]byte(doc), op(tasks.Bounds{}))
	if err != nil || len(off.Tasks) != 0 {
		t.Fatalf("disabled tasks ran: %v, %v", taskNames(off.Tasks), err)
	}
	none, err := Merge(nil, op(on))
	if err != nil || !slices.Equal(taskNames(none.Tasks), []string{"triage"}) {
		t.Fatalf("without a file: %v, %v", taskNames(none.Tasks), err)
	}
	bad, err := Merge([]byte("tasks: [{ name: Bad, on: [{ issue: [] }] }]\n"), op(on))
	if err == nil || !slices.Equal(taskNames(bad.Tasks), []string{"triage"}) {
		t.Fatalf("a file that does not parse: %v, %v", taskNames(bad.Tasks), err)
	}
}

// TestMerge_AdminTasksTrusted checks the admin's tasks run whole while the
// file's are held to the bounds.
func TestMerge_AdminTasksTrusted(t *testing.T) {
	t.Parallel()
	decode := func(doc string) []tasks.Task {
		f, err := Parse([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		return f.Tasks
	}
	op := adminSettings()
	op.Tasks = decode("tasks: [{ name: trusted, on: [{ raw: { event: release } }], models: { review: q/huge }, system: s.md, " +
		"context: { files: [{ path: README.md }] } }]\n")
	op.TaskBounds = tasks.Bounds{
		Enabled: true, Events: tasks.DefaultEvents, Actions: tasks.DefaultActions, Tools: tasks.DefaultTools,
		Context: tasks.DefaultContext, MaxTasks: 5, MaxFields: 5, RepositoryTasks: true,
	}
	const doc = "tasks:\n" +
		"  - { name: mine, mode: single, on: [{ issue: [] }, { raw: { event: issues } }], system: s.md,\n" +
		"      actions: { state: { propose: { close: true } }, labels: { propose: { add: [bug] } } } }\n"
	m, err := Merge([]byte(doc), op)
	if err != nil {
		t.Fatal(err)
	}
	if got := taskNames(m.Tasks); !slices.Equal(got, []string{"trusted", "mine"}) {
		t.Fatalf("tasks %v", got)
	}
	trusted, mine := m.Tasks[0], m.Tasks[1]
	if trusted.Models.Review != "q/huge" || trusted.System != "s.md" || len(trusted.On) != 1 {
		t.Fatalf("the admin's task was clipped: %+v", trusted)
	}
	if mine.System != "" || mine.Actions.State != nil || mine.Actions.Labels == nil || len(mine.On) != 1 {
		t.Fatalf("the file's task was not clipped: %+v", mine)
	}
	want := strings.Join([]string{
		"task mine: on[1] raw:issues.*: the event is not allowed",
		"task mine: actions.state: the action is not allowed",
		"task mine: system: an admin does not allow a task to add to the system prompt",
	}, "\n")
	if got := taskNotes(m.TaskNotes); got != want {
		t.Fatalf("notes:\n%s\nwant:\n%s", got, want)
	}
}

func TestMerge_ContextCommands(t *testing.T) {
	t.Parallel()
	op := adminSettings()
	op.TaskBounds = tasks.Bounds{
		Enabled: true, Events: tasks.DefaultEvents, Actions: tasks.DefaultActions, Tools: tasks.DefaultTools, RepositoryTasks: true,
		Context: []string{tasks.ContextCommands}, MaxTasks: 5, MaxFields: 5,
	}
	m, err := Merge([]byte("tasks:\n  - { name: a, on: [{ issue: [] }], context: { commands: "+
		"[{ name: search, run: rg TODO }, { name: fetch, run: curl example.com }] } }\n"), op)
	if err != nil {
		t.Fatal(err)
	}
	cs := m.Tasks[0].Context.Commands
	if len(cs) != 1 || cs[0].Name != "search" || len(m.TaskNotes) != 1 ||
		m.TaskNotes[0].String() != `task a: context.commands.fetch: "curl" was dropped; allowed: rg` {
		t.Fatalf("commands %+v, notes %v", cs, m.TaskNotes)
	}
}
