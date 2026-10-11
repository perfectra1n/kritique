//go:build integration

package store

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// taskFixture is one account's repository and connection a task test
// writes under.
type taskFixture struct {
	s                                  *Store
	alpha, beta, connID, repoID, event string
}

func newTaskFixture(t *testing.T) *taskFixture {
	t.Helper()
	s := openStore(t)
	ctx := t.Context()
	if err := s.ApplyConfig(ctx, parse(t, twoAccounts)); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	f := &taskFixture{s: s, alpha: accountID(t, s, "alpha"), beta: accountID(t, s, "beta")}
	if err := s.owner.QueryRow(ctx, `SELECT id FROM connections WHERE name = 'alpha-bot'`).Scan(&f.connID); err != nil {
		t.Fatal(err)
	}
	if err := s.owner.QueryRow(ctx, `SELECT id FROM repositories WHERE account_id = $1 AND name = 'alpha/one'`, f.alpha).
		Scan(&f.repoID); err != nil {
		t.Fatal(err)
	}
	return f
}

// as runs fn in a transaction scoped to account, failing the test on its
// error.
func (f *taskFixture) as(t *testing.T, account string, fn func(tx pgx.Tx) error) {
	t.Helper()
	if err := f.s.WithAccount(t.Context(), account, fn); err != nil {
		t.Fatal(err)
	}
}

// TestTaskEventsAndRuns walks a task event and its run through the store:
// a redelivery is stored once, a run is queued once per event and task,
// started, charged, finished and read back as the dashboard reads it, and
// another account sees none of it.
func TestTaskEventsAndRuns(t *testing.T) {
	f := newTaskFixture(t)
	ctx := t.Context()
	delivery := "task-" + time.Now().Format(time.RFC3339Nano)
	ev := TaskEvent{
		AccountID: f.alpha, ConnectionID: f.connID, RepositoryID: f.repoID, Forge: "github", Event: "issue", RawEvent: "issues",
		Action: "opened", Sender: "devin", Delivery: delivery, SubjectKind: "issue", SubjectNumber: 7,
		Payload: json.RawMessage(`{"action":"opened"}`),
	}
	var again string
	f.as(t, f.alpha, func(tx pgx.Tx) error {
		var err error
		if f.event, err = InsertTaskEvent(ctx, tx, ev); err != nil {
			return err
		}
		again, err = InsertTaskEvent(ctx, tx, ev)
		return err
	})
	if f.event == "" || again != "" {
		t.Fatalf("event ids %q then %q; want one, then none for the redelivery", f.event, again)
	}
	runID := queueAndCharge(t, f, delivery)
	finishAndRead(t, f, runID, delivery)
	sweepTaskEvent(t, f, runID)
}

// queueAndCharge queues a run of triage on f's event twice, starts and
// charges it, and returns its id.
func queueAndCharge(t *testing.T, f *taskFixture, delivery string) string {
	t.Helper()
	ctx := t.Context()
	run := TaskRun{
		AccountID: f.alpha, RepositoryID: f.repoID, Task: "triage", EventID: f.event, SubjectKind: "issue", SubjectNumber: 7,
		Trigger: "issue.opened", Mode: "single", ConfigSHA: "abc",
	}
	var loaded TaskRun
	var event TaskEvent
	f.as(t, f.alpha, func(tx pgx.Tx) error {
		for _, want := range []bool{true, false} {
			inserted, err := QueueTaskRun(ctx, tx, run)
			if err != nil {
				return err
			}
			if inserted != want {
				t.Errorf("queue inserted = %v, want %v", inserted, want)
			}
		}
		var err error
		if loaded, err = LoadTaskRun(ctx, tx, f.event, "triage"); err != nil {
			return err
		}
		if err := StartTaskRun(ctx, tx, loaded.ID); err != nil {
			return err
		}
		usage := []TaskUsage{{Model: "m", Input: 10, Output: 5, CostUSD: 0.01}}
		if err := ChargeTaskRun(ctx, tx, f.alpha, f.repoID, loaded.ID, usage, true); err != nil {
			return err
		}
		event, err = LoadTaskEvent(ctx, tx, f.event)
		return err
	})
	if loaded.Status != TaskQueued || loaded.Trigger != "issue.opened" || event.Delivery != delivery ||
		string(event.Payload) != `{"action": "opened"}` {
		t.Fatalf("run %+v, event %+v", loaded, event)
	}
	f.as(t, f.beta, func(tx pgx.Tx) error {
		if _, err := LoadTaskRun(ctx, tx, f.event, "triage"); !errors.Is(err, ErrTaskRunGone) {
			t.Errorf("another account loaded the run: %v", err)
		}
		return nil
	})
	return loaded.ID
}

