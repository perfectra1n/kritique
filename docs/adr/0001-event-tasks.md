# ADR-0001: event-driven tasks

- **Status:** Proposed
- **Date:** 2026-10-09
- **Replaces:** ADR-0012 of the earlier kritique, which predates this
  repository's configuration layout; [§2.9](#29-changes-from-earlier-kritique)
  lists what changed.
- **Authors:** perfectra1n.

> Scope: running a declared prompt when a forge event fires, and applying
> what the model's answer asks for once kritique has checked it. Issue triage
> is the first use. It does not change how a pull request review, a
> follow-up or an index run works; the review is not rebuilt on tasks.

## 1. Context

kritique reviews pull requests and nothing else. Every issue event, and every
delivery it has no use for, is dropped at the webhook parser. Issue triage
was the first request beyond reviews, and it generalizes: "when this event
fires, run this prompt with these things, and report back when done".
Hard-coding triage would answer one request and leave the next one
(labelling pull requests by area, summarizing a release, welcoming a first
contributor) to another feature each time.

The pieces a general answer needs mostly exist:

- The webhook parser verifies and decodes every delivery.
- `repoconfig` reads a repository's `.kritique.yaml` and holds what it may
  choose to the admin's settings.
- Repository-written comment templates are already Go `text/template` in a
  bounded sandbox, and repository-written conditions (`trigger.include`,
  `trigger.exclude`, a rule's `when`) are already CEL.
- The agent loop, the gateway and the runner pod run a model with
  read-only tools and no secrets: the runner never holds a provider key and
  reaches its model through the worker's gateway.
- The forge client posts and updates sticky comments, and reads files at a
  commit.

What does not exist is a way to let a repository say what should happen,
without letting a model (or whoever wrote the issue it reads) write to the
forge beyond what the repository and the admin allowed.

## 2. Decision

A **task** is a small declarative program: a **trigger** (any forge event,
plus `when` conditions), the **context** it gathers, the **prompt** (Go
templates), the **answer** (the custom fields it declares and the actions it
may propose), the **actions** (proposed by the model or driven by rules,
each with its own `when`) and the **report** (a templated comment and the
dashboard). Repositories declare tasks in `.kritique.yaml`; the admin turns
tasks on, bounds them and may declare its own. A new `internal/tasks`
package owns the definition, its validation, clipping to the bounds,
matching, rendering, the answer schema and the plan of writes, and does no
I/O itself.

### 2.1 Events: normalized and raw

The webhook parser gains an issue kind, stops dropping issue comments, and
names the subject of every issue, pull request and comment event (its kind,
`issue` or `pull`, and number). Every verified delivery also carries its
forge, the event header verbatim, the action, the sender and the raw JSON
payload, including kinds the review pipeline ignores (`release`, `push`,
`workflow_run` and so on). "Ignored" keeps meaning the review ignores it; a
task may still match it.

Tasks trigger on a normalized vocabulary that is meant to read the same on
every forge (`issue`, `pull_request` and `comment`, each with the forge's
action strings as globs), or on a forge-specific raw event: `raw: { event,
actions }`, globs over the forge's event header and the action. kritique
serves GitHub alone for now; a forge added later maps its own action
spellings onto the normalized ones in its parser.

The raw payload is stored once per delivery some task could match, in a
`task_events` row under row-level security, and jobs reference it by id so
the queue's arguments stay small. The dispatch job (§2.4) deletes it again
when no task matches, and the rest are swept with the transcripts, after
`KRITIQUE_TRANSCRIPT_RETENTION`. Tasks see the payload decoded, as `raw`
in conditions and `.Raw` in templates, and the prompt carries it (cut to
32 KiB) whatever the templates say.

### 2.2 The definition

The implemented surface, with issue triage as the example:

```yaml
tasks:
  - name: triage # [a-z0-9-]; names the task's marker, runs and notes
    on:
      - issue: [opened, reopened]
      - comment: [created]
      - raw: { event: issues, actions: [transferred] }
    when: # CEL over event, subject, raw; one must hold
      - { name: untriaged, expr: '!("triaged" in subject.labels)' }
    mode: agentic # agentic (default) | single
    models: { review: openrouter/openai/gpt-6-mini }
    agent:
      steps: 20
      timeout: 5m
      tools: [read_file, grep]
      commands: [rg]
    context:
      thread: { comments: 20 }
      files: [{ path: CONTRIBUTING.md }, { glob: "docs/area/*.md", max: 5 }]
      search: [{ name: code, query: "{{ .Subject.Title }}", k: 8 }]
      related: [{ name: dupes, query: "{{ .Subject.Title }}", k: 5 }]
      commands: [{ name: owners, run: cat .github/CODEOWNERS }] # agentic only
    system: .kritique/tasks/system.md.tmpl # after kritique's fixed preamble
    prompt: .kritique/tasks/triage.md.tmpl # or promptInline
    fields:
      priority: { type: string, enum: [p0, p1, p2, p3] }
      needsInfo: { type: boolean }
      missing:
        type: array
        items: { type: string, maxLength: 200 }
        maxItems: 5
    actions:
      comment:
        mode: sticky
        template: .kritique/tasks/triage-comment.md.tmpl
        when: [{ expr: answer.fields.needsInfo }]
      labels:
        propose: { add: [bug, enhancement], remove: [needs-triage] }
        rules:
          - add: ["priority/{{ .Fields.priority }}"]
          - { add: [needs-info], when: [{ expr: answer.fields.needsInfo }] }
      state:
        propose: { close: true }
        rules: [{ close: true, when: [{ expr: '"spam" in answer.labels.add' }] }]
      assign: { propose: { users: [alice, bob] } }
      reviewers: { rules: [{ users: [bob], when: [{ expr: 'subject.kind == "pull"' }] }] }
      inlineComments: { propose: true }
    enabled: true # false switches off a task of this name
```

[Event tasks](../tasks.md) is the reference for every key, and
`docs/kritique.schema.json` its schema, kept in step with the Go types by
`schema_test.go`. The rules the definition follows:

- **Validated whole, when the file is parsed.** Names, trigger globs, `when`
  conditions (a task's sees `event`, `subject` and `raw`; an action's also
  sees `answer`), field declarations, context sources, paths and template
  syntax are checked by `repoconfig.Parse`, and by the configuration file's
  loader for the admin's tasks. Preparing a task parses its template files
  and smoke-renders every template against a sample event and a sample
  answer, so a template that fails on the shape of the data fails before
  any event. A bad `tasks` block fails the whole `.kritique.yaml`, as any
  other bad key does, and a bad admin task fails startup.
- **`when` is a list, as everywhere else.** A task's, an action block's and
  a rule's `when` are lists of `{name, expr}` conditions, the shape a
  review rule's `when` takes: the list holds when one condition does, an
  empty one always holds, and a condition that fails to evaluate does not
  hold. An action left out for it is recorded as "no when condition holds"
  or "its when failed: ...".
- **Fields are a restricted JSON Schema.** A string with an `enum`,
  `maxLength` or `pattern`; a number or integer with a `minimum` and
  `maximum`; a boolean; an array of those with `maxItems`; or an object of
  those, one level deep. Unset lengths and sizes take defaults, so every
  string and list in an answer is bounded. The fields are compiled into the
  task's answer schema next to a required `summary` and the proposals its
  actions allow, and validated again when the answer is parsed.
- **Templates are sandboxed Go templates**, with a small funcmap of their
  own rather than the comment templates' sprout helpers: no `define` or
  `template`, a source cap, and renders bounded in output, loop iterations
  and bytes handed to functions. Prompt templates see the event, the
  subject, the thread (`.Thread`), the context sources and the task. The
  context sources are `.Context.files`, a list of `{path, content}` of the
  files named by path, and each `search` and `related` source under its
  `name`; a `related` source is a list of `{number, title, state, url,
  isPull, labels}`. Glob files and `commands` output need a checkout, so
  only an agentic task's runner gathers them, after the templates have
  rendered: it appends each to the prompt in its own `<untrusted>` block,
  and a template never sees them in `.Context`. The names `files` and
  `notes` are reserved. The comment and rule templates see the event, the
  answer and its fields, and the comment also what was applied and
  dropped.
- **Subject-less events write nothing.** A task a raw event without an
  issue or pull request can trigger may declare no action at all; its
  report is its run on the dashboard. Inline comments need a trigger that
  can be about a pull request. A `createIssue` action is deferred (§5).

### 2.3 Actions are declared, and the worker applies them

The model never holds a forge write credential and never calls a tool that
writes. It returns one structured answer; the worker parses it against the
answer schema, turns it into a plan, and applies the plan through its own
forge client.

Actions have two sources and one validator. `propose` lets the model choose
within an allowlist: label globs (offered to the model as an `enum` of the
repository's matching labels), close and/or reopen, a list of users.
`rules` are deterministic, with templated values and a `when` over the
event and the answer. Both feed one plan, which is checked as a whole:

- a label must exist in the repository, and a proposed one must match a
  `propose` glob; when a label is both added and removed, the add wins;
- a user must be a well-formed login with at least read access, and a
  proposed one must be listed; reviews are requested only on a pull
  request, never from its author;
- a state change must be declared, and a rule's wins over the model's; a
  change to the state the subject is already in is dropped;
- an inline comment must be on a pull request and anchored in its diff;
- a block whose `when` does not hold, or fails, contributes nothing.

Everything left out is recorded with its reason, and the report comment is
rendered last, with what was applied and dropped. Comments carry a hidden
`<!-- kritique:task:<name> -->` marker so a sticky report updates in place;
anything in a rendered comment that could pass for one of kritique's markers
is escaped.

The actions are a sticky or appended comment, inline comments, adding and
removing labels, closing and reopening, and assigning users and requesting
reviewers.

### 2.4 Execution

Execution is chosen per task:

- `mode: single` is one structured model call in the worker.
- `mode: agentic`, the default, runs a runner pod with a checkout of the
  default branch and the task's subset of the read-only agent tools, plus
  its allowlisted commands (which is where `context.commands` and glob
  files are gathered, argv split on whitespace, never through a shell).
  The agent gets the `run` tool only when the task lists `run` among its
  tools and names commands for it. The model is reached through the
  gateway, and the final answer comes back through the agent run, as a
  review's does.

Ingest makes no forge call. It consults only the admin's settings: when
tasks are on and an admin's task, or an event `taskBounds.events` lets a
repository's task trigger on, could match the delivery, it stores the
delivery and enqueues one `task_dispatch` job. That job drops the event if
its sender is the installation's bot, resolves the repository's tasks
(`repoconfig.Merge`, §2.6) from the `.kritique.yaml` at the default branch's
tip, records that commit and file on the repository for the dashboard
(§2.8), and enqueues one job per matching task on a `task` queue, unique by
task and event, carrying the commit. The task worker re-resolves the task
at that commit, applies `taskBounds.maxRunsPerSubjectPerHour` (runs of the
task on the subject that started in the last hour; past it the run is
skipped), admits the run through the account's model slots and token
budget, gathers the context sources (each bounded, and bounded in total),
renders the prompts, runs the model, plans and applies the actions, and
posts the report. An agentic run takes a slot on its model before it does
anything billable; while every slot is held the job is snoozed, which
spends no attempt, with a growing back-off. A `task_runs` row records the
trigger, status, mode, model, fields, and the proposed, applied and dropped
actions, with the run's transcript; usage flows into the usage and model
call records under the role `task`, like a review's under its own.

A failure before the model answers is retried. The answer's charge marks
the run answered (`task_runs.answered_at`) in the same transaction, and from
then on the run is never run again: an attempt that finds it answered but
unfinished ends it as failed, since some of its forge writes, which are not
idempotent, may already have been made.

The GitHub App needs the Issues and Pull requests write permissions, and
the Issues and Issue comment event subscriptions, for the tasks that use
them.

### 2.5 Security model

A task is written by the repository's maintainers, reads text written by
anyone who can open an issue, and runs a model that text may try to steer.
The design keeps each of those in its lane:

- **Declared actions only.** The model's answer is data. It can propose
  only what the task declared, and the worker checks every proposal against
  the task, the repository and the admin's bounds before it writes (§2.3).
  A prompt injection can at most pick among the options a maintainer
  already offered.
- **Only the worker writes.** Runner pods of agentic tasks get a git
  credential that can only read, and reach the model through the gateway,
  never a provider key or a forge write credential; their tools are
  read-only. On GitHub the worker mints an installation token for each run,
  scoped to the task's repository with `contents: read` alone, although the
  App itself holds Issues and Pull requests write for tasks. A forge
  without a way to mint a read-only credential must not fall back to a
  writing one for a task's runner: its agentic tasks are skipped. Single
  mode tasks start no runner and need none.
- **A fixed preamble and fenced data.** Every task's system prompt starts
  with kritique's preamble, which states that text inside `<untrusted>`
  blocks is data, never instructions, and that only what the answer schema
  offers may be proposed. A task's `system` is added after it, for a
  repository's task only when `taskBounds.systemPrompt` allows it, and
  cannot replace it. The subject, the thread and the raw payload are
  appended to the prompt as JSON inside `<untrusted>` blocks whatever the
  templates say. Templates see every event and thread string with any
  opening or closing `untrusted` tag escaped, and each context source's result
  already wrapped in its own block, so a template cannot inline data that
  escapes its fence; the price is that a template cannot post-process a
  context source's raw text.
- **No echo loops.** An event whose sender is the installation's own bot
  never triggers a task, so the labels and comments a task writes cannot
  trigger it again. The dispatch job checks it, since it needs the bot's
  login from the forge.
- **The configuration comes from the default branch's tip.** A task and
  every file it names are read at the default branch's tip, not at a pull
  request's merge base or head: an issue has no merge base, and only
  someone who can push to the default branch can change what a task does
  or what its templates say. The worker re-reads the task at the commit
  the dispatch job resolved it at.
- **Admin bounds, and who is trusted.** The configuration file is trusted:
  the admin's own tasks are not clipped, only switched on or off by
  `taskBounds.enabled`; they pick their own mode, models and agent limits,
  and a task job's timeout is sized to cover their agent timeouts too. A
  repository's tasks are clipped (§2.6).

### 2.6 The admin's bounds and tasks

`taskBounds` is written at the configuration file's root and in any
`repositories` entry (`owner/*` or `owner/name`), and overlays key by key:
one written at a narrower scope replaces the broader one's.

| Bound                      | Default                                  | Bounds                                                     |
| -------------------------- | ---------------------------------------- | ---------------------------------------------------------- |
| `enabled`                  | `false`                                  | whether any task runs, the admin's own included            |
| `events`                   | `issue.*`, `pull_request.*`, `comment.*` | globs over trigger names, such as `raw:release.*`          |
| `actions`                  | `comment`, `labels`, `inlineComments`    | action kinds; `state`, `assign` and `reviewers` are opt-in |
| `context`                  | all but `commands`                       | context source kinds                                       |
| `tools`                    | `read_file`, `grep`, `list_files`        | agent tools                                                |
| `systemPrompt`             | `false`                                  | whether a task may add to the system prompt                |
| `repository`               | `true`                                   | whether `.kritique.yaml` may define tasks                  |
| `maxTasks`                 | 10                                       | a repository's tasks                                       |
| `maxRunsPerSubjectPerHour` | 6                                        | runs of one task on one issue or pull request              |
| `maxFields`                | 16                                       | a task's fields                                            |

A task's trigger names are `issue.opened`, `raw:release.published` and so
on, with `*` for a trigger that lists no actions. `events` takes only `*`
and `**` wildcards, since a bound is matched against a trigger's own glob
text, which proves the trigger lies inside it only for those.

A repository's task is otherwise held to what the admin's settings resolve
to for the repository, as `.kritique.yaml`'s review keys are: its models to
a `<provider>/<model>` of a provider the account may use, as
`review.model` is; its agent `steps`, `output`, `tokens` and `timeout` to
at most the admin's `agent` values; and its commands, agent and context
alike, to the admin's `agent.commands`. Its mode is its own: a single call
and an agentic run are both bounded by the account's limits.

The admin's own tasks are `tasks` at the root and in each `repositories`
entry. They merge by name across the scopes, a narrower scope's replacing a
broader one's where it stands, so an account entry cannot silently hide a
root task; a task with `enabled: false` switches off the one it names,
which is also the only way to remove it.

`repoconfig.Merge` resolves the tasks that run: the admin's, unclipped,
then the repository's, clipped, less any sharing an admin task's name.
Clipping drops what is out of bounds rather than the whole task where it
can (an event, an action block, a context source, a tool, the `system`
prompt, fields past `maxFields`, a model, a limit or a command list), and
drops a task left with no trigger. Each drop is a note with the task, what
was dropped and why (`Merged.TaskNotes`), and the dashboard shows them
beside each repository's resolved tasks and where each came from. Callers
take tasks only from `Merge`'s result; the unclipped lists in
`configfile.Settings` are its input.

### 2.7 Forge clients

`forge.Client` gains reading an issue, the repository's labels, adding and
removing labels, closing and reopening, assigning users, requesting
reviewers and searching issues (for the `related` context source).
Comments, conversations, reviews, permissions, file reads and branch tips
reuse what exists.

### 2.8 Dashboard

The dashboard lists task runs, their fields, the three action lists and a
transcript link, with live updates. A repository's resolved tasks show
their names, where each came from (the configuration file or
`.kritique.yaml`), triggers, mode and action kinds, with the clipping
notes. They are resolved from the `.kritique.yaml` the last dispatch read at
the default branch's tip, which it records on the repository; before any
dispatch, from the one the last review read at its merge base, and the page
says which. The dashboard only shows tasks: like the rest of the
configuration, they are written in the configuration file and
`.kritique.yaml`.

### 2.9 Changes from earlier kritique

The earlier kritique's ADR-0012 described the same design against a
configuration file with `defaults`, `tenants` and an `allow` block, and
tasks a tenant admin could write on the dashboard. This repository's file
has none of those, so:

- `defaults.tasks`, `tenants[].tasks` and `repositories[].tasks` are now
  `tasks` at the root, in `repositories["owner/*"]` and in
  `repositories["owner/name"]`.
- `allow.tasks.*` is now `taskBounds.*`, at any scope, and
  `allow.tasks.repositoryTasks` is `taskBounds.repository`.
- A task's `if: <expr>` is now `when: [{name, expr}]`, any one holding,
  empty always running; the same holds for `actions.comment`, `labels`,
  `state`, `assign`, `reviewers` and `inlineComments`, and for each rule of
  `labels`, `state`, `assign` and `reviewers`. Plan drop reasons now read
  "no when condition holds" and "its when failed: ...".
- A task's `agent.maxSteps`, `agent.maxToolOutputBytes` and
  `agent.maxTokens` are now `agent.steps`, `agent.output` and
  `agent.tokens`, as the admin's `agent` spells them; `timeout`, `tools`
  and `commands` are unchanged.
- A repository task's models were held to `allow.models`; now any model of
  a provider the account may use. Its agent limits were held to
  `allow.agent`; now to the admin's own agent settings. Its commands were
  held to `allow.commands` (or the admin's `agent.commands`); now to the
  admin's `agent.commands`. Its mode was held to `allow.modes`; now it may
  be either.
- Dashboard-managed tasks are gone: the dashboard writes no configuration.
- The earlier kritique also served Forgejo and Gitea, and mapped their
  action spellings (`synchronized`, `label_updated`, `label_cleared`) onto
  GitHub's. This repository serves GitHub alone for now; their parsing is
  to be added back separately (§5).

Everything else is unchanged: dispatch at the default branch tip's
`.kritique.yaml`, the loop guard on the bot's own events,
`maxRunsPerSubjectPerHour`, single and agentic modes, agentic runs in a
runner pod reaching the model through the gateway with a read-only git
token, the model slot snooze, never re-running an answered run,
transcripts, usage counted under the role `task`, and task events kept for
the transcript retention and swept.

## 3. Consequences

- A repository can automate issue and pull request housekeeping with a
  file and two templates, and kritique gains no feature code per use: triage
  ships as a documented recipe, tested like any other.
- Tasks are off until the admin turns them on, and a new action kind, raw
  event or context kind is off for repositories until the admin lists it.
- Everything a task can write is visible in its definition, and every
  write is recorded with what the model proposed and what was dropped.
- `internal/tasks` is a large surface in one pure, table-tested package.
- A typo in `tasks` stops the whole `.kritique.yaml` from applying, the
  review's settings included, until it is fixed.
- The App needs write access to issues and pull requests, beyond what
  reviews need, and the Issues event subscription.
- Templates cannot post-process a context source's text, which stays
  fenced, and an agentic task's glob files and command output reach the
  prompt but not its templates.
- What a run's context left out, the worker's cuts and the runner's, is
  recorded in the run's notes.

## 4. Rejected alternatives

- **A hard-coded triage feature.** Quicker for one use, but every later
  request (area labels, release notes, welcome messages) would be another
  feature with its own configuration, and the security review would be
  repeated for each. The general form costs one package and one review.
- **An agent with live forge tools.** Giving the model tools that label,
  close or comment directly would let text in an issue steer real writes,
  and would put a write credential in the runner pod, which kritique keeps
  out of every runner. A declared, validated answer applied by the worker
  keeps the model's reach to the options a maintainer offered.
- **Comment-only tasks.** Letting a task only post a comment is safe, but
  leaves the actual triage (labels, state, assignees) to a human reading
  the comment; the plan-and-validate step gives the same safety for the
  writes that matter.
- **A separate `allow` block for tasks.** The earlier layout bounded tasks
  in `allow`, beside bounds for the review. This repository bounds a
  `.kritique.yaml` by the admin's own resolved settings rather than a
  separate allowlist, so a task's models, agent limits and commands follow
  the same rule, and only what tasks alone need (`taskBounds`) is new.

## 5. Deferred

- A `createIssue` action, so a subject-less event such as a release can
  report somewhere other than the dashboard.
- Rebuilding the pull request review on tasks.
- Forgejo and Gitea, whose event parsing is being added back separately,
  and GitLab.
- Letting a broken `tasks` block leave the rest of `.kritique.yaml` in force.
