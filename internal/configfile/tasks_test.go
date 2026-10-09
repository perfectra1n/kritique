package configfile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/perfectra1n/kritique/internal/tasks"
)

func TestTasks(t *testing.T) {
	t.Setenv("TEST_PRIVATE_KEY", "tok")
	t.Setenv("TEST_WEBHOOK_SECRET", "whsec")
	const head = `providers:
  p: { type: openai, apiKey: { env: TEST_WEBHOOK_SECRET } }
tasks:
  - { name: triage, on: [{ issue: [opened] }], actions: { labels: { propose: { add: [bug] } } } }
taskBounds: { enabled: true, events: ["issue.*"], maxTasks: 3 }
`
	f := mustLoad(t, head+acme("  acme/*: { taskBounds: { actions: [comment, state] } }\n"+
		"  acme/x: { tasks: [{ name: other, on: [{ comment: [] }] }], taskBounds: { maxTasks: 1, repository: false } }\n"))
	account := f.Settings(&f.Accounts[0], "")
	if len(account.Tasks) != 1 || account.Tasks[0].Name != "triage" {
		t.Fatalf("account tasks = %+v", account.Tasks)
	}
	want := tasks.Bounds{
		Enabled: true, Events: []string{"issue.*"}, Actions: []string{"comment", "state"}, Context: tasks.DefaultContext,
		Tools: tasks.DefaultTools, RepositoryTasks: true, MaxTasks: 3, MaxRunsPerSubjectPerHour: tasks.DefaultMaxRunsPerSubjectPerHour,
		MaxFields: tasks.DefaultMaxFields,
	}
	if !reflect.DeepEqual(account.TaskBounds, want) {
		t.Fatalf("account bounds = %+v, want %+v", account.TaskBounds, want)
	}
	repo := f.Settings(&f.Accounts[0], "acme/x")
	want.MaxTasks, want.RepositoryTasks = 1, false
	if len(repo.Tasks) != 2 || repo.Tasks[1].Name != "other" || !reflect.DeepEqual(repo.TaskBounds, want) {
		t.Fatalf("repository tasks = %+v, bounds %+v", repo.Tasks, repo.TaskBounds)
	}
	if s := f.Sources(&f.Accounts[0], "acme/x"); s["tasks"] != SourceAccount || s["taskBounds.enabled"] != SourceDefaults {
		t.Fatalf("tasks from %s, taskBounds.enabled from %s", s["tasks"], s["taskBounds.enabled"])
	}

	tests := []struct {
		name, entry, want string
	}{
		{"a task that does not check", "tasks: [{ name: Bad, on: [{ issue: [] }] }]", "acme/*.tasks[0]: name"},
		{"two tasks with one name", "tasks: [{ name: a, on: [{ issue: [] }] }, { name: a, on: [{ comment: [] }] }]",
			"tasks[1]: name \"a\" is used twice"},
		{"a task model of an undeclared provider", "tasks: [{ name: a, on: [{ issue: [] }], models: { review: q/big } }]",
			"tasks[0].models.review references provider \"q\""},
		{"an unknown task key", "tasks: [{ name: a, on: [{ issue: [] }], bogus: 1 }]", "bogus"},
		{"an old if key", "tasks: [{ name: a, on: [{ issue: [] }], if: 'true' }]", "field if not found"},
		{"an unknown action kind", "taskBounds: { actions: [delete] }", "taskBounds.actions[0] must be one of"},
		{"an unknown context kind", "taskBounds: { context: [web] }", "taskBounds.context[0] must be one of"},
		{"a bad event glob", "taskBounds: { events: ['['] }", "taskBounds.events[0]"},
		{"an event glob with ?", "taskBounds: { events: ['raw:issue?.opened'] }", "taskBounds.events[0] \"raw:issue?.opened\" may use * and ** only"},
		{"an event glob with a class", "taskBounds: { events: ['issue.*', 'raw:issue[s].*'] }", "taskBounds.events[1]"},
		{"a bad tool name", "taskBounds: { tools: [Read-File] }", "taskBounds.tools[0]"},
		{"a cap that is not positive", "taskBounds: { maxFields: 0 }", "taskBounds.maxFields must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, head+acme("  acme/*: { "+tt.entry+" }\n"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse = %v, want an error mentioning %q", err, tt.want)
			}
		})
	}
}

func TestTaskBoundsDefaults(t *testing.T) {
	t.Parallel()
	got := TaskBounds{}.Resolve()
	want := tasks.Bounds{
		Events: tasks.DefaultEvents, Actions: tasks.DefaultActions, Context: tasks.DefaultContext, Tools: tasks.DefaultTools,
		RepositoryTasks: true, MaxTasks: tasks.DefaultMaxTasks, MaxRunsPerSubjectPerHour: tasks.DefaultMaxRunsPerSubjectPerHour,
		MaxFields: tasks.DefaultMaxFields,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
	got.Events[0], got.Actions[0], got.Context[0], got.Tools[0] = "x", "x", "x", "x"
	if tasks.DefaultEvents[0] == "x" || tasks.DefaultActions[0] == "x" || tasks.DefaultContext[0] == "x" || tasks.DefaultTools[0] == "x" {
		t.Fatal("Resolve handed out the package defaults")
	}
}

// TestTasksMergeByName checks the operator's tasks merge across scopes by
// name, and that enabled false switches an inherited one off.
func TestTasksMergeByName(t *testing.T) {
	t.Setenv("TEST_PRIVATE_KEY", "tok")
	t.Setenv("TEST_WEBHOOK_SECRET", "whsec")
	f := mustLoad(t, `tasks:
  - { name: triage, on: [{ issue: [] }] }
  - { name: stale, on: [{ issue: [] }] }
  - { name: welcome, on: [{ pull_request: [] }] }
`+acme(`  acme/*:
    tasks:
      - { name: triage, on: [{ comment: [] }] }
      - { name: extra, on: [{ issue: [] }] }
  acme/x: { tasks: [{ name: stale, enabled: false }, { name: welcome, mode: single, on: [{ pull_request: [opened] }] }] }
`))
	s := f.Settings(&f.Accounts[0], "acme/x")
	got := make([]string, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		got = append(got, t.Name+":"+t.On[0].Event+":"+string(t.Mode))
	}
	if strings.Join(got, ",") != "triage:comment:,welcome:pull_request:single,extra:issue:" {
		t.Fatalf("tasks %v", got)
	}
	if s := f.Settings(&f.Accounts[0], "acme/y"); len(s.Tasks) != 4 {
		t.Fatalf("another repository's tasks = %+v", s.Tasks)
	}
}
