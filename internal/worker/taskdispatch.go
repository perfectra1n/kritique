package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/perfectra1n/kritique/internal/forge"
	"github.com/perfectra1n/kritique/internal/jobs"
	"github.com/perfectra1n/kritique/internal/review"
	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/taskrun"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// taskRepo is the repository a task event concerns, as the store has it.
type taskRepo struct {
	id, name, defaultBranch string
	// configSHA is the default branch tip whose .kritique.yaml a dispatch
	// last recorded, "" before the first.
	configSHA string
}

func (r taskRepo) split() (owner, name string) {
	owner, name, _ = strings.Cut(r.name, "/")
	return owner, name
}

func loadTaskRepo(ctx context.Context, tx pgx.Tx, id string) (taskRepo, error) {
	r := taskRepo{id: id}
	err := tx.QueryRow(ctx, `SELECT name, default_branch, task_config_sha FROM repositories WHERE id = $1`, id).
		Scan(&r.name, &r.defaultBranch, &r.configSHA)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, river.JobCancel(fmt.Errorf("worker: repository %s is unknown", id))
	}
	if err != nil {
		return r, fmt.Errorf("worker: load repository: %w", err)
	}
	return r, nil
}

// repoConfigs caches .kritique.yaml by repository and commit: a commit's file
// never changes, and every event on a quiet default branch reads the same
// one.
type repoConfigs struct {
	mu   sync.Mutex
	docs map[string][]byte
}

// repoConfigsMax bounds the cache; past it, it starts over.
const repoConfigsMax = 512

// read returns the .kritique.yaml of owner/repo at sha, nil when it has none
// or one too large to use.
func (c *repoConfigs) read(ctx context.Context, client forge.Client, accountID, owner, repo, sha string) ([]byte, error) {
	key := accountID + "\x00" + strings.ToLower(owner+"/"+repo) + "\x00" + sha
	c.mu.Lock()
	doc, ok := c.docs[key]
	c.mu.Unlock()
	if ok {
		return doc, nil
	}
	doc, _, err := readRepoConfig(ctx, client, owner, repo, sha)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.docs == nil || len(c.docs) >= repoConfigsMax {
		c.docs = map[string][]byte{}
	}
	c.docs[key] = doc
	return doc, nil
}

// taskInput is the stored event as a task sees it, without its subject,
// which the caller reads fresh from the forge.
func taskInput(ev store.TaskEvent, repo taskRepo, defaultBranch string) tasks.Input {
	return tasks.Input{
		Forge: ev.Forge, Event: ev.Event, RawEvent: ev.RawEvent, Action: ev.Action, Sender: ev.Sender,
		Raw: taskrun.DecodeRaw(ev.Payload), Repo: taskrun.Repo(repo.name, defaultBranch),
	}
}

// matchTasks are the tasks of ts in fires. A task whose when fails to
// evaluate is left out and logged: its author's mistake must not stop the
// others.
func matchTasks(ts []tasks.Task, in tasks.Input, logger *slog.Logger) []tasks.Task {
	var out []tasks.Task
	for _, t := range ts {
		ok, err := t.Matches(in)
		if err != nil {
			logger.Warn("task condition failed", "task", t.Name, "error", err)
			continue
		}
		if ok {
			out = append(out, t)
		}
	}
	return out
}

// isBot reports whether sender is the connection's own bot, whose events
// never trigger a task: its label and comment writes would re-trigger the
// task that made them.
func isBot(sender, botLogin string) bool {
	return sender != "" && strings.EqualFold(sender, botLogin)
}

// TaskDispatch works task_dispatch jobs: it resolves the tasks a stored
// delivery runs, from the admin's settings and the repository's
// .kritique.yaml at the default branch tip, and enqueues one task job for
// each that matches.
type TaskDispatch struct {
	river.WorkerDefaults[jobs.TaskDispatchArgs]
	Base
	configs repoConfigs
}

