package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/perfectra1n/kritique/internal/configfile"
	"github.com/perfectra1n/kritique/internal/executor"
	"github.com/perfectra1n/kritique/internal/forge"
	"github.com/perfectra1n/kritique/internal/gateway"
	"github.com/perfectra1n/kritique/internal/jobs"
	"github.com/perfectra1n/kritique/internal/runner"
	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// agentic runs the task's agent in a runner over the commit the task was
// defined at, with the prompts and answer schema rendered here, and
// concludes on what it submitted as a single-mode run does on its answer.
// The runner reaches its model only through the gateway, holds only a
// read-only git token (see agenticSlot), and writes nothing to the forge:
// every write is the plan's, made here. An error before the runner starts
// is retried; once it has started, the run ends here.
func (r *taskRunner) agentic(ctx context.Context, data tasks.PromptData, labels []string) (store.TaskRunResult, error) {
	ref, fallback := r.models()
	system, user, err := r.prepared.RenderPrompt(data)
	if err != nil {
		return failed("", err), nil
	}
	schema, err := json.Marshal(r.prepared.AnswerSchema(labels))
	if err != nil {
		return failed("", fmt.Errorf("worker: encode answer schema: %w", err)), nil
	}
	// The caps are read under the model slot agenticSlot took, so
	// concurrent runs cannot all pass a cap of one.
	budget, capped, err := r.agentBudget(ctx)
	if err != nil {
		return store.TaskRunResult{}, err
	}
	if capped != "" {
		return skipped(capped), nil
	}
	return r.runAgent(ctx, ref, fallback, budget, r.taskPrompt(system, user, schema), labels)
}

// agenticSlot settles whether an agentic run can start and takes a slot on
// its model, before the run does anything billable: with every slot held
// the job is snoozed, as an agentic review is, and a snoozed job has spent
// nothing, not even an index search's embedding, when it comes back.
// release is nil when the run does not go on, for the result or error
// given; otherwise the caller holds the slot until the run ends.
func (r *taskRunner) agenticSlot(ctx context.Context) (store.TaskRunResult, func(), error) {
	if r.w.GatewayURL == "" || r.w.Executor == nil {
		return failed("", errors.New("worker: agentic tasks need the model gateway (KRITIQUE_GATEWAY_URL)")), nil, nil
	}
	ref, _ := r.models()
	if ref == "" {
		return skipped(noModel), nil, nil
	}
	if _, ok := r.file.Provider(r.account, ref.Provider()); !ok {
		return failed("", fmt.Errorf("worker: provider %q is not in the configuration", ref.Provider())), nil, nil
	}
	tok, err := r.client.ReadGitToken(ctx, r.owner, r.name)
	if errors.Is(err, forge.ErrNoReadToken) {
		return skipped(noReadToken), nil, nil
	}
	if err != nil {
		return store.TaskRunResult{}, nil, err
	}
	r.gitToken = tok
	slots := r.settings.Limits.Concurrency
	free, err := r.w.Store.SlotFree(ctx, r.account.ID(), string(ref), slots)
	if err != nil {
		return store.TaskRunResult{}, nil, err
	}
	if !free {
		return store.TaskRunResult{}, nil, r.snooze(string(ref))
	}
	l, err := r.w.Store.AcquireLease(ctx, r.account.ID(), string(ref), slots, r.jobID)
	if err != nil {
		return store.TaskRunResult{}, nil, err
	}
	return store.TaskRunResult{}, func() { r.w.releaseLease(ctx, r.logger, l, string(ref)) }, nil
}

// noReadToken is why an agentic task is skipped on a forge that has no
// read-only credential for its runner, whose agent reads untrusted input.
const noReadToken = "agentic tasks need a read-only git token"

// snooze puts the job back for later, backing off with each snooze, when
// every slot of the task's model is held.
func (r *taskRunner) snooze(modelKey string) error {
	d := store.Backoff(r.snoozes, snoozeMin, snoozeMax)
	r.logger.Info("task snoozed: every model slot is held", "model", modelKey, "snoozes", r.snoozes+1, "for", d.Round(time.Second))
	return river.JobSnooze(d)
}

