# Upgrading from the earlier kritique layout

kritique 0.1.x ran with tenants, installations, a `defaults` block and a
split `roles` topology. This release rebases kritique on a newer design:
one `kritique serve` Deployment, a configuration file keyed by `apps`,
`repositories` and `accounts`, secrets from environment variables only,
and most process settings as `KRITIQUE_*` variables the chart renders from
`config`. Nothing is converted automatically: the chart refuses the old
values it no longer knows, and kritique refuses a configuration file with
an unknown key.

This page lists every change an operator of the earlier layout has to
make, in the order to make them.

## Before you start

- **The database is upgraded in place.** Keep pointing the chart at the
  same database. Old kritique's migrations `0001`–`0014` are kept as they
  were; `0015_users`, `0016_accounts` and `0017_kritika_schema` turn
  tenants into accounts and installations into connections, giving each
  row the id the new code derives from its name. What does not carry over:
  - everyone signs in again (sessions are recreated);
  - dashboard-managed tenants, memberships and pending invites are
    dropped; each dashboard tenant's spec is first copied into
    `audit_events` (action `migration.archive_dashboard_tenant`), so move
    anything you still need into the configuration file;
  - model leases, `reviews.mode`, `reviews.canceled_by`,
    `pull_requests.base_sha` and a few other columns upstream removed.
  The owner role needs the default `TEMPORARY` privilege on the database
  (the account id hashing runs in temporary SQL functions). The migration
  stops with an error if two installations of the same forge account held
  the same repository.
- **Uninstall the old release first, or delete its Deployments.** The
  Deployment that was `roles.all` keeps its name but its selector changes
  (`app.kubernetes.io/component: all` becomes `server`), which
  Kubernetes refuses to update in place. The `-ingest`, `-worker` and
  `-web` Deployments and the `-web` Service are no longer rendered.
- **One URL for everything.** The dashboard and the webhooks now share
  `config.webUrl` on the one public port: webhooks arrive at
  `<webUrl>/hooks/<app name>`. Update each GitHub App's webhook URL, and
  each OAuth client's callback URL to `<webUrl>/auth/callback/oidc` or
  `<webUrl>/auth/callback/github`.
- **Secrets come from the environment only.** `{ file: ... }` and
  `{ sealed: ... }` references are gone, along with `secretMounts`: every
  secret is `{ env: NAME }`, set by the chart's `env` or `envFrom` from an
  existing Secret.
- **Metrics labels changed.** Series that carried `tenant` now carry
  `account`, and `installation` is now `connection`. Alerts and
  dashboards written against the old labels need updating; the chart now
  ships its own alerts and Grafana dashboard
  (`monitoring.prometheusRule`, `monitoring.dashboards`).

## Chart values

### Removed or moved