// finishAndRead finishes the run runID and reads it back as the dashboard
// lists and shows it.
func finishAndRead(t *testing.T, f *taskFixture, runID, delivery string) {
	t.Helper()
	ctx := t.Context()
	applied, _ := json.Marshal(TaskRunApplied{AddLabels: []string{"bug"}})
	res := TaskRunResult{
		Status: TaskSucceeded, Model: "p/m", Fields: json.RawMessage(`{"area":"api"}`), Applied: applied,
		Dropped: json.RawMessage(`[{"action":"state","value":"close","reason":"no"}]`), CommentID: 42, Notes: []string{"context x cut"},
	}
	var detail TaskRunDetail
	var list []TaskRunRow
	f.as(t, f.alpha, func(tx pgx.Tx) error {
		if err := FinishTaskRun(ctx, tx, runID, res); err != nil {
			return err
		}
		var err error
		if list, _, err = ListTaskRuns(ctx, tx, TaskRunFilter{RepositoryID: f.repoID, Status: TaskSucceeded}, Page{Limit: 10}); err != nil {
			return err
		}
		if _, _, err := ListTaskRuns(ctx, tx, TaskRunFilter{Status: "bogus"}, Page{Limit: 10}); !errors.Is(err, ErrFilter) {
			t.Errorf("a bogus status filter = %v, want ErrFilter", err)
		}
		detail, err = FindTaskRun(ctx, tx, runID)
		return err
	})
	if len(list) == 0 || list[0].ID != runID || list[0].DroppedCount != 1 || list[0].Repository != "alpha/one" {
		t.Fatalf("list = %+v", list)
	}
	if detail.Status != TaskSucceeded || detail.Event == nil || detail.Event.Delivery != delivery || detail.CommentID == nil ||
		*detail.CommentID != 42 || string(detail.Fields) != `{"area": "api"}` || detail.Proposed != nil {
		t.Fatalf("detail = %+v", detail)
	}
	var answered *time.Time
	var notes []string
	var usage int
	if err := f.s.owner.QueryRow(ctx, `SELECT answered_at, notes, (SELECT count(*) FROM usage WHERE role = 'task' AND repository_id = $2)
		FROM task_runs WHERE id = $1`, runID, f.repoID).Scan(&answered, &notes, &usage); err != nil {
		t.Fatal(err)
	}
	if answered == nil || len(notes) != 1 || usage == 0 {
		t.Fatalf("answered %v, notes %v, usage rows %d", answered, notes, usage)
	}
}

// sweepTaskEvent sweeps f's event, which leaves the run runID's record
// without it.
func sweepTaskEvent(t *testing.T, f *taskFixture, runID string) {
	t.Helper()
	ctx := t.Context()
	if _, err := f.s.owner.Exec(ctx, `UPDATE task_events SET received_at = now() - interval '2 days' WHERE id = $1`, f.event); err != nil {
		t.Fatal(err)
	}
	if n, err := f.s.SweepTaskEvents(ctx, 24*time.Hour); err != nil || n < 1 {
		t.Fatalf("swept %d, %v", n, err)
	}
	var detail TaskRunDetail
	f.as(t, f.alpha, func(tx pgx.Tx) error {
		if _, err := LoadTaskEvent(ctx, tx, f.event); !errors.Is(err, ErrTaskEventGone) {
			t.Errorf("swept event = %v", err)
		}
		var err error
		detail, err = FindTaskRun(ctx, tx, runID)
		return err
	})
	if detail.Event != nil || detail.Status != TaskSucceeded {
		t.Fatalf("run after the sweep = %+v", detail)
	}
}