// agentBudget is the tokens the task's agent may spend, cut to what is
// left of the account's monthly cap, or the cap that stops it.
func (r *taskRunner) agentBudget(ctx context.Context) (int64, string, error) {
	maxTokens := r.settings.Agent.MaxTokens
	if t := r.task.Agent.MaxTokens; t != nil {
		maxTokens = *t
	}
	limit := r.settings.Limits.TokensPerMonth
	if limit <= 0 {
		return maxTokens, "", nil
	}
	u, err := r.w.Store.AccountUsage(ctx, r.account.ID())
	if err != nil {
		return 0, "", fmt.Errorf("worker: read caps: %w", err)
	}
	budget, capped := agentBudget(maxTokens, limit, u.Tokens)
	return budget, capped, nil
}

// taskPrompt is what the runner needs beyond the checkout: the rendered
// prompts and schema, the tools and commands the task uses, and the
// context sources only a checkout can gather, within what the worker's
// sources left of the context budget.
func (r *taskRunner) taskPrompt(system, user string, schema []byte) *runner.TaskPrompt {
	t := r.task
	p := &runner.TaskPrompt{
		Name: t.Name, System: system, User: user, Schema: schema, SourceBytes: taskSourceBytes, ContextBytes: r.contextLeft,
	}
	tools := t.Agent.Tools
	if tools == nil {
		tools = r.settings.TaskBounds.Tools
	}
	for _, tool := range tools {
		if slices.Contains(runner.TaskTools, tool) {
			p.Tools = append(p.Tools, tool)
		}
	}
	// The run tool takes both the tool and commands for it. A nil
	// agent.commands, which chooseTask also leaves when it drops a list
	// out of bounds, offers no command rather than the admin's own.
	if slices.Contains(tools, runTool) {
		p.Run = t.Agent.Commands
	}
	for _, f := range t.Context.Files {
		if f.Glob != "" {
			p.Files = append(p.Files, runner.TaskFiles{Glob: f.Glob, Max: f.Max})
		}
	}
	for _, c := range t.Context.Commands {
		p.Commands = append(p.Commands, runner.TaskCommand{Name: c.Name, Argv: strings.Fields(c.Run)})
	}
	return p
}

// runTool is the agent tool that runs the task's agent.commands.
const runTool = "run"

// taskCommands are the binaries a task's runner may execute: those its
// agent's run tool offers and those its context commands run.
func taskCommands(p *runner.TaskPrompt) []string {
	commands := slices.Clone(p.Run)
	for _, c := range p.Commands {
		commands = append(commands, c.Argv[0])
	}
	slices.Sort(commands)
	return slices.Compact(commands)
}

// taskAgent is the agent settings an agentic task's runner gets: the
// repository's, with each limit the task sets in their place.
func taskAgent(settings configfile.AgentSettings, t *tasks.Task) configfile.AgentSettings {
	if v := t.Agent.MaxSteps; v != nil {
		settings.MaxSteps = *v
	}
	if v := t.Agent.MaxToolOutputBytes; v != nil {
		settings.MaxToolOutputBytes = *v
	}
	if v := t.Agent.Timeout; v != nil {
		settings.Timeout = *v
	}
	return settings
}