| Old value                                                            | New value                                                                                                                                                     |
| -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `roles.all.enabled`, `roles.ingest.*`, `roles.worker.*`, `roles.web.*` | removed: one Deployment of `kritique serve` serves webhooks, the dashboard, the job queues and the gateway                                                  |
| `roles.all.replicas`                                                 | `replicas` (default 1)                                                                                                                                        |
| `roles.<role>.resources`                                             | `resources`                                                                                                                                                   |
| `web.url`                                                            | `config.webUrl`, now **required**                                                                                                                             |
| `web.port`, `service.webPort`                                        | removed: the dashboard is served on `service.port` (8080) with the webhooks                                                                                  |
| `config.file`                                                        | `configFile` (top level), in the new layout ([below](#configuration-file))                                                                                    |
| `config.existingConfigMap`                                           | `existingConfigMap` (top level); a change to it now takes a restart                                                                                          |
| `config.reloadInterval`                                              | removed: the file is read once at startup; the pods carry a checksum of `configFile`, so a change to it rolls them                                           |
| `config.logLevel`, `config.logFormat`                                | unchanged path, `config.logLevel`, `config.logFormat`                                                                                                         |
| `config.reviewWorkers` (2), `config.indexWorkers` (1)                | unchanged path; the default is now empty, which means the same 2 and 1                                                                                        |
| `config.extraEnv` (a list)                                           | `env` (a map keyed by variable name, a plain value or a `valueFrom`) and `envFrom`. A `KRITIQUE_*` variable is refused there: it is a key of `config`, in camelCase |
| `embedding.model`, `embedding.dims`                                  | `configFile.embedding.model` (now `<provider>/<model>`) and `.dims`, or `config.embeddingModel` and `config.embeddingDims`                                    |
| `embedding.baseUrl`, `embedding.apiKey`, `embedding.existingSecret`, `embedding.existingSecretKey` | a `providers` entry the embedding model names: its `baseUrl`, and `apiKey: { env: NAME }` set through `env` from the Secret              |
| `embedding.maxBatch`, `embedding.maxBatchChars`, `embedding.maxItemChars` | `configFile.embedding.maxBatch`, `.maxBatchChars`, `.maxItemChars`                                                                                    |
| `embedding.reindexOnModelChange`                                     | removed: changing the embedding model or dimensions rebuilds every repository's index                                                                        |
| `secretMounts`                                                       | removed: secrets are `{ env: NAME }` from `env` or `envFrom`                                                                                                  |
| `runner.image` (a string; empty meant the chart's own image)         | `runner.image.repository`, `.tag`, `.digest`, `.pullPolicy`; the default is now the release's `-tools` image, `ghcr.io/perfectra1n/kritique:<appVersion>-tools` |
| `runner.runtimeClassName`                                            | `config.runnerRuntimeClass`                                                                                                                                   |
| `runner.ttl`                                                         | unchanged path; the default drops from `1h` to `10m`                                                                                                          |
| `gateway.enabled`                                                    | removed: the gateway always runs                                                                                                                              |
| `gateway.port`                                                       | `service.gatewayPort` (8082)                                                                                                                                  |
| `ingress.web.*`, `httpRoute.web.*`                                   | removed: one Ingress or HTTPRoute serves `webUrl`'s host; the default path is now `/` instead of `/hooks`                                                    |
| `dashboard.keySecret.*`, `dashboard.oldKeysSecret.*`                 | removed with dashboard-managed tenants; nothing is sealed any more, so `KRITIQUE_DASHBOARD_KEY` is no longer read                                            |
| `podDisruptionBudget.enabled`                                        | unchanged path; now `true` by default, rendering only with more than one replica                                                                             |
| `terminationGracePeriodSeconds`                                      | unchanged path; the default rises from 150 to 200                                                                                                             |
| `networkPolicy.egressPorts`                                          | unchanged path; runner pods never get these ports now, only the gateway                                                                                      |
| `database.owner.existingSecret`, `database.runner.existingSecret`    | unchanged path; now required in every install                                                                                                                 |

Unchanged: `image.*`, `imagePullSecrets` (now also attached to the runner
ServiceAccount), `nameOverride`, `fullnameOverride`, `database.*`,
`runner.serviceAccount.*`, `serviceAccount.*`, `rbac.create`,
`deploymentAnnotations`, `podAnnotations`, `podLabels`,
`podSecurityContext`, `securityContext`, `service.type`, `service.port`,
`service.metricsPort`, `ingress.enabled`, `.className`, `.annotations`,
`.hosts`, `.tls`, `httpRoute.enabled`, `.apiVersion`, `.annotations`,
`.labels`, `.parentRefs`, `.hostnames`, `.matches`, `networkPolicy.enabled`,
`.allowDNS`, `.postgresPort`, the three probes, `monitoring.serviceMonitor.*`,
`podDisruptionBudget.maxUnavailable`, `nodeSelector`, `tolerations`,
`affinity`, `priorityClassName`, `volumes` and `volumeMounts`.

### Added

| Value                                                         | What                                                                                                                                                       |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `replicas`, `strategy`, `topologySpreadConstraints`           | the one Deployment's size, rolling update and spread                                                                                                       |
| `config.*`                                                    | every `KRITIQUE_*` variable the chart does not derive, in camelCase; see the [chart README](../charts/kritique/README.md) and [Configuration](configuration.md) |
| `configFile`, `existingConfigMap`                             | the configuration file, inline or from a ConfigMap                                                                                                         |
| `env`, `envFrom`                                              | the container's environment, where the file's `{ env: NAME }` secrets come from                                                                             |
| `runner.image.*`                                              | the runner image, digest-pinned by the release                                                                                                             |
| `runner.resources`                                            | runner pod resources (was the file's `defaults.runner.resources` and a tenant's `runner.resources`)                                                       |
| `runner.tools`                                                | tools mounted from images for `agent.commands` (was the file's `tools`, same entries: `name`, `image`, `path`, `commands`); needs Kubernetes 1.33 or newer |
| `service.gatewayPort`                                         | the gateway's port                                                                                                                                         |
| `httpRoute.kind`, `.additionalRules`, `.filters`, `.httpsRedirect` | HTTPRoute options                                                                                                                                      |
| `networkPolicy.type`                                          | `default`, `cilium` or `calico` policies                                                                                                                   |
| `monitoring.prometheusRule.*`                                 | the chart's alerts: no leader, more than one leader, a configuration the leader could not apply, configuration drift                                       |
| `monitoring.dashboards.*`                                     | the chart's Grafana dashboard, as a ConfigMap or a grafana-operator `GrafanaDashboard`                                                                     |
| `podDisruptionBudget.minAvailable`                            | the alternative to `maxUnavailable`                                                                                                                        |
| `tests.image.*`                                               | the `helm test` pod's image                                                                                                                                |

### Required now

- `config.webUrl`.
- `database.app.existingSecret`, `database.owner.existingSecret` and
  `database.runner.existingSecret`.
- A way to sign in that can make an admin: `config.authAdminPassword`, or
  an OIDC or GitHub sign-in with a role mapping that yields `"admin"`
  ([sign-in](#sign-in-web-becomes-auth)). Without one, kritique refuses
  to start.

## Configuration file

The file's top level is now:

```yaml
auth: {}          # sign-in (was web)
apps: {}          # forge connections (was tenants[].installations)
providers: {}     # model keys, as before
embedding: {}     # the embedder (was the chart's embedding values)
egress: {}        # runner egress through the gateway
# the repository settings every repository gets, at the root (was defaults)
review: {}
confidence: {}
trigger: {}
comments: {}
agent: {}
rules: []
context: []
skills: {}
ignore: []
enabled: true
limits: {}        # every account's limits (was defaults.limits)
repositories: {}  # owner/* and owner/name entries (was tenants[].repositories)
accounts: {}      # per-account limits and providers (was tenants)
```

### Top-level keys

| Old key                                    | New                                                                                                                                                     |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tenants`                                  | removed. An account (`github/<name>`) is now the unit of isolation; see [tenants and installations](#tenants-and-installations)                        |
| `defaults`                                 | removed as a wrapper: its repository settings move to the file's root, `defaults.limits` to the root's `limits`                                       |
| `defaults.runner.activeDeadlineSeconds`, a tenant's `runner.activeDeadlineSeconds` | `KRITIQUE_RUNNER_DEADLINE` (`config.runnerDeadline`), a duration, for every runner; 15m unless set, at most 2h       |
| `defaults.runner.resources`, a tenant's `runner.resources` | the chart's `runner.resources`, for every runner                                                                                        |
| `providers`                                | kept; see [providers](#providers-and-the-embedder)                                                                                                      |
| `polling.interval`, `polling.lookback`     | `KRITIQUE_POLL_INTERVAL`, `KRITIQUE_POLL_LOOKBACK` (`config.pollInterval`, `config.pollLookback`)                                                       |
| `indexing.onboardWindow`                   | `KRITIQUE_ONBOARD_WINDOW` (`config.onboardWindow`)                                                                                                      |
| `retention.disabledIndexGrace`             | `KRITIQUE_INDEX_GRACE` (`config.indexGrace`)                                                                                                            |
| `retention.transcripts`                    | `KRITIQUE_TRANSCRIPT_RETENTION` (`config.transcriptRetention`); new beside it, `KRITIQUE_DIFF_RETENTION` (`config.diffRetention`)                       |
| `tools`                                    | the chart's `runner.tools` (`KRITIQUE_RUNNER_TOOLS`), same entries                                                                                      |
| `egress.allowHosts`                        | `egress.allow`; see [egress](#egress)                                                                                                                   |
| `egress.credentials`                       | kept, `{ env: NAME }` only                                                                                                                              |
| `web`                                      | `auth`; see [sign-in](#sign-in-web-becomes-auth)                                                                                                        |
| (new) `apps`, `embedding`, `repositories`, `accounts`, the root's repository settings | see below                                                                                                    |

### Tenants and installations

A tenant grouped installations, one per forge account, with limits and
settings for all of them. Now:

- **A GitHub installation** becomes an entry of `apps`, keyed by its name,
  which stays its webhook path. One App serving several accounts is one
  entry listing them under `accounts`, instead of an installation per
  account repeating the App's credentials.

  | Old installation key            | New `apps.<name>` key                                       |
  | ------------------------------- | ----------------------------------------------------------- |
  | `name`                          | the entry's key; the webhook path `/hooks/<name>`            |
  | `forge: github`                 | implied                                                     |
  | `account`                       | an item of `accounts`                                       |
  | `host`                          | removed for GitHub: github.com only                         |
  | `app.clientId`, `app.clientIdFrom` | `clientId`, inline or `{ env: NAME }`                    |
  | `app.privateKey`                | `privateKey: { env: NAME }`                                 |
  | `app.webhookSecret`             | `webhookSecret: { env: NAME }`                              |

- **A Forgejo or Gitea installation** (`forge: forgejo` or `gitea`, with
  `host`, `token`, `gitToken` and `webhookSecret`):

  | Old                     | New                                                              |
  | ----------------------- | ---------------------------------------------------------------- |
  | `forge`                 | `forge: forgejo` or `forge: gitea` (`github` is the default)     |
  | `host`                  | `url`, the forge's base URL, e.g. `https://git.example.com`      |
  | `account`               | an item of `accounts`                                            |
  | `token`                 | `token: { env: NAME }`                                           |
  | `gitToken`              | `gitToken: { env: NAME }`, optional read-only token for runners  |
  | `webhookSecret`         | `webhookSecret: { env: NAME }`                                   |

  The webhook URL becomes `<webUrl>/hooks/<app name>`. Each app also has
  `KRITIQUE_APPS_FORGE`, `_URL`, `_TOKEN`, `_GIT_TOKEN` and
  `_WEBHOOK_SECRET` variables for the environment's app. The forge's host
  is always allowed through egress on 443; a forge on a private address
  or another port also needs an `egress.allow` entry (and the port in
  `networkPolicy.egressPorts`). Account ids are `forgejo/<owner>` or
  `gitea/<owner>`, as before, so existing rows match.

  ```yaml
  apps:
    home-forgejo:
      forge: forgejo
      url: https://git.example.com
      accounts: [perfectra1n, my-org]
      token: { env: FORGEJO_TOKEN }
      gitToken: { env: FORGEJO_READ_TOKEN }
      webhookSecret: { env: FORGEJO_WEBHOOK_SECRET }
  ```

- **A tenant's limits** move to `accounts.<account name>.limits`, one per
  forge account, with the same keys: `concurrency`, `reviewsPerDay`,
  `tokensPerMonth`. A tenant of several accounts shared one budget; each
  account now has its own. The root's `limits` sets every account's, and
  an account's entry replaces it key by key.
- **A tenant's repository settings** move to a `repositories` entry keyed
  `owner/*` for each of its accounts.
- **A tenant's `repositories` list** becomes the `repositories` map, keyed
  `owner/name`.
  - `installation` (which installation an entry belongs to) is removed:
    an entry belongs to the app that serves its owner.

    The same owner name on two forges is one account, so the
    configuration refuses an owner served by two apps.

  - `enabled` may no longer be set on an `owner/name` entry, only at the
    root or on `owner/*`: it says where repositories start, and an admin
    turns a single repository on or off on the dashboard's Repositories
    page.
- **Dashboard-managed tenants** are removed. The dashboard no longer
  writes configuration; everything comes from the file and the
  environment.
- **Per-account providers** are new: `accounts.<name>.providers`, keys an
  account's repositories can name.

### Repository settings

These are the keys of the root, of each `repositories` entry, and of the
old `defaults`, tenant and repository scopes. A narrower scope still
replaces a broader one key by key; the order is now kritique's default,
the root, `owner/*`, `owner/name`, then the repository's `.kritique.yaml`.

| Old key                                          | New key                                                                                                                                                  |
| ------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `models.review`                                  | `review.model`                                                                                                                                           |
| `models.fallback`                                | `review.fallback`                                                                                                                                        |
| (new)                                            | `review.effort`: `none`, `minimal`, `low`, `medium`, `high`, `xhigh` or `max`                                                                           |
| `mode: single \| agentic`                        | removed: every review is the agent's tool loop; `agent.steps: 1` is one model call, closest to `single`                                                 |
| `filter` (one CEL expression; true reviews)      | `trigger.include` and `trigger.exclude`, lists of conditions, each an `expr`, `paths` or both, with an optional `name`. Invert the old sense: `filter: "!pr.draft"` is `trigger: { exclude: [{ expr: pr.draft }] }`. Expressions still see `pr`; `pr.lines` and `pr.fork` are new |
| `forks` (unset or `false` skipped forks)        | removed. A fork's pull request is now reviewed unless excluded: keep the old default with `trigger.exclude: [{ name: forks, expr: pr.fork }]`; `forks: true` needs nothing |
| `settle`                                         | `trigger.settle`                                                                                                                                         |
| (new)                                            | `trigger.limit`: automatic reviews per pull request before they pause                                                                                   |
| `incremental.maxDeltaFiles`                      | `review.incremental`                                                                                                                                     |
| `review.requireSuggestedFix`                     | `review.fixes`                                                                                                                                           |
| `review.instructions` (paths, or `{ path, paths }`) | `rules`: each `{ id, file, paths }` for a file, or `{ id, rule }` inline, with optional `when` conditions. Rules add up by `id` across scopes          |
| `review.templates.summary`                       | `comments.summary`                                                                                                                                       |
| `review.templates.inline`                        | `comments.finding`; the template variables changed, see [comment templates](repository-config.md#comment-templates)                                    |
| `review.inlineComments`                          | `comments.inline`                                                                                                                                        |
| `review.minSeverity`                             | removed, no replacement                                                                                                                                  |
| `review.context`                                 | `context` at the same level, same entries (`path`, `description`, `paths`)                                                                              |
| `ignore`                                         | kept; globs still add up across scopes                                                                                                                   |
| `agent.maxSteps`                                 | `agent.steps`                                                                                                                                            |
| `agent.maxToolOutputBytes`                       | `agent.output`                                                                                                                                           |
| `agent.maxTokens`                                | `agent.tokens`                                                                                                                                           |
| `agent.timeout`, `agent.commands`, `agent.commandTimeout` | unchanged                                                                                                                                       |
| (new)                                            | `agent.prompt`, `agent.parts`                                                                                                                            |
| `allow.*` (`modes`, `models`, `commands`, `agent`, `settle`, `tasks`) | removed. A `.kritique.yaml` may name any model of a provider its account can use; `agent`, `trigger.settle`, `trigger.limit`, `review.incremental` and `review.cost` are the admin's alone |
| (new)                                            | `review.approve`, `review.diagram`, `review.cost`                                                                                                       |
| (new)                                            | `confidence.*`: `model`, `fallback`, `effort`, `threshold`, `gate`, `risk`, `instructions` ([Confidence](confidence.md))                                |
| (new)                                            | `skills.paths`, `skills.scope`                                                                                                                           |
| `tasks`, `allow.tasks`                           | `tasks` and `taskBounds`: see below                                                                                                                      |

Event tasks keep their shape but move with the rest of the layout
([Event tasks](tasks.md#changes-from-earlier-kritique) has the full list
and an example):

| Old                                                                   | New                                                                                                   |
| --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `defaults.tasks`, `tenants[].tasks`, `repositories[].tasks`           | `tasks` at the root, or under a `repositories` entry (`owner/*`, `owner/name`); merged by name, `enabled: false` turns an inherited one off |
| `allow.tasks.*`                                                       | `taskBounds.*` at any scope; `repositoryTasks` becomes `repository`                                   |
| `if: 'expr'` (on a task, an action or a rule)                         | `when: [{ name?, expr }]`; any one holding is enough                                                  |
| `agent.maxSteps`, `maxToolOutputBytes`, `maxTokens`                   | `agent.steps`, `output`, `tokens`                                                                     |
| a repository task's model held to `allow.models`                      | any model of a provider the account may use                                                           |
| agent limits, modes and commands held to `allow.agent`/`modes`/`commands` | held to the admin's own `agent` settings and `agent.commands`; any mode                          |
| tasks written in the dashboard                                        | removed: tasks come from the file and `.kritique.yaml`                                                |

### Providers and the embedder

- `providers.<name>.type`, `baseUrl` and `pricing` are unchanged.
  `apiKey` takes `{ env: NAME }` only. `type` gains `opencode`, and
  `retries` is new.
- The embedder moves from the chart's `embedding` values and the
  `KRITIQUE_EMBED_*` variables into the file's `embedding`: `model` is now
  `<provider>/<model>`, a provider the file declares, whose `baseUrl` and
  `apiKey` it uses; `dims`, `maxBatch`, `maxBatchChars` and `maxItemChars`
  are as before; `similarFloor` is new. See [Models](models.md).

### Egress

| Old                                 | New                                                                                                                                                                      |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `egress.allowHosts`                 | `egress.allow`: hosts (exact, `*.`-prefixed, or `"*"` for every host), and now IP addresses and CIDRs                                                                   |
| (new)                               | `egress.deny`: destinations refused however `allow` matches them                                                                                                        |
| `egress.credentials`                | unchanged, `{ env: NAME }` only                                                                                                                                         |
| the forge hosts of every installation, always allowed | `github.com` and `api.github.com` once an app is declared                                                                                             |

A destination on a private address (RFC 1918, loopback, link-local) is
now refused unless an `allow` entry covers that address, even when its
name is allowed. See [the egress gateway](security.md#the-egress-gateway).

### Sign-in: `web` becomes `auth`

| Old                                                   | New                                                                                                                                                                                  |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `web.signIn[]` with `type: oidc`                      | `auth.oidc`, one provider: `name`, `issuer`, `clientId`, `clientSecret`, `scopes`, and new `rolesClaim`, `roleMappingExpr`, `defaultRole`                                           |
| `web.signIn[]` with `type: github`                    | `auth.github`: `clientId`, `clientSecret`, `roleMappingExpr`; github.com only (`host` is gone)                                                                                       |
| `web.signIn[]` with `type: forgejo` or `gitea`        | `auth.forgejo` / `auth.gitea`: `url`, `clientId`, `clientSecret`, `roleMappingExpr` (`KRITIQUE_AUTH_FORGEJO_*`, `KRITIQUE_AUTH_GITEA_*`); callback `/auth/callback/forgejo` or `/gitea`                                                                                  |
| `web.signIn[].name`                                   | gone: the callback is `/auth/callback/<provider>` (`oidc`, `github`, `forgejo`, `gitea`)                                                                                                              |
| `web.operators`                                       | a `roleMappingExpr` yielding `"admin"`, e.g. `login == "user-1" ? "admin" : ""` for GitHub, or `"kritique-admins" in roles ? "admin" : ""` for OIDC; members get `"member"` or a per-account map |
| (new)                                                 | `auth.admin.user`, `auth.admin.password`: a local admin, the way into a fresh instance                                                                                             |
| `web.sessionTTL`                                      | `auth.sessionTTL`                                                                                                                                                                    |
| `web.dashboardForgeHosts`                             | removed with dashboard-managed tenants                                                                                                                                              |

Every `auth` key also has a `KRITIQUE_AUTH_*` variable, a key of the
chart's `config` ([`auth`](configuration.md#auth)).

## A repository's `.kritique.yaml`

Repositories with their own file need the same renames:

| Old key                                                         | New key                                                                                         |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `enabled`                                                       | unchanged; can only turn the repository off                                                     |
| `models.review`, `models.fallback`                              | `review.model`, `review.fallback`                                                               |
| `mode`, `agent.*`, `settle`                                     | removed: the admin's alone                                                                      |
| `filter`                                                        | `trigger.include`, `trigger.exclude`, judged beside the admin's lists                          |
| `skip.onlyPaths`                                                | `ignore`: a pull request that changes only ignored paths is skipped                             |
| `ignore`                                                        | unchanged                                                                                       |
| `review.instructions`                                           | `rules`, each `{ id, file }` or `{ id, rule }`                                                  |
| `review.requireSuggestedFix`                                    | `review.fixes`, which can only turn it on                                                       |
| `review.templates.summary`, `review.templates.inline`           | `comments.summary`, `comments.finding`                                                          |
| `review.inlineComments`                                         | `comments.inline`                                                                               |
| `review.minSeverity`                                            | removed                                                                                         |
| `review.context`                                                | `context`, added to the admin's                                                                 |
| `tasks`                                                         | `tasks`, within the admin's `taskBounds` (see [Event tasks](tasks.md))                          |
| (new)                                                           | `review.effort`, `review.approve`, `review.diagram`, `confidence.*`, `skills`                   |

[`kritique.schema.json`](kritique.schema.json) validates the new file in
an editor.

## Environment variables

| Old variable                                                                       | New                                                                                                  |
| ---------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| `KRITIQUE_CONFIG_RELOAD_INTERVAL`                                                  | removed: the file is read at startup                                                                 |
| `KRITIQUE_WEB_ADDR`                                                                | removed: the dashboard is on `KRITIQUE_ADDR`                                                         |
| `KRITIQUE_EMBED_MODEL`, `KRITIQUE_EMBED_DIMS`                                      | `KRITIQUE_EMBEDDING_MODEL` (`<provider>/<model>`), `KRITIQUE_EMBEDDING_DIMS`                         |
| `KRITIQUE_EMBED_BASE_URL`, `KRITIQUE_EMBED_API_KEY`                                | the embedding provider's `baseUrl` and `apiKey`, or `KRITIQUE_PROVIDERS_BASE_URL`, `KRITIQUE_PROVIDERS_API_KEY` |
| `KRITIQUE_EMBED_MAX_BATCH`, `KRITIQUE_EMBED_MAX_BATCH_CHARS`, `KRITIQUE_EMBED_MAX_ITEM_CHARS` | the file's `embedding.maxBatch`, `.maxBatchChars`, `.maxItemChars`                        |
| `KRITIQUE_REINDEX_ON_MODEL_CHANGE`                                                 | removed: a change reindexes                                                                          |
| `KRITIQUE_DASHBOARD_KEY`, `KRITIQUE_DASHBOARD_OLD_KEYS`                            | removed                                                                                              |
| `KRITIQUE_RUNNER_TTL`                                                              | unchanged; the chart's default is now `10m`                                                          |
| `KRITIQUE_ADDR`, `KRITIQUE_METRICS_ADDR`, `KRITIQUE_GATEWAY_ADDR`, `KRITIQUE_GATEWAY_URL`, `KRITIQUE_GATEWAY_TOKEN_TTL`, `KRITIQUE_WEB_URL`, `KRITIQUE_CONFIG_FILE`, `KRITIQUE_DATABASE_*`, `KRITIQUE_LEADER_RETRY_INTERVAL`, `KRITIQUE_REVIEW_WORKERS`, `KRITIQUE_INDEX_WORKERS`, `KRITIQUE_EXECUTOR`, `KRITIQUE_RUNNER_IMAGE`, `KRITIQUE_RUNNER_SERVICE_ACCOUNT`, `KRITIQUE_RUNNER_DATABASE_SECRET`, `KRITIQUE_RUNNER_DATABASE_SECRET_KEY`, `KRITIQUE_RUNNER_RUNTIME_CLASS`, `KRITIQUE_LOG_LEVEL`, `KRITIQUE_LOG_FORMAT` | unchanged |

New: `KRITIQUE_POLL_INTERVAL`, `KRITIQUE_POLL_LOOKBACK`,
`KRITIQUE_ONBOARD_WINDOW`, `KRITIQUE_INDEX_GRACE`,
`KRITIQUE_TRANSCRIPT_RETENTION`, `KRITIQUE_DIFF_RETENTION`,
`KRITIQUE_RUNNER_DEADLINE`, `KRITIQUE_RUNNER_IMAGE_PULL_POLICY`,
`KRITIQUE_RUNNER_RESOURCES`, `KRITIQUE_RUNNER_TOOLS`, and the overlays of
the file: `KRITIQUE_AUTH_*`, `KRITIQUE_APPS_*`, `KRITIQUE_PROVIDERS_*`,
`KRITIQUE_REVIEW_*`, `KRITIQUE_CONFIDENCE_*`, `KRITIQUE_TRIGGER_*` and
`KRITIQUE_EMBEDDING_*`. A variable under one of these prefixes that names
no key is refused at startup ([Configuration](configuration.md)).

## Example

Before:

```yaml
web:
  url: https://kritique.example.com
config:
  file:
    providers:
      openrouter:
        type: openrouter
        apiKey: { file: /var/run/secrets/kritique/providers/openrouter-api-key }
    defaults:
      runner: { activeDeadlineSeconds: 900 }
      models:
        review: openrouter/vendor/large-model
      filter: "!pr.draft"
      forks: false
      settle: 2m
      agent: { maxTokens: 1000000, commands: [curl, fd, rg] }
      limits: { reviewsPerDay: 50 }
    web:
      signIn:
        - { name: github, type: github, clientId: Iv1.example, clientSecret: { file: /var/run/secrets/kritique/bot/client-secret } }
      operators: ["github:user-1"]
    tenants:
      - slug: example
        installations:
          - name: example-github
            forge: github
            account: example
            app:
              clientId: Iv1.example
              privateKey: { file: /var/run/secrets/kritique/bot/private-key.pem }
              webhookSecret: { file: /var/run/secrets/kritique/bot/webhook-secret }
        repositories:
          - name: example/widgets
            mode: agentic
            review:
              instructions: [.kritique/review.md]
embedding:
  model: voyageai/voyage-code-4
  dims: 1024
  existingSecret: kritique-openrouter
secretMounts:
  - { name: bot, secretName: kritique-bot, mountPath: /var/run/secrets/kritique/bot }
  - { name: providers, secretName: kritique-providers, mountPath: /var/run/secrets/kritique/providers }
runner:
  image: ghcr.io/perfectra1n/kritique:0.1.0-tools
ingress:
  enabled: true
  hosts: [{ host: kritique.example.com, paths: [{ path: /hooks, pathType: Prefix }] }]
  web:
    enabled: true
    hosts: [{ host: kritique.example.com, paths: [{ path: /, pathType: Prefix }] }]
```

After:

```yaml
config:
  webUrl: https://kritique.example.com
  runnerDeadline: 15m
configFile:
  auth:
    github:
      clientId: Iv1.example
      clientSecret: { env: GITHUB_CLIENT_SECRET }
      roleMappingExpr: 'login == "user-1" ? "admin" : ""'
  apps:
    example-github:
      accounts: [example]
      clientId: Iv1.example
      privateKey: { env: GITHUB_APP_PRIVATE_KEY }
      webhookSecret: { env: GITHUB_APP_WEBHOOK_SECRET }
  providers:
    openrouter: { type: openrouter, apiKey: { env: OPENROUTER_API_KEY } }
  embedding:
    model: openrouter/voyageai/voyage-code-4
    dims: 1024
  review:
    model: openrouter/vendor/large-model
  trigger:
    settle: 2m
    exclude:
      - expr: pr.draft
      - { name: forks, expr: pr.fork }
  agent: { tokens: 1000000, commands: [curl, fd, rg] }
  limits: { reviewsPerDay: 50 }
  repositories:
    example/widgets:
      rules: [{ id: house-style, file: .kritique/review.md }]
env:
  OPENROUTER_API_KEY:
    valueFrom:
      secretKeyRef: { name: kritique-providers, key: openrouter-api-key }
# kritique-bot holds the keys GITHUB_CLIENT_SECRET, GITHUB_APP_PRIVATE_KEY
# and GITHUB_APP_WEBHOOK_SECRET.
envFrom:
  - secretRef:
      name: kritique-bot
ingress:
  enabled: true
  hosts: [{ host: kritique.example.com, paths: [{ path: /, pathType: Prefix }] }]
```

The runner image now defaults to the release's own `-tools` image, so
`runner.image` is left out, and the GitHub App's webhook URL becomes
`https://kritique.example.com/hooks/example-github`.
