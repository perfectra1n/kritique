package webapi

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/tasks"
)

func TestTaskRunDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		row  store.TaskRunRow
		want *int64
	}{
		{"queued", store.TaskRunRow{CreatedAt: t0}, nil},
		{"from start", store.TaskRunRow{CreatedAt: t0.Add(-30 * time.Second), StartedAt: &t0, FinishedAt: &t1}, new(int64(90000))},
		{"skipped before it started", store.TaskRunRow{CreatedAt: t0, FinishedAt: &t1}, new(int64(90000))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := taskRun(tt.row).DurationMs
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("DurationMs = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTaskRunDetailDecodesRecords feeds the records as the task worker
// stores them.
func TestTaskRunDetailDecodesRecords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      store.TaskRunDetail
		want    func(TaskRunDetail) bool
		wantErr bool
	}{
		{
			name: "queued run has empty records",
			in:   store.TaskRunDetail{},
			want: func(d TaskRunDetail) bool {
				return d.Proposed == nil && d.Applied == nil && len(d.Fields) == 0 && d.Fields != nil && d.Dropped != nil && d.Event == nil
			},
		},
		{
			name: "finished run",
			in: store.TaskRunDetail{
				Event:  &store.TaskEventRow{RawEvent: "issues", Action: "opened", Sender: "ada"},
				Fields: json.RawMessage(`{"priority":"high","area":["db"]}`),
				Proposed: json.RawMessage(`{"summary":"s","fields":{"priority":"high"},"labels":{"add":["bug","x"],"remove":null},
					"inline":[{"path":"a.go","line":3,"end_line":4,"body":"b"}]}`),
				Applied: json.RawMessage(`{"add_labels":["bug"],"inline":[{"path":"a.go","line":3,"end_line":4,"body":"b"}],"comment":"sticky"}`),
				// tasks.Drop has no JSON tags: the worker writes its field names.
				Dropped: json.RawMessage(`[{"Action":"labels.add","Value":"x","Reason":"it is not a label the task lets the model add"}]`),
				Notes:   []string{"context.related: the index is not built yet"},
			},
			want: func(d TaskRunDetail) bool {
				return string(d.Fields["area"]) == `["db"]` && d.Event.Sender == "ada" &&
					reflect.DeepEqual(d.Proposed.AddLabels, []string{"bug", "x"}) &&
					d.Proposed.Inline[0] == TaskInline{Path: "a.go", Line: 3, EndLine: 4, Body: "b"} &&
					reflect.DeepEqual(d.Applied.AddLabels, []string{"bug"}) &&
					d.Applied.Inline[0].EndLine == 4 && d.Applied.Comment == "sticky" &&
					d.Dropped[0] == TaskDrop{Action: "labels.add", Value: "x", Reason: "it is not a label the task lets the model add"} &&
					reflect.DeepEqual(d.Notes, []string{"context.related: the index is not built yet"})
			},
		},
		{name: "corrupt record", in: store.TaskRunDetail{Applied: json.RawMessage(`[1]`)}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := taskRunDetail(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !tt.want(got) {
				t.Errorf("taskRunDetail = %+v", got)
			}
		})
	}
}

func TestRepoTasks(t *testing.T) {
	t.Parallel()
	on := configfile.TaskBounds{Enabled: new(true)}
	settings := func(b configfile.TaskBounds) configfile.Settings {
		return configfile.Settings{
			Tasks: []tasks.Task{{
				Name: "welcome", On: []tasks.Trigger{{Event: tasks.EventPullRequest, Actions: []string{"opened"}}},
				Mode: tasks.ModeSingle, Actions: tasks.Actions{Comment: &tasks.CommentSpec{}},
			}},
			TaskBounds: b.Resolve(),
		}
	}
	doc := "tasks:\n" +
		"  - name: triage\n" +
		"    mode: single\n" +
		"    when: [{ expr: 'subject.kind == \"issue\"' }]\n" +
		"    on: [{ issue: [opened, reopened] }, { raw: { event: issues, actions: [transferred] } }]\n" +
		"    actions: { labels: { propose: { add: [bug] } }, state: { propose: { close: true } }, comment: { mode: none } }\n"
	bad := "tasks: [{ name: Bad, on: [{ issue: [] }] }]\n"

	welcome := TaskDef{
		Name: "welcome", Source: TaskFromFile, Triggers: []string{"pull_request.opened"}, Mode: tasks.ModeSingle,
		Actions: []string{tasks.ActionComment},
	}
	triage := TaskDef{
		Name: "triage", Source: TaskFromRepository, Triggers: []string{"issue.opened", "issue.reopened"},
		When: []tasks.When{{Expr: `subject.kind == "issue"`}}, Mode: tasks.ModeSingle, Actions: []string{tasks.ActionLabels},
	}
	triageNotes := []TaskNote{
		{Task: "triage", What: "on[1] raw:issues.transferred", Reason: "the event is not allowed"},
		{Task: "triage", What: "actions.state", Reason: "the action is not allowed"},
	}
	tests := []struct {
		name        string
		bounds      configfile.TaskBounds
		file        *string
		wantTasks   []TaskDef
		wantNotes   []TaskNote
		wantIgnored bool
	}{
		{name: "no file read yet", bounds: on, wantTasks: []TaskDef{welcome}, wantNotes: []TaskNote{}},
		{name: "a file with a task", bounds: on, file: &doc, wantTasks: []TaskDef{welcome, triage}, wantNotes: triageNotes},
		{name: "a file that does not parse", bounds: on, file: &bad, wantTasks: []TaskDef{welcome}, wantNotes: []TaskNote{}, wantIgnored: true},
		{
			name: "tasks disabled", file: &doc, wantTasks: []TaskDef{},
			wantNotes: []TaskNote{
				{Task: "welcome", What: "task", Reason: "tasks are disabled"},
				{Task: "triage", What: "task", Reason: "tasks are disabled"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotTasks, gotNotes, ignored := repoTasks(settings(tt.bounds), tt.file)
			if !reflect.DeepEqual(gotTasks, tt.wantTasks) {
				t.Errorf("tasks = %+v, want %+v", gotTasks, tt.wantTasks)
			}
			if !reflect.DeepEqual(gotNotes, tt.wantNotes) {
				t.Errorf("notes = %+v, want %+v", gotNotes, tt.wantNotes)
			}
			if (ignored != "") != tt.wantIgnored {
				t.Errorf("ignored = %q, want it set: %v", ignored, tt.wantIgnored)
			}
		})
	}
}