// runAgent starts the runner, waits for it under supervision and concludes
// on the agent's answer. Its error, retried, is one from before the
// runner starts.
func (r *taskRunner) runAgent(
	ctx context.Context, ref, fallback configfile.ModelRef, budget int64, prompt *runner.TaskPrompt, labels []string,
) (store.TaskRunResult, error) {
	accountID := r.account.ID()
	var runID string
	err := r.w.Store.WithAccount(ctx, accountID, func(tx pgx.Tx) error {
		var err error
		runID, err = store.InsertTaskRunner(ctx, tx, accountID, r.run.ID, r.jobID)
		return err
	})
	if err != nil {
		return store.TaskRunResult{}, err
	}
	logger := r.logger.With("runner_run", runID)
	deadline, resources := r.file.RunnerFor()
	agent := taskAgent(r.settings.Agent, r.task)
	deadline = agentDeadline(deadline, agent.Timeout)
	commands := taskCommands(prompt)
	spec := runner.Spec{
		Version: runner.SpecVersion, Kind: runner.KindTask, RunID: runID, CloneURL: r.client.CloneURL(r.owner, r.name),
		Head: r.args.ConfigSHA, Ignore: r.settings.Ignore,
		Model: &runner.ModelEndpoint{GatewayURL: r.w.GatewayURL, Model: gateway.ModelName},
		Agent: &runner.AgentLimits{
			MaxSteps: agent.MaxSteps, MaxToolOutputBytes: agent.MaxToolOutputBytes, MaxTokens: budget,
			TimeoutSeconds: int(agent.Timeout / time.Second), Commands: commands, CommandTimeoutSeconds: int(agent.CommandTimeout / time.Second),
		},
		Task: prompt,
	}
	if err := spec.Validate(); err != nil {
		r.runnerNotStarted(ctx, runID, err)
		return failed("", err), nil
	}
	// The token is minted last but for the mark, whose failure revokes it.
	token, err := r.w.Store.MintGatewayToken(ctx, store.GatewayGrant{
		RunID: runID, AccountID: accountID, TaskRunID: r.run.ID, RepositoryID: r.args.RepositoryID,
		Model: string(ref), Fallback: string(fallback), Effort: string(r.settings.Models.Effort), Budget: budget,
	}, time.Now().Add(deadline+r.w.GatewayTokenTTL))
	if err != nil {
		r.runnerNotStarted(ctx, runID, err)
		return store.TaskRunResult{}, err
	}
	// From here a crash leaves the run to be failed, not to start a
	// second runner.
	if err := r.w.Store.WithAccount(ctx, accountID, func(tx pgx.Tx) error {
		return store.StartTaskRunner(ctx, tx, r.run.ID)
	}); err != nil {
		r.w.revokeGatewayTokens(ctx, logger, runID)
		r.runnerNotStarted(ctx, runID, err)
		return store.TaskRunResult{}, err
	}
	logger.Info("task runner starting", "model", ref, "budget", budget, "commands", commands)
	sup := runSupervision(r.w.Store, accountID, runID, "", "", r.w.superviseEvery, logger)
	out := r.w.runRunner(ctx, r.w.Executor, sup, executor.Spec{
		Labels:      runnerLabels(r.account.Key(), r.repo.name, jobs.QueueTask, 0),
		Annotations: runnerAnnotations(r.jobID, r.args.ConfigSHA),
		Job:         spec,
		Secrets:     runner.Secrets{GitToken: r.gitToken, GatewayToken: token},
		Deadline:    deadline,
		Resources:   resources,
		Tools:       r.file.ToolsFor(commands),
	}, accountID, runID, ref, r.w.rowWait, logger)
	ctx, cancel := detach(ctx)
	defer cancel()
	if err := recordRun(ctx, r.w.Store, r.w.Metrics, r.account.Key(), accountID, runID, jobs.QueueTask, out.res); err != nil {
		logger.Error("runner run not recorded", "error", err)
	}
	run := out.agent
	if run != nil {
		r.notes = append(r.notes, run.Notes...)
	}
	switch {
	case out.agentErr != nil:
		return failed(string(ref), out.agentErr), nil
	case run == nil && errors.Is(out.cause, errHeartbeatLost):
		return failed(string(ref), errors.New("runner heartbeat lost")), nil
	case run == nil && out.res.Err != nil:
		return failed(string(ref), out.res.Err), nil
	case run == nil:
		return failed(string(ref), errors.New("worker: the runner wrote no agent run")), nil
	}
	logger.Info("agent answered", "stop", run.StopReason, "steps", run.Steps, "model", run.Model, "input_tokens", run.Usage.Prompt(),
		"output_tokens", run.Usage.Output, "cost_usd", run.CostUSD)
	if err := stopError(*run); err != nil {
		return failed(run.Model, err), nil
	}
	// The gateway charged every step as it served it; marking the run
	// answered before any write keeps a retry from running it again.
	if err := r.charge(ctx, true); err != nil {
		return failed(run.Model, err), nil
	}
	return r.conclude(ctx, run.Result, run.Model, labels), nil
}

// runnerNotStarted ends a runner run whose runner never started, for
// cause.
func (r *taskRunner) runnerNotStarted(ctx context.Context, runID string, cause error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	if err := failRun(ctx, r.w.Store, r.account.ID(), runID, cause.Error()); err != nil {
		r.logger.Warn("runner run not ended", "error", err)
	}
}
