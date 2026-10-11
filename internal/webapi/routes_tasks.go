package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/repoconfig"
	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/tasks"
	"github.com/perfectra1n/kritique/internal/transcript"
)

// registerTasks mounts the reads over event tasks: an account's task runs
// and the tasks that run for one of its repositories.
func (s *Server) registerTasks(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/accounts/{forge}/{name}/task-runs", s.account(s.listTaskRuns))
	mux.HandleFunc("GET /api/v1/accounts/{forge}/{name}/task-runs/{id}", s.account(s.getTaskRun))
	mux.HandleFunc("GET /api/v1/accounts/{forge}/{name}/task-runs/{id}/transcript", s.account(s.getTaskRunTranscript))
	mux.HandleFunc("GET /api/v1/accounts/{forge}/{name}/repos/{owner}/{repo}/tasks", s.account(s.getRepoTasks))
}

// listTaskRuns lists the account's task runs, newest first, narrowed by
// ?repo=, ?task= and ?status=.
func (s *Server) listTaskRuns(w http.ResponseWriter, r *http.Request, t *accountScope) error {
	page, err := parsePage(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	f := store.TaskRunFilter{Task: q.Get("task"), Status: store.TaskRunStatus(q.Get("status"))}
	if f.Status != "" && !f.Status.Valid() {
		return errBadRequest(CodeBadRequest, "status must be queued, running, succeeded, failed or skipped")
	}
	ctx := r.Context()
	var rows []store.TaskRunRow
	var next *store.Cursor
	if err := s.read(ctx, t, func(tx pgx.Tx) error {
		if f.RepositoryID, err = repoFilter(ctx, tx, r); err != nil {
			return err
		}
		rows, next, err = store.ListTaskRuns(ctx, tx, f, page)
		return err
	}); err != nil {
		return err
	}
	items := make([]TaskRun, len(rows))
	for i, row := range rows {
		items[i] = taskRun(row)
	}
	writeJSON(w, http.StatusOK, newPage(items, next))
	return nil
}

// taskRun is x as lists show it. A run skipped before it started took
// from when it was queued.
func taskRun(x store.TaskRunRow) TaskRun {
	out := TaskRun{
		ID: x.ID, Repository: x.Repository, Task: x.Task, SubjectKind: x.SubjectKind, SubjectNumber: x.SubjectNumber,
		Trigger: x.Trigger, Mode: x.Mode, Status: x.Status, Reason: x.Reason, Model: x.Model, ConfigSHA: x.ConfigSHA,
		CommentID: x.CommentID, Error: x.Error, DroppedCount: x.DroppedCount, CreatedAt: x.CreatedAt, StartedAt: x.StartedAt,
		FinishedAt: x.FinishedAt,
	}
	if x.FinishedAt != nil {
		start := x.CreatedAt
		if x.StartedAt != nil {
			start = *x.StartedAt
		}
		out.DurationMs = new(x.FinishedAt.Sub(start).Milliseconds())
	}
	return out
}

// findTaskRun reads the task run {id}.
func findTaskRun(r *http.Request, tx pgx.Tx) (store.TaskRunDetail, error) {
	d, err := store.FindTaskRun(r.Context(), tx, r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		return d, errNotFound("task run")
	}
	return d, err
}

func (s *Server) getTaskRun(w http.ResponseWriter, r *http.Request, t *accountScope) error {
	var d store.TaskRunDetail
	if err := s.read(r.Context(), t, func(tx pgx.Tx) error {
		var err error
		d, err = findTaskRun(r, tx)
		return err
	}); err != nil {
		return err
	}
	out, err := taskRunDetail(d)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// taskRunDetail decodes the records d stores in the shapes the task worker
// writes them: Proposed as tasks.Answer, Applied as store.TaskRunApplied
// and Dropped as tasks.Drop.
func taskRunDetail(d store.TaskRunDetail) (TaskRunDetail, error) {
	out := TaskRunDetail{
		Run: taskRun(d.TaskRunRow), Fields: map[string]json.RawMessage{}, Notes: d.Notes, ModelCalls: d.ModelCalls,
		CostUSD: d.CostUSD, Tokens: TokenCounts{Input: d.InputTokens, Output: d.OutputTokens},
	}
	if e := d.Event; e != nil {
		out.Event = &TaskEvent{
			Forge: e.Forge, Event: e.Event, RawEvent: e.RawEvent, Action: e.Action, Sender: e.Sender, Delivery: e.Delivery,
			ReceivedAt: e.ReceivedAt,
		}
	}
	if err := decodeRecord("fields", d.Fields, &out.Fields); err != nil {
		return out, err
	}
	if d.Proposed != nil {
		var a tasks.Answer
		if err := decodeRecord("proposed", d.Proposed, &a); err != nil {
			return out, err
		}
		out.Proposed = &TaskAnswer{
			Summary: a.Summary, Comment: a.Comment, AddLabels: a.Labels.Add, RemoveLabels: a.Labels.Remove,
			State: a.State, Assignees: a.Assignees, Reviewers: a.Reviewers, Inline: taskInlines(a.Inline),
		}
	}
	if d.Applied != nil {
		var a store.TaskRunApplied
		if err := decodeRecord("applied", d.Applied, &a); err != nil {
			return out, err
		}
		out.Applied = &TaskApplied{
			AddLabels: a.AddLabels, RemoveLabels: a.RemoveLabels, State: a.State, Assignees: a.Assignees,
			Reviewers: a.Reviewers, Inline: taskInlines(a.Inline), Comment: a.Comment,
		}
	}
	var dropped []tasks.Drop
	if err := decodeRecord("dropped", d.Dropped, &dropped); err != nil {
		return out, err
	}
	out.Dropped = make([]TaskDrop, len(dropped))
	for i, x := range dropped {
		out.Dropped[i] = TaskDrop{Action: x.Action, Value: x.Value, Reason: x.Reason}
	}
	return out, nil
}

// decodeRecord decodes one of a task run's JSON records into v, leaving v
// as it is for a record never written.
func decodeRecord(what string, raw json.RawMessage, v any) error {
	if raw == nil {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("webapi: decode task run %s: %w", what, err)
	}
	return nil
}

func taskInlines(in []tasks.Inline) []TaskInline {
	out := make([]TaskInline, len(in))
	for i, c := range in {
		out[i] = TaskInline{Path: c.Path, Line: c.Line, EndLine: c.EndLine, Body: c.Body}
	}
	return out
}

func (s *Server) getTaskRunTranscript(w http.ResponseWriter, r *http.Request, t *accountScope) error {
	ctx := r.Context()
	var rows []transcript.StoredRow
	if err := s.read(ctx, t, func(tx pgx.Tx) error {
		// The run must be the account's: an empty transcript and a missing
		// run are different answers.
		d, err := findTaskRun(r, tx)
		if err != nil {
			return err
		}
		rows, err = store.TaskRunModelCalls(ctx, tx, d.ID)
		return err
	}); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, transcriptOf(transcript.Rebuild(rows)))
	return nil
}

// getRepoTasks resolves the tasks that run for {owner}/{repo}: the admin's
// settings with the .kritique.yaml a task dispatch last read at the default
// branch tip applied, or, before any dispatch, the one the last review
// read.
func (s *Server) getRepoTasks(w http.ResponseWriter, r *http.Request, t *accountScope) error {
	ctx := r.Context()
	var row store.RepoRow
	var dispatched *store.TaskConfigRow
	var reviewed *store.RepoFileRow
	if err := s.read(ctx, t, func(tx pgx.Tx) error {
		var err error
		if row, err = findRepo(ctx, tx, r); err != nil {
			return err
		}
		tc, err := store.RepoTaskConfig(ctx, tx, row.ID)
		if err == nil {
			dispatched = &tc
			return nil
		}
		if none, err := missing(err); !none {
			return err
		}
		f, err := store.LastRepoFile(ctx, tx, row.ID)
		if none, err := missing(err); none || err != nil {
			return err
		}
		reviewed = &f
		return nil
	}); err != nil {
		return err
	}
	out := RepoTasks{}
	var doc *string
	switch {
	case dispatched != nil:
		out.Source, out.Commit, out.ResolvedAt, doc = TasksSourceDefaultBranch, dispatched.Commit, &dispatched.ResolvedAt, dispatched.Doc
	case reviewed != nil:
		out.Source, out.Commit, doc = TasksSourceLastReview, reviewed.Commit, reviewed.Doc
	}
	out.Found = doc != nil
	settings := t.file.Settings(t.account, row.FullName)
	out.Enabled = settings.TaskBounds.Enabled
	out.Tasks, out.Notes, out.Ignored = repoTasks(settings, doc)
	writeJSON(w, http.StatusOK, out)
	return nil
}

// repoTasks resolves the tasks that run from the admin's settings and the
// repository's .kritique.yaml, nil when there is none. A file that does not
// parse leaves the admin's tasks, as it does for a run, and ignored says
// why.
func repoTasks(settings configfile.Settings, file *string) (defs []TaskDef, notes []TaskNote, ignored string) {
	var doc []byte
	if file != nil {
		doc = []byte(*file)
	}
	m, err := repoconfig.Merge(doc, settings)
	if err != nil {
		ignored = err.Error()
	}
	defs = make([]TaskDef, len(m.Tasks))
	for i, task := range m.Tasks {
		d := TaskDef{Name: task.Name, Source: TaskFromRepository, When: task.When, Mode: task.RunMode()}
		if slices.ContainsFunc(settings.Tasks, func(o tasks.Task) bool { return o.Name == task.Name }) {
			d.Source = TaskFromFile
		}
		for _, tr := range task.On {
			d.Triggers = append(d.Triggers, tr.Names()...)
		}
		a := task.Actions
		for _, k := range []struct {
			kind string
			set  bool
		}{
			{tasks.ActionComment, a.Comment != nil && a.Comment.PostMode() != tasks.CommentNone},
			{tasks.ActionLabels, a.Labels != nil},
			{tasks.ActionState, a.State != nil},
			{tasks.ActionAssign, a.Assign != nil},
			{tasks.ActionReviewers, a.Reviewers != nil},
			{tasks.ActionInlineComments, a.InlineComments != nil},
		} {
			if k.set {
				d.Actions = append(d.Actions, k.kind)
			}
		}
		defs[i] = d
	}
	notes = make([]TaskNote, len(m.TaskNotes))
	for i, n := range m.TaskNotes {
		notes[i] = TaskNote{Task: n.Task, What: n.What, Reason: n.Reason}
	}
	return defs, notes, ignored
}