// TestTaskGatewayGrant checks an agentic task's runner run and its gateway
// token, which names the task run and no review.
func TestTaskGatewayGrant(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	if err := s.ApplyConfig(ctx, parse(t, twoAccounts)); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	alpha := accountID(t, s, "alpha")
	g := GatewayGrant{AccountID: alpha, Model: "openrouter/acme/large", Budget: 1000}
	if err := s.WithAccount(ctx, alpha, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM repositories WHERE name = 'alpha/one'`).Scan(&g.RepositoryID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO task_runs (account_id, repository_id, task, mode, status)
			VALUES ($1, $2, 'triage', 'agentic', 'running') RETURNING id`, alpha, g.RepositoryID).Scan(&g.TaskRunID); err != nil {
			return err
		}
		var err error
		if g.RunID, err = InsertTaskRunner(ctx, tx, alpha, g.TaskRunID, 0); err != nil {
			return err
		}
		return StartTaskRunner(ctx, tx, g.TaskRunID)
	}); err != nil {
		t.Fatal(err)
	}
	token, err := s.MintGatewayToken(ctx, g, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.LookupGatewayToken(ctx, token); err != nil || got != g {
		t.Fatalf("task grant = %+v, %v; want %+v", got, err, g)
	}
	var linked string
	var started *time.Time
	if err := s.owner.QueryRow(ctx, `SELECT runner_run_id::text, runner_started_at FROM task_runs WHERE id = $1`, g.TaskRunID).
		Scan(&linked, &started); err != nil || linked != g.RunID || started == nil {
		t.Fatalf("task run links %q started %v, %v; want %q", linked, started, err, g.RunID)
	}
	if err := s.RevokeGatewayTokens(ctx, g.RunID); err != nil {
		t.Fatal(err)
	}
	if err := s.WithAccount(ctx, alpha, func(tx pgx.Tx) error {
		return FailRunnerRun(ctx, tx, g.RunID, "ended by the test")
	}); err != nil {
		t.Fatal(err)
	}
}

// TestRepoTaskConfig checks the default branch's .kritique.yaml a dispatch
// records is kept per commit.
func TestRepoTaskConfig(t *testing.T) {
	s := openStore(t)
	ctx := t.Context()
	if err := s.ApplyConfig(ctx, parse(t, twoAccounts)); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	alpha := accountID(t, s, "alpha")
	var repoID string
	if err := s.owner.QueryRow(ctx, `SELECT id FROM repositories WHERE account_id = $1 AND name = 'alpha/two'`, alpha).Scan(&repoID); err != nil {
		t.Fatal(err)
	}
	var row TaskConfigRow
	if err := s.WithAccount(ctx, alpha, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE repositories SET task_config_sha = '', task_config_doc = NULL WHERE id = $1`, repoID); err != nil {
			return err
		}
		if _, err := RepoTaskConfig(ctx, tx, repoID); !errors.Is(err, ErrNotFound) {
			t.Errorf("before a dispatch = %v, want ErrNotFound", err)
		}
		if err := SaveRepoTaskConfig(ctx, tx, repoID, "sha1", []byte("tasks: []\n")); err != nil {
			return err
		}
		if err := SaveRepoTaskConfig(ctx, tx, repoID, "sha2", nil); err != nil {
			return err
		}
		var err error
		row, err = RepoTaskConfig(ctx, tx, repoID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if row.Commit != "sha2" || row.Doc != nil {
		t.Fatalf("config = %+v; want sha2 with no file", row)
	}
}
