// Package jobs defines the River job arguments that ingest, the leader and
// the dashboard enqueue and the worker consumes. Uniqueness lives here because it is the contract
// between the two: a review is unique per head SHA so no push is ever lost,
// a follow-up per comment, a thread change per thread and state, an index
// run per target commit.
package jobs

import (
	"strings"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Queue names, one per job kind so the worker can bound each separately.
const (
	QueueReview   = "review"
	QueueFollowUp = "followup"
	QueueIndex    = "index"
	QueueTask     = "task"
)

// TriggerManual is the Trigger a human-requested re-run carries. It is the
// only trigger the worker lets bypass the bot-author patch-id skip.
const TriggerManual = "manual"

// Settles reports whether a review started by trigger waits out the
// repository's settle time first: only a new head (a push, or one the
// poller found) does, since an open, reopen or draft transition has no
// earlier head to supersede.
func Settles(trigger string) bool { return trigger == "synchronize" || trigger == "poll" }

// LabelChange reports whether trigger is a label added to or removed from
// the pull request: the head is the one already judged, so only a filter
// that reads labels can decide differently.
func LabelChange(trigger string) bool { return trigger == "labeled" || trigger == "unlabeled" }

// TriggerReindex is the Trigger EnqueueReindex gives a forced full reindex,
// as opposed to the worker-internal onboard and push triggers.
const TriggerReindex = "reindex"

// Index job triggers the service itself sets.
const (
	TriggerOnboard = "onboard"
	TriggerPush    = "push"
)

// Index job priorities, highest first: River always fetches a higher
// priority first, so an onboarding wave never delays keeping an indexed
// repository current.
const (
	indexPriorityUpdate  = 1
	indexPriorityReindex = 2
	indexPriorityOnboard = 4
)

// indexAttempts bounds an index job's tries: transient failures (a fetch
// timeout, a runner killed at its deadline) get River's backoff, and an
// onboarding that fails every try is offered again later.
const indexAttempts = 3

// reviewAttempts bounds a review, follow-up or thread job's tries. With
// River's backoff the last comes over an hour after the first, which
// outlasts a restart or a database failover; a job that fails every try is
// failing on its input, and each further try would only repeat its failed
// review and commit status.
const reviewAttempts = 8

// LiveStates are the states a job is in while queued or running: what a
// job's unique key spans, and what counts as a job still to come.
var LiveStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
	rivertype.JobStateRunning, rivertype.JobStateScheduled,
}

// LiveStatesSQL is LiveStates as a SQL list, for a state IN (...) clause.
func LiveStatesSQL() string {
	quoted := make([]string, len(LiveStates))
	for i, s := range LiveStates {
		quoted[i] = "'" + string(s) + "'"
	}
	return strings.Join(quoted, ", ")
}

// ReviewArgs reviews one head of one pull request.
type ReviewArgs struct {
	AccountID    string `json:"account_id"     river:"unique"`
	RepositoryID string `json:"repository_id" river:"unique"`
	Number       int    `json:"number"        river:"unique"`
	HeadSHA      string `json:"head_sha"      river:"unique"`
	// Trigger is why: opened, synchronize, reopened, ready_for_review, poll,
	// labeled, unlabeled, manual.
	Trigger string `json:"trigger"`
	// Request distinguishes one manual re-run, or one label change, from
	// another. River dedupes by the river:"unique" fields, so every other
	// trigger leaves it empty and dedupes on account, repository, number
	// and head, and these two set a fresh value (a UUID) so the job is never
	// deduped against a prior run of the same head, including another of
	// its kind.
	Request string `json:"request,omitempty" river:"unique"`
}