// Work implements river.Worker.
func (w *TaskDispatch) Work(ctx context.Context, job *river.Job[jobs.TaskDispatchArgs]) error {
	args := job.Args
	file := w.Current.Get()
	account, err := w.account(file, args.AccountID)
	if err != nil {
		return err
	}
	var ev store.TaskEvent
	var repo taskRepo
	err = w.Store.WithAccount(ctx, args.AccountID, func(tx pgx.Tx) error {
		if ev, err = store.LoadTaskEvent(ctx, tx, args.EventID); err != nil {
			return err
		}
		repo, err = loadTaskRepo(ctx, tx, args.RepositoryID)
		return err
	})
	if errors.Is(err, store.ErrTaskEventGone) {
		return nil
	}
	if err != nil {
		return err
	}
	logger := w.Logger.With("account", account.Key(), "repository", repo.name, "event", ev.RawEvent, "action", ev.Action,
		"delivery", ev.Delivery)
	client, err := w.client(ctx, file, account, repo.name)
	if err != nil {
		return err
	}
	login, err := client.BotLogin(ctx)
	if err != nil {
		return err
	}
	if isBot(ev.Sender, login) {
		logger.Debug("task event from the bot itself ignored")
		return w.dropEvent(ctx, args)
	}
	owner, name := repo.split()
	sha, branch, err := client.BranchTip(ctx, owner, name, "")
	if err != nil {
		return err
	}
	doc, err := w.configs.read(ctx, client, account.ID(), owner, name, sha)
	if err != nil {
		return err
	}
	if sha != repo.configSHA {
		if err := w.Store.WithAccount(ctx, args.AccountID, func(tx pgx.Tx) error {
			return store.SaveRepoTaskConfig(ctx, tx, repo.id, sha, doc)
		}); err != nil {
			// The record only feeds the dashboard's task list; the
			// dispatch does not depend on it.
			logger.Warn("repository task config not recorded", "error", err)
		}
	}
	eff, notes := effective(file.Settings(account, repo.name), doc)
	for _, n := range notes {
		logger.Debug("repository configuration note", "note", n)
	}
	for _, n := range eff.TaskNotes {
		logger.Debug("task left out", "note", n.String())
	}
	in := taskInput(ev, repo, branch)
	if ev.SubjectNumber > 0 && len(eff.Tasks) > 0 {
		issue, err := client.Issue(ctx, owner, name, ev.SubjectNumber)
		if err != nil {
			return err
		}
		in.Subject = taskrun.Subject(issue, in.Raw)
	}
	matched := matchTasks(eff.Tasks, in, logger)
	if len(matched) == 0 {
		logger.Debug("no task matches the event")
		return w.dropEvent(ctx, args)
	}
	runs, queued := taskJobs(matched, args, ev, taskrun.Names(in), sha)
	queue := river.ClientFromContext[pgx.Tx](ctx)
	return w.Store.WithAccount(ctx, args.AccountID, func(tx pgx.Tx) error {
		for i, run := range runs {
			inserted, err := store.QueueTaskRun(ctx, tx, run)
			if err != nil {
				return err
			}
			if !inserted {
				continue
			}
			if _, err := queue.InsertTx(ctx, tx, queued[i], nil); err != nil {
				return fmt.Errorf("worker: enqueue task: %w", err)
			}
			logger.Info("task queued", "task", run.Task, "mode", run.Mode, "config_sha", review.ShortSHA(sha))
		}
		return nil
	})
}

// taskJobs are the queued run and the job of each matched task on the
// event args dispatches, whose event names are names, with the tasks as
// defined at sha.
func taskJobs(
	matched []tasks.Task, args jobs.TaskDispatchArgs, ev store.TaskEvent, names []string, sha string,
) ([]store.TaskRun, []jobs.TaskArgs) {
	var trigger string
	if len(names) > 0 {
		trigger = names[0]
	}
	runs := make([]store.TaskRun, len(matched))
	queued := make([]jobs.TaskArgs, len(matched))
	for i, t := range matched {
		runs[i] = store.TaskRun{
			AccountID: args.AccountID, RepositoryID: args.RepositoryID, Task: t.Name, EventID: args.EventID,
			SubjectKind: ev.SubjectKind, SubjectNumber: ev.SubjectNumber, Trigger: trigger, Mode: string(t.RunMode()), ConfigSHA: sha,
		}
		queued[i] = jobs.TaskArgs{AccountID: args.AccountID, RepositoryID: args.RepositoryID, EventID: args.EventID, Task: t.Name, ConfigSHA: sha}
	}
	return runs, queued
}

// dropEvent deletes a task event no task runs on.
func (w *TaskDispatch) dropEvent(ctx context.Context, args jobs.TaskDispatchArgs) error {
	return w.Store.WithAccount(ctx, args.AccountID, func(tx pgx.Tx) error {
		return store.DeleteTaskEvent(ctx, tx, args.EventID)
	})
}
