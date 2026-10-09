# Event tasks

A task runs a prompt when a forge event fires: a new issue, a label on a
pull request, a comment, a published release. It gathers the context the
task names, has the model answer with the fields the task declares and the
actions it may propose, applies what survives validation and reports back.
Issue triage is one such task, written in [the recipes](#recipes), not a
feature of its own. [ADR-0001](adr/0001-event-tasks.md) records the design.

Tasks are declared in two places, by the same keys:

- the [configuration file](configuration.md), under `tasks` at its root
  and in each `repositories` entry: the admin's own tasks, which are
  trusted;
- a repository's [`.kritique.yaml`](repository-config.md), under `tasks`:
  held to the admin's [`taskBounds`](#bounds) and agent settings.

Tasks are off until an admin sets `taskBounds.enabled: true`, the admin's
own included.

```yaml
# The configuration file.
taskBounds:
  enabled: true
  events: ["issue.*", "pull_request.*", "comment.*", "raw:release.*"]
  actions: [comment, labels, inlineComments, assign]
tasks:
  - name: welcome
    on: [{ pull_request: [opened] }]
    when:
      - name: first-timer
        expr: raw.pull_request.author_association == "FIRST_TIME_CONTRIBUTOR"
    mode: single
    promptInline: "Welcome the author of pull request #{{ .Subject.Number }} and point them at CONTRIBUTING.md."
    actions: { comment: { mode: sticky } }
repositories:
  org-1/*:
    taskBounds: { actions: [comment, labels] }
  org-1/repo-1:
    tasks:
      - { name: welcome, enabled: false }
```

The model never writes to the forge. It returns one structured answer, and
kritique's worker checks every value in it against what the task declares and
what the repository has before it applies anything. A runner pod that runs
an agentic task holds only a git credential that can only read.

## Where a task comes from

- **Read at the default branch's tip.** A repository's tasks, and every
  file they name, are read from the `.kritique.yaml` at the tip of the
  default branch, not a pull request's merge base as the review settings
  are: an issue has no merge base, and only someone who can push to the
  default branch should change what a task does or what its templates say.
  A run re-reads its task at the commit its event was matched at.
- **Merged by name.** The admin's tasks merge by name across the file's
  root, `owner/*` and `owner/name`: a narrower scope's task replaces a
  broader scope's of the same name where it stands, and one with
  `enabled: false` switches it off, which is the only way to remove it.
- **The admin's run first.** A repository's tasks run after the admin's,
  and one named like an admin's task is dropped.
- **A bad block fails the file.** A `tasks` block that does not parse makes
  the whole `.kritique.yaml` fail to parse, as any other bad key does, and in
  the configuration file fails startup.

## A task's keys

| Key                                                            | What it does                                                                                                                                                                                                                    |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`                                                         | Required. Lowercase letters, digits and `-`, at most 63, starting with a letter or digit; unique among the tasks it is written beside. It names the task in its sticky comment's marker, its runs and its notes.                |
| `enabled`                                                      | `false` switches the task off, and then only `name` is checked. Unset is on.                                                                                                                                                    |
| `on`                                                           | Required. The [triggers](#triggers); the task runs when any of them fires.                                                                                                                                                      |
| `when`                                                         | [Conditions](#conditions) over `event`, `subject` and `raw`, each an `expr` with an optional `name`; the task runs when one holds. Empty always runs.                                                                           |
| `mode`                                                         | `agentic` (the default) runs the task in a runner pod with a checkout of the default branch, the read-only agent tools and a git credential that can only read; `single` is one structured model call in the worker.            |
| `models.review`, `models.fallback`                             | `<provider>/<model>` of a provider the repository's account may use. In `.kritique.yaml` one of any other provider is dropped and the admin's model applies; in the configuration file it fails startup.                        |
| `agent.steps`, `agent.output`, `agent.tokens`, `agent.timeout` | As the admin's [`agent`](configuration.md#the-agent) keys, for the task's tool loop. In `.kritique.yaml` each is at most the admin's resolved `agent` value for the repository; one over it is dropped and the admin's applies. |
| `agent.tools`                                                  | The agent tools an agentic run gets. Unset is every tool `taskBounds.tools` allows; `[]` is none; `run` needs `agent.commands`.                                                                                                 |
| `agent.commands`                                               | Command names the agent's `run` tool may execute. In `.kritique.yaml` they must all be among the admin's `agent.commands`, or the list is dropped.                                                                              |
| `context`                                                      | The [context sources](#context-sources) a run gathers.                                                                                                                                                                          |
| `system`                                                       | A path to a template added after kritique's fixed system preamble. A repository's task keeps it only when `taskBounds.systemPrompt` is on.                                                                                      |
| `prompt`                                                       | A path to the prompt template.                                                                                                                                                                                                  |
| `promptInline`                                                 | The prompt template itself; set `prompt` or `promptInline`, not both. With neither, a built-in prompt asks for a summary and the fields.                                                                                        |
| `fields`                                                       | The custom [answer fields](#fields).                                                                                                                                                                                            |
| `actions`                                                      | What the answer may lead to: [actions](#actions).                                                                                                                                                                               |

Paths are relative to the repository root, may not leave it, and are read
from the default branch's tip, for the admin's tasks as for a
repository's.

## Triggers

Each entry of `on` has exactly one key:

- `issue: [actions]`: an issue, never a pull request. Actions include
  `opened`, `edited`, `reopened`, `closed`, `labeled`, `unlabeled` and
  `assigned`.
- `pull_request: [actions]`: a pull request. Actions include `opened`,
  `synchronize`, `reopened`, `ready_for_review`, `closed`, `labeled` and
  `unlabeled`.
- `comment: [actions]`: a comment on an issue or a pull request, including a
  review comment on a pull request's lines; `created` or `edited`.
- `raw: { event: <name>, actions: [actions] }`: any delivery whose event
  header, such as GitHub's `X-GitHub-Event`, matches `event`, such as
  `release`, `push` or `workflow_run`, whether or not kritique otherwise acts
  on it. The names are the forge's own, so a raw trigger is forge-specific.

The actions are the forge's `action` strings, as globs; an empty or missing
list is any action. An event kritique's own bot account sent never triggers a
task, so the labels and comments a task writes cannot trigger it again.

`taskBounds.events` globs a trigger's event names: `issue.opened`,
`pull_request.synchronize`, `comment.created`, or `raw:<event>.<action>`
such as `raw:release.published`, with `*` for a trigger that lists no
actions.

Globs are [doublestar](https://github.com/bmatcuk/doublestar) globs, for
actions, labels and paths alike: `*` matches within one `/`-separated
segment, so `area/*` matches `area/web` but not `area/web/ui`, and `*`
alone matches only values without a `/`; `**` matches across segments.

## Conditions

A task's `when`, each action block's `when` and each rule's `when` are
lists of conditions, each a [CEL](https://cel.dev) `expr` that must be
boolean, with an optional `name`. A list holds when one of its conditions
does, and an empty one always holds. A condition that fails to evaluate
does not hold. A condition sees:

- `event`: `forge` (`github`), `name` (`issue`, `pull_request`, `comment`,
  or `""` for a raw event), `rawEvent` (the event header), `action`,
  `sender`, and `repo` with `owner`, `name` and `defaultBranch`.
- `subject`: the issue or pull request: `kind` (`issue` or `pull`),
  `number`, `title`, `body`, `state`, `author`, `url`, `labels`,
  `assignees` and `draft`. An event with no issue or pull request has every
  key, with `kind` empty, so `subject.kind == "issue"` is false rather than
  an error.
- `raw`: the decoded payload, as the forge sent it.
- `answer`, in an action's `when` only (and a rule's): the model's validated
  answer, with `summary`, `comment`, `fields`, `labels.add`,
  `labels.remove`, `state`, `assignees`, `reviewers` and `inline` (each
  with `path`, `line`, `end_line` and `body`). Every key is present, empty
  when the task does not offer it. A task's own `when` using `answer` does
  not parse.

Each expression is compiled when the file is read, so a broken one is
rejected there. Evaluation is cost-limited, since `raw` is as large as a
webhook body may be. An action left out because its `when` did not hold
is recorded with the reason `no when condition holds`, or `its when
failed: ...` with the error.

## Templates

Templates are Go [text/template](https://pkg.go.dev/text/template), run in
a sandbox: `define` and `template` are refused, a template's source is
capped at 64 KiB, and a render is bounded in loop iterations, bytes handed
to functions and output size (256 KiB for a prompt, 64 KiB for a comment,
1 KiB for a rule value or query). Missing map keys render as their zero
value. Every template is parsed and smoke-rendered against a sample event
and a sample answer when the task is prepared, so one that fails on the
shape of the data fails there rather than on an event.

The functions are `join`, `lower`, `upper`, `trim`, `truncate` (to a number
of runes), `toJSON`, `default`, `contains` (a string or a list),
`hasPrefix`, `quote`, `fence`, `printf` (widths and precisions at most
1024), `print`, `println`, `html`, `js` and `urlquery`, as well as
text/template's builtins such as `index`, `len`, `eq`, `and`, `or` and
`not`. Note the argument order: `{{ join ", " .Subject.Labels }}`,
`{{ truncate 80 .Subject.Title }}`, `{{ default "none" .Fields.area }}`,
`{{ contains "bug" .Subject.Labels }}`, `{{ hasPrefix "area/" . }}`.

What a template sees depends on what it renders:

| Variable                                                                                                                                                           | `system`, `prompt`, `promptInline` | `context` queries | `actions.comment.template`, rule values |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------- | ----------------- | --------------------------------------- |
| `.Forge`, `.Event`, `.RawEvent`, `.Action`, `.Sender`                                                                                                              | yes                                | yes               | yes                                     |
| `.Subject` (`.Kind`, `.Number`, `.Title`, `.Body`, `.State`, `.Author`, `.URL`, `.Labels`, `.Assignees`, `.Draft`); nil without one                                | yes                                | yes               | yes                                     |
| `.Raw`, the decoded payload                                                                                                                                        | yes                                | yes               | yes                                     |
| `.Repo` (`.Owner`, `.Name`, `.DefaultBranch`)                                                                                                                      | yes                                | yes               | yes                                     |
| `.Task`, the task's definition                                                                                                                                     | yes                                | yes               | yes                                     |
| `.Thread`, the subject's comments (`.Author`, `.Body`, `.CreatedAt`)                                                                                               | yes                                |                   |                                         |
| `.Context.files` and `.Context.<name>`, context sources' results                                                                                                   | yes                                |                   |                                         |
| `.Answer` (`.Summary`, `.Comment`, `.Fields`, `.Labels.Add`, `.Labels.Remove`, `.State`, `.Assignees`, `.Reviewers`, `.Inline`) and `.Fields`, the answer's fields |                                    |                   | yes                                     |
| `.Applied` (`.AddLabels`, `.RemoveLabels`, `.State`, `.Assignees`, `.Reviewers`, `.Inline`) and `.Dropped` (each `.Action`, `.Value`, `.Reason`)                   |                                    |                   | comment only                            |

Everything in an event comes from people who may try to steer the model, so
the prompt keeps it apart from instructions whatever the templates say:

- The system prompt always starts with kritique's fixed preamble: data inside
  `<untrusted>` blocks is never instructions, and only what the answer
  schema offers may be proposed. A task's `system` is added after it and
  cannot replace it.
- The subject, the thread and the raw payload (cut to 32 KiB) are appended
  to the prompt as JSON inside `<untrusted>` blocks.
- In the prompt templates every string of the event and thread has any
  opening or closing `untrusted` tag escaped, so text a template inlines
  cannot open or close a block, and every `.Context` value is already inside its
  own `<untrusted>` block. `{{ fence "name" .X }}` wraps any other value
  the same way.
- A report comment has anything that could pass for one of kritique's hidden
  markers escaped.

## Context sources

`context` names what a run gathers besides the event. A repository's task
may use only the kinds `taskBounds.context` lists:

- `thread: { comments: N }`: the subject's last `N` comments (at most 200),
  as `.Thread`, not in `.Context`.
- `files`: files from the default branch, each a `path`, or a `glob` with
  a `max` of at most 50 files, as `.Context.files`: a list of
  `{path, content}`.
- `search`: `[{ name, query, k }]`, the `k` (at most 50) chunks of the
  repository's index most like `query`, as `.Context.<name>`.
- `related`: `[{ name, query, k }]`, the forge's issues and pull requests
  matching `query`, such as likely duplicates, as `.Context.<name>`: a list
  of `{number, title, state, url, isPull, labels}`. The query is free text:
  on GitHub every word with a colon (a qualifier such as `repo:` or
  `is:open`) is dropped, since the query usually holds an issue author's
  title, and issues and pull requests are searched apart and interleaved.
  A `search` or `related` source that fails is left out with a note; the
  run goes on without it.
- `commands`: `[{ name, run }]`, agentic tasks only: a command line run in
  the runner pod without a shell, split on whitespace, appended to the
  prompt in its own block under its `name` (not in a template's
  `.Context`, see below). In `.kritique.yaml` its first word must be one of
  the admin's `agent.commands`, or the source is dropped. Shell characters
  (`| ; & $ < > ( ) { } * ? ~`, quotes and backslashes) are refused. Off
  for a repository's task unless `taskBounds.context` lists `commands`.

A `search`, `related` or `commands` source is keyed by its `name`, which
must start with a lowercase letter and hold only letters, digits and `_`,
unique across them; `files` and `notes` are reserved. A `query` is a
template over the event. Every `.Context` value reaches a template already
inside its own `<untrusted>` block, as JSON when it is not text, so
`{{ .Context.files }}` inlines the whole list and a template cannot pick
out its fields. `{{ range .Context }}{{ . }}{{ end }}`, as the built-in
prompt does, puts every gathered source in the prompt.

Glob files and command output need a checkout, so only an agentic task
gets them: its runner gathers them after the templates have rendered and
appends each to the prompt in its own `<untrusted>` block, so they are not
in a template's `.Context`, and `.Context.files` holds the `path` files
alone. A single-mode task leaves a glob out. Each source is cut to 32 KiB
and all of them to 128 KiB; what a run left out, in the worker or the
runner, is listed in its notes. An agentic task's agent gets the `run`
tool only when `agent.tools` lists `run` (which `taskBounds.tools` must
allow for a repository's task) and `agent.commands` names commands for it.

## Fields

`fields` declares the custom values the model answers with, beside the
required `summary`, as a restricted JSON Schema, in the order written:

- `string`, with `enum`, `maxLength` (1 to 10000; 1000 when unset) or
  `pattern` (a Go regular expression);
- `number` or `integer`, with `minimum` and `maximum`;
- `boolean`;
- `array` of one of those, with `items` and `maxItems` (1 to 100; 20 when
  unset);
- `object` of those, one level deep, with `properties`.

Any may have a `description`. A field name starts with a lowercase letter
and holds only letters, digits and `_`. Every field is required in the
answer, and an answer that breaks the schema is rejected as a whole. The
fields are `.Fields` in output templates and `answer.fields` in action
conditions, and the dashboard shows them with the run.

## Actions

`actions` declares what the answer may lead to; a block left out is off.
Most blocks take a model's proposal (`propose`), rules that apply whatever
the model says (`rules`, each with templated values and a `when`), or both,
and a `when` over the whole block. Both feed one plan, which is checked as
a whole before anything is written, and everything left out is recorded
with its reason.

- `comment: { mode, template, when }`: the report. `sticky` (the default)
  updates one comment per task and subject, `append` posts a new one each
  run, `none` posts nothing. `template` is a path to the comment template;
  without one the report is the summary, the answer's `comment` and what
  was applied. When the comment is on, the answer gains a `comment` string.
- `labels: { propose: { add, remove }, rules: [{ add, remove, when }], when }`:
  `propose` lists label globs the model may add or remove; the answer
  schema offers only the repository's labels that match. Each rule's `add`
  and `remove` are templates, rendered against the answer. Every label must
  exist in the repository; when a label is both added and removed, the add
  wins.
- `state: { propose: { close, reopen }, rules: [{ close | reopen, when }], when }`:
  close or reopen the subject. A rule sets exactly one of them, and a rule
  wins over the model. A change to the state the subject is already in is
  dropped.
- `assign: { propose: { users }, rules: [{ users, when }], when }`: assign
  users. The model picks among `propose.users`; rule values are templates.
  A user must be a valid login with at least read access to the repository.
- `reviewers`: the same shape as `assign`, to request reviews; pull
  requests only, and never from the author.
- `inlineComments: { propose: true, when }`: comments on lines the pull
  request changes; lines outside the diff are dropped. It needs a trigger
  that can be about a pull request.

A task that a subject-less event can trigger (a `raw` trigger on anything
but `issues`, `issue_comment`, `pull_request`, `pull_request_review_comment`
or `pull_request_comment`) may declare no action at all: there is no issue
or pull request to write to, so its report is the run on the dashboard.
A raw `issues` trigger counts as carrying a subject, but a delivery about
an issue that is a pull request has none: the task runs with
`subject.kind` empty, and every action on the subject is dropped from its
plan. Use `pull_request` for pull requests.

## Running a task

- **Dispatch.** When tasks are on and the admin's task, or an event
  `taskBounds.events` lets a repository's task trigger on, could match a
  delivery, kritique keeps the delivery and resolves the repository's tasks
  from the `.kritique.yaml` at the default branch's tip. Each matching task
  is queued once per event.
- **Rate.** `taskBounds.maxRunsPerSubjectPerHour` caps how often one task
  runs on one issue or pull request; a run past it is skipped.
- **Modes.** A single-mode run is one model call in the worker. An agentic
  run starts a runner pod, which reaches its model through kritique's
  gateway and never holds a provider key or a forge write credential: on
  GitHub its token is minted for the run with `contents: read` alone. An
  agentic run waits for a free slot on its model
  ([`limits.concurrency`](configuration.md#accounts)) before it spends
  anything, without using up a retry.
- **Retries.** A run that fails before the model answers is tried again.
  Once the model has answered, the run is never run again: some of its
  forge writes, which are not idempotent, may already have been made.
- **Records.** Each run records its trigger, status, mode, model, fields,
  and the proposed, applied and dropped actions, with its transcript. Its
  usage counts under the role `task`, against the account's
  `tokensPerMonth`. Kept deliveries and run transcripts are deleted after
  [`KRITIQUE_TRANSCRIPT_RETENTION`](configuration.md#how-kritique-runs).

The GitHub App needs, for the tasks that use them, the **Issues** and
**Pull requests** permissions at read and write, and subscriptions to the
**Issues** and **Issue comment** events, plus **Pull request** and any
other event a task triggers on, a `raw` trigger's included
([setup](setup.md#register-the-github-app)).

## Bounds

`taskBounds` bounds tasks. It may be written at the file's root and in any
`repositories` entry, and overlays key by key: one written at a narrower
scope replaces the broader one's.

| Key                        | Default                                      | What it bounds                                                                                               |
| -------------------------- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `enabled`                  | `false`                                      | Whether any task runs, the admin's own included.                                                             |
| `events`                   | `["issue.*", "pull_request.*", "comment.*"]` | Globs over trigger event names, with `*` and `**` only; a raw event must be listed, such as `raw:release.*`. |
| `actions`                  | `[comment, labels, inlineComments]`          | Action kinds; `state`, `assign` and `reviewers` must be listed.                                              |
| `context`                  | `[thread, files, search, related]`           | Context source kinds; `commands` must be listed.                                                             |
| `tools`                    | `[read_file, grep, list_files]`              | Agent tools; `run` is opt-in.                                                                                |
| `systemPrompt`             | `false`                                      | Whether a task may add to the system prompt.                                                                 |
| `repository`               | `true`                                       | Whether a `.kritique.yaml` may define tasks at all.                                                          |
| `maxTasks`                 | `10`                                         | A repository's tasks.                                                                                        |
| `maxRunsPerSubjectPerHour` | `6`                                          | How often one task runs on one issue or pull request, the admin's own included.                              |
| `maxFields`                | `16`                                         | A task's fields.                                                                                             |

The admin's own tasks are trusted: only `enabled` and
`maxRunsPerSubjectPerHour` apply to them. A repository's task is clipped
to every bound, and its models, agent limits and commands are held to
what the admin's settings resolve to for the repository (see
[a task's keys](#a-tasks-keys)). Clipping drops what is out of bounds, not
the whole task where it can: an event, an action block, a context source,
a tool, the `system` prompt, fields past `maxFields`, a model, a limit or
a command list. A task left with no trigger is dropped. Each drop is a
note naming the task, what was dropped and why, such as
`task releases: on[0] raw:release.*: the event is not allowed`.

## Recipes

Each recipe is a directory under
[`docs/recipes`](https://github.com/perfectra1n/kritique/tree/main/docs/recipes)
laid out as it would be in a repository, and kritique's tests parse and
prepare every one of them and check it fires on the event it is written
for.

### Issue triage

Label a new issue by type, priority and area, ask its author for what is
missing, and point out duplicates. The model picks the type label from a
short list; the rules add the priority, area and `triaged` labels from its
fields, so the task's `when` leaves an issue alone once it is triaged.

`.kritique.yaml`:

```yaml
--8<-- "docs/recipes/issue-triage/.kritique.yaml"
```

`.kritique/tasks/triage.md.tmpl`:

```text
--8<-- "docs/recipes/issue-triage/.kritique/tasks/triage.md.tmpl"
```

`.kritique/tasks/triage-comment.md.tmpl`:

```text
--8<-- "docs/recipes/issue-triage/.kritique/tasks/triage-comment.md.tmpl"
```

The repository needs the labels the task may add (`bug`, `enhancement`,
`question`, `documentation`, `needs-info`, `triaged` and each
`priority/…` and `area/…`); any it lacks is dropped from the run and noted.

### Label pull requests by area

Offer every `area/` label the repository has, and add the ones the model
picks only when it says it is sure. A task sees a pull request's title and
description, not its diff.

`.kritique.yaml`:

```yaml
--8<-- "docs/recipes/pr-area-labels/.kritique.yaml"
```

### Release summary

Summarize each published release for the dashboard. `release` is a raw
event, so an admin must allow it, with `raw:release.*` in
`taskBounds.events`, and the task has no actions: the summary and its
fields are the run's report.

`.kritique.yaml`:

```yaml
--8<-- "docs/recipes/release-summary/.kritique.yaml"
```

## Changes from earlier kritique

Tasks came over from an earlier kritique whose configuration file had
`defaults`, `tenants` and an `allow` block. They work as they did; what
changed is where they are written and what bounds them:

| Earlier                                                         | Now                                                                                            |
| --------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `defaults.tasks`, `tenants[].tasks`, `repositories[].tasks`     | `tasks` at the root, in `repositories["owner/*"]` and in `repositories["owner/name"]`          |
| `allow.tasks.*`                                                 | `taskBounds.*`, at any scope                                                                   |
| `allow.tasks.repositoryTasks`                                   | `taskBounds.repository`                                                                        |
| a task's `if: <expr>`                                           | `when: [{ name, expr }]`, any one holding; empty always runs                                   |
| an action's or a rule's `if`                                    | its `when`, the same way                                                                       |
| `agent.maxSteps`, `agent.maxToolOutputBytes`, `agent.maxTokens` | `agent.steps`, `agent.output`, `agent.tokens`; `timeout`, `tools` and `commands` are unchanged |
| a repository task's models held to `allow.models`               | any model of a provider the account may use                                                    |
| its agent limits held to `allow.agent`                          | held to the admin's own `agent` settings for the repository                                    |
| its commands held to `allow.commands`                           | held to the admin's `agent.commands`                                                           |
| its mode held to `allow.modes`                                  | any mode, `single` or `agentic`                                                                |
| tasks a tenant admin wrote on the dashboard                     | gone: the dashboard writes no configuration                                                    |
| an action dropped as "its if is false" or "its if failed"       | "no when condition holds" or "its when failed: ..."                                            |

kritique serves GitHub alone for now.

An earlier configuration file:

```yaml
defaults:
  allow:
    modes: [single, agentic]
    tasks:
      enabled: true
      repositoryTasks: true
      events: ["issue.*", "raw:release.*"]
  tasks:
    - name: triage
      on: [{ issue: [opened] }]
      if: '!("triaged" in subject.labels)'
      agent: { maxSteps: 20, maxTokens: 200000 }
      actions:
        labels:
          rules: [{ add: [needs-info], if: answer.fields.needsInfo }]
tenants:
  - name: org-1
    allow:
      tasks: { actions: [comment, labels, assign] }
```

is now:

```yaml
taskBounds:
  enabled: true
  repository: true
  events: ["issue.*", "raw:release.*"]
tasks:
  - name: triage
    on: [{ issue: [opened] }]
    when: [{ name: untriaged, expr: '!("triaged" in subject.labels)' }]
    agent: { steps: 20, tokens: 200000 }
    actions:
      labels:
        rules: [{ add: [needs-info], when: [{ expr: answer.fields.needsInfo }] }]
repositories:
  org-1/*:
    taskBounds: { actions: [comment, labels, assign] }
```

A `.kritique.yaml` task changes the same way: `if` becomes `when`, and the
agent keys are renamed.