// Kind implements river.JobArgs.
func (ReviewArgs) Kind() string { return "review" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (ReviewArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueReview, MaxAttempts: reviewAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// FollowUpArgs answers one comment that addressed the bot.
type FollowUpArgs struct {
	AccountID    string `json:"account_id"`
	RepositoryID string `json:"repository_id"`
	Number       int    `json:"number"`
	CommentID    int64  `json:"comment_id" river:"unique"`
	Inline       bool   `json:"inline"`
}

// Kind implements river.JobArgs.
func (FollowUpArgs) Kind() string { return "followup" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (FollowUpArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueFollowUp, MaxAttempts: reviewAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// ThreadArgs applies a review thread someone resolved or unresolved to the
// finding it holds.
type ThreadArgs struct {
	AccountID    string `json:"account_id"`
	RepositoryID string `json:"repository_id"`
	Number       int    `json:"number"`
	// CommentID is the inline comment that opened the thread.
	CommentID int64 `json:"comment_id" river:"unique"`
	// Resolved is the thread's state now.
	Resolved bool `json:"resolved" river:"unique"`
	// Outdated is whether a push had changed the lines the comment was made
	// on by the time the thread changed state.
	Outdated bool `json:"outdated"`
	// Sender is who changed it.
	Sender string `json:"sender"`
}

// Kind implements river.JobArgs.
func (ThreadArgs) Kind() string { return "thread" }

// InsertOpts implements river.JobArgsWithInsertOpts. A job is unique per
// thread and state while queued or running: a thread resolved and then
// unresolved is two jobs, the same change delivered twice is one, and a
// thread resolved again later is a new job.
func (ThreadArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: QueueFollowUp, MaxAttempts: reviewAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: LiveStates},
	}
}

// IndexArgs builds or advances a repository's index to its default branch
// tip, whatever the tip is when the job runs.
type IndexArgs struct {
	AccountID    string `json:"account_id"`
	RepositoryID string `json:"repository_id" river:"unique"`
	// CommitSHA is the commit a push moved the default branch to, for the
	// record only: a burst of pushes needs one job, not one each, so it is
	// not part of the unique key.
	CommitSHA string `json:"commit_sha"`
	// Trigger is why: TriggerOnboard, TriggerPush or TriggerReindex.
	Trigger string `json:"trigger"`
	// Full forces a full rebuild even when the active generation already
	// covers the tip. It is part of the unique key, so a forced rebuild is
	// queued beside an update rather than folded into it, and dedupes only
	// onto another forced rebuild.
	Full bool `json:"full,omitempty" river:"unique"`
}

// Kind implements river.JobArgs.
func (IndexArgs) Kind() string { return "index" }

// InsertOpts implements river.JobArgsWithInsertOpts. A job is unique per
// repository while queued or running (River requires running in the set):
// a push while the repository's job is running is absorbed by it, and the
// worker indexes the tip again when it finds the branch moved.
func (a IndexArgs) InsertOpts() river.InsertOpts {
	priority := indexPriorityUpdate
	switch a.Trigger {
	case TriggerReindex:
		priority = indexPriorityReindex
	case TriggerOnboard:
		priority = indexPriorityOnboard
	}
	return river.InsertOpts{
		Queue: QueueIndex, Priority: priority, MaxAttempts: indexAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: LiveStates},
	}
}

// taskAttempts bounds a task job's tries: a task writes to the forge, so a
// run that keeps failing stops rather than retrying for days.
const taskAttempts = 5

// TaskDispatchArgs resolves the tasks one delivery may run, stored as the
// task event EventID, and enqueues a TaskArgs for each that matches: ingest
// stays cheap, since resolving reads the repository's .kritique.yaml
// through the forge.
type TaskDispatchArgs struct {
	AccountID    string `json:"account_id"`
	RepositoryID string `json:"repository_id"`
	EventID      string `json:"event_id" river:"unique"`
}

// Kind implements river.JobArgs.
func (TaskDispatchArgs) Kind() string { return "task_dispatch" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (TaskDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueTask, MaxAttempts: taskAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// TaskArgs runs one task on one task event, with the task as the
// repository's .kritique.yaml at ConfigSHA, the default branch tip it was
// matched at, defines it.
type TaskArgs struct {
	AccountID    string `json:"account_id"`
	RepositoryID string `json:"repository_id"`
	EventID      string `json:"event_id"   river:"unique"`
	Task         string `json:"task"       river:"unique"`
	ConfigSHA    string `json:"config_sha"`
}

// Kind implements river.JobArgs.
func (TaskArgs) Kind() string { return "task" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (TaskArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueTask, MaxAttempts: taskAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}
