package worker

import (
	"time"

	"github.com/riverqueue/river"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/jobs"
	"github.com/perfectra1n/kritique/internal/jobtimeout"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// Timeout implements river.Worker: the runner's deadline, which the
// agent's timeout may lengthen, plus the lease wait and the publish phase
// around it.
func (w *Review) Timeout(job *river.Job[jobs.ReviewArgs]) time.Duration {
	return agentRunTimeout(w.Current.Get(), job.Args.AccountID, job.Args.RepositoryID, true)
}

// agentRunTimeout is the timeout of a job that runs an agent in a runner
// for the repository: a review, or a follow-up. A review may be split, each
// part with the agent's timeout, and its size is read only once the job
// runs, so its timeout allows for the most parts.
func agentRunTimeout(file *configfile.File, accountID, repositoryID string, review bool) time.Duration {
	account, _ := file.AccountByID(accountID)
	deadline, _ := file.RunnerFor()
	if account != nil {
		agent := repoSettings(file, account, repositoryID).Agent
		timeout := agent.Timeout
		if review {
			timeout = partsTimeout(agent.MaxParts, timeout)
		}
		deadline = agentDeadline(deadline, timeout)
	}
	return min(deadline+jobtimeout.LeaseWaitHeadroom+jobtimeout.PublishHeadroom, jobtimeout.MaxJobTimeout)
}

// Timeout implements river.Worker: the runner's deadline plus embedding
// and writing the generation, which waits on embedding leases.
func (w *Index) Timeout(*river.Job[jobs.IndexArgs]) time.Duration {
	deadline, _ := w.Current.Get().RunnerFor()
	return min(deadline+jobtimeout.IndexWriteHeadroom, jobtimeout.MaxJobTimeout)
}

// Timeout implements river.Worker: a follow-up's agent runs in a runner as
// a review's does, around the same lease wait and forge write-back.
func (w *FollowUp) Timeout(job *river.Job[jobs.FollowUpArgs]) time.Duration {
	return agentRunTimeout(w.Current.Get(), job.Args.AccountID, job.Args.RepositoryID, false)
}

// repoSettings resolves a repository's settings from its id, which a job
// carries instead of the name the configuration is keyed by. A repository
// the account does not list gets the account's settings, as in Settings.
func repoSettings(file *configfile.File, account *configfile.Account, repositoryID string) configfile.Settings {
	for _, r := range account.Repositories {
		if name := account.Name + "/" + r.Name; configfile.RepositoryID(account.ID(), name) == repositoryID {
			return file.Settings(account, name)
		}
	}
	return file.Settings(account, "")
}

// taskTimeout bounds a task job that runs no runner: a single-mode task's
// model call under its lease wait, its context sources and its writes.
const taskTimeout = jobtimeout.LeaseWaitHeadroom + jobtimeout.PublishHeadroom

// Timeout implements river.Worker: a single-mode task's model call and
// writes, or, where an agentic task may run, its runner's deadline, which
// the longest agent timeout of the repository's tasks may lengthen, with
// the same lease wait and write-back around it.
func (w *Task) Timeout(job *river.Job[jobs.TaskArgs]) time.Duration {
	if w.timeout > 0 {
		return w.timeout
	}
	timeout := taskTimeout
	file := w.Current.Get()
	if account, ok := file.AccountByID(job.Args.AccountID); ok && w.GatewayURL != "" {
		deadline, _ := file.RunnerFor()
		agentTimeout := taskAgentTimeout(repoSettings(file, account, job.Args.RepositoryID))
		timeout = max(timeout, agentDeadline(deadline, agentTimeout)+jobtimeout.LeaseWaitHeadroom+jobtimeout.PublishHeadroom)
	}
	return min(timeout, jobtimeout.MaxJobTimeout)
}

// taskAgentTimeout is the longest agent timeout an agentic task on a
// repository with settings may run with: a repository's own tasks are held
// to the admin's agent timeout, and the admin's own tasks may set theirs.
// Whether an agentic task runs at all is only known once the repository's
// .kritique.yaml is read, so one is always allowed for.
func taskAgentTimeout(settings configfile.Settings) time.Duration {
	timeout := settings.Agent.Timeout
	for _, t := range settings.Tasks {
		if t.RunMode() == tasks.ModeAgentic && t.Agent.Timeout != nil {
			timeout = max(timeout, *t.Agent.Timeout)
		}
	}
	return timeout
}
