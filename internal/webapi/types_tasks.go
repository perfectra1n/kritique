package webapi

import (
	"encoding/json"
	"time"

	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// TaskRun is one task run as lists show it. SubjectKind is "issue", "pull"
// or "" for an event with no subject. DurationMs is null until it
// finishes.
type TaskRun struct {
	ID            string              `json:"id"`
	Repository    string              `json:"repository"`
	Task          string              `json:"task"`
	SubjectKind   string              `json:"subjectKind"`
	SubjectNumber int                 `json:"subjectNumber"`
	Trigger       string              `json:"trigger"`
	Mode          string              `json:"mode"`
	Status        store.TaskRunStatus `json:"status"`
	Reason        string              `json:"reason"`
	Model         string              `json:"model"`
	ConfigSHA     string              `json:"configSha"`
	CommentID     *int64              `json:"commentId"`
	Error         string              `json:"error"`
	DroppedCount  int                 `json:"droppedCount"`
	CreatedAt     time.Time           `json:"createdAt"`
	StartedAt     *time.Time          `json:"startedAt"`
	FinishedAt    *time.Time          `json:"finishedAt"`
	DurationMs    *int64              `json:"durationMs"`
}

// TaskEvent is the delivery a task run ran on.
type TaskEvent struct {
	Forge      string    `json:"forge"`
	Event      string    `json:"event"`
	RawEvent   string    `json:"rawEvent"`
	Action     string    `json:"action"`
	Sender     string    `json:"sender"`
	Delivery   string    `json:"delivery"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// TaskInline is an inline comment on a pull request's diff.
type TaskInline struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	EndLine int    `json:"endLine"`
	Body    string `json:"body"`
}

// TaskAnswer is what the model proposed.
type TaskAnswer struct {
	Summary      string       `json:"summary"`
	Comment      string       `json:"comment"`
	AddLabels    []string     `json:"addLabels"`
	RemoveLabels []string     `json:"removeLabels"`
	State        string       `json:"state"`
	Assignees    []string     `json:"assignees"`
	Reviewers    []string     `json:"reviewers"`
	Inline       []TaskInline `json:"inline"`
}

// TaskApplied is what the forge accepted. Comment is the report comment's
// mode, "" when none was posted.
type TaskApplied struct {
	AddLabels    []string     `json:"addLabels"`
	RemoveLabels []string     `json:"removeLabels"`
	State        string       `json:"state"`
	Assignees    []string     `json:"assignees"`
	Reviewers    []string     `json:"reviewers"`
	Inline       []TaskInline `json:"inline"`
	Comment      string       `json:"comment"`
}

// TaskDrop is an action a run left out, and why.
type TaskDrop struct {
	Action string `json:"action"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// TaskRunDetail is one task run with its event and records. Event is null
// once retention swept it; Proposed and Applied are null until the model
// answered. Notes say what the run's context left out, and why.
type TaskRunDetail struct {
	Run        TaskRun                    `json:"run"`
	Event      *TaskEvent                 `json:"event"`
	Fields     map[string]json.RawMessage `json:"fields"`
	Proposed   *TaskAnswer                `json:"proposed"`
	Applied    *TaskApplied               `json:"applied"`
	Dropped    []TaskDrop                 `json:"dropped"`
	Notes      []string                   `json:"notes"`
	ModelCalls int                        `json:"modelCalls"`
	CostUSD    float64                    `json:"costUsd"`
	Tokens     TokenCounts                `json:"tokens"`
}

// RepoTasks are the tasks that run for one repository: the admin's and
// those of its .kritique.yaml, after the admin's task bounds clipped them.
// Enabled is the admin's taskBounds.enabled; off, no task runs and Tasks
// is empty. Source says which .kritique.yaml they were resolved from: the
// one a task dispatch last read at the default branch tip, where tasks run
// from, or, before any dispatch, the one the last review read at its merge
// base; "" when neither has been read. Commit is that file's commit,
// ResolvedAt when a dispatch read it (null for a review's), and Found
// whether the commit had a file at all. Ignored is why the file was
// ignored as a whole, when it does not parse: its tasks are left out, as
// a dispatch leaves them out. Notes say what the bounds left out.
type RepoTasks struct {
	Enabled    bool       `json:"enabled"`
	Source     string     `json:"source"`
	Commit     string     `json:"commit"`
	ResolvedAt *time.Time `json:"resolvedAt"`
	Found      bool       `json:"found"`
	Ignored    string     `json:"ignored,omitempty"`
	Tasks      []TaskDef  `json:"tasks"`
	Notes      []TaskNote `json:"notes"`
}

// Values of RepoTasks.Source.
const (
	TasksSourceDefaultBranch = "defaultBranch"
	TasksSourceLastReview    = "lastReview"
)

// TaskSource is where a task is defined.
type TaskSource string

// Task sources: the admin's configuration file, or the repository's
// .kritique.yaml.
const (
	TaskFromFile       TaskSource = "file"
	TaskFromRepository TaskSource = "repository"
)

// TaskDef is one resolved task and where it is defined. Triggers are the
// event names it runs on, as taskBounds.events globs them, When the
// conditions one of which must hold for it to run (empty always runs),
// and Actions the action kinds it declares.
type TaskDef struct {
	Name     string       `json:"name"`
	Source   TaskSource   `json:"source"`
	Triggers []string     `json:"triggers"`
	When     []tasks.When `json:"when"`
	Mode     tasks.Mode   `json:"mode"`
	Actions  []string     `json:"actions"`
}

// TaskNote is something the admin's task bounds left out of a task, and
// why.
type TaskNote struct {
	Task   string `json:"task"`
	What   string `json:"what"`
	Reason string `json:"reason"`
}
