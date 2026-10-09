-- The rest of kritika's schema, from what kritika gained while kritique ran
-- its own migrations: each existing row gets the value kritika's code
-- reads for "before this existed", and a value kritika no longer has is
-- turned into the one it now records instead. kritique's own task values
-- stay allowed beside kritika's in every check they share.

-- Not account-scoped: written by the leader, read by every replica to
-- report configuration drift. Only the hash is kept.
ALTER TABLE config_state DROP COLUMN applied_at, DROP COLUMN leader;

-- Repositories: what the forge last said of one, an admin's choice to
-- review it or not, NULL until one is made (the configuration decides until
-- then), and when one that holds an index was first found not running, so
-- the leader drops the index once it is older than KRITIQUE_INDEX_GRACE. A
-- repository added from the dashboard is one the forge reported, which is
-- how kritika keeps a repository the configuration file no longer lists.
UPDATE repositories SET managed_by = 'forge' WHERE managed_by = 'dashboard';
ALTER TABLE repositories DROP CONSTRAINT repositories_managed_by_check;
ALTER TABLE repositories ADD CONSTRAINT repositories_managed_by_check CHECK (managed_by IN ('file', 'forge'));
ALTER TABLE repositories
    ADD COLUMN archived   boolean NOT NULL DEFAULT false,
    ADD COLUMN fork       boolean NOT NULL DEFAULT false,
    ADD COLUMN turned_on  boolean,
    ADD COLUMN turned_at  timestamptz,
    ADD COLUMN stopped_at timestamptz;
UPDATE repositories SET stopped_at = disabled_at WHERE NOT enabled AND active_index_run_id IS NOT NULL;
-- Forge names are not case sensitive: one row per name however it is
-- spelled, which the id, derived from the lowercased name, also keys.
CREATE UNIQUE INDEX repositories_account_lower_name_key ON repositories (account_id, lower(name));
DROP INDEX repositories_account_id_idx;

-- Pull requests: their state is open or closed, merged or not; when one
-- closed (taken, for one closed before this, as its last update); when
-- the forge last changed it, so a late or repeated event cannot rewind the
-- head; and whether its automatic reviews are paused, by a maintainer or by
-- auto_reviews reaching the repository's maxAutoReviews.
UPDATE pull_requests SET state = 'closed' WHERE state <> 'open';
ALTER TABLE pull_requests ADD CONSTRAINT pull_requests_state_check CHECK (state IN ('open', 'closed'));
ALTER TABLE pull_requests DROP COLUMN base_sha;
ALTER TABLE pull_requests
    ADD COLUMN closed_at        timestamptz,
    ADD COLUMN forge_updated_at timestamptz,
    ADD COLUMN paused           boolean NOT NULL DEFAULT false,
    ADD COLUMN auto_reviews     int     NOT NULL DEFAULT 0;
UPDATE pull_requests SET closed_at = updated_at WHERE state = 'closed';
DROP INDEX pull_requests_account_id_idx;

-- Reviews: every review is an agent loop, and the audit log, not the
-- review, records who canceled one. A skip records why whoever decided it;
-- confidence is the score a second model gave, NULL where none was asked
-- for or given; a partial review is a split one some of whose parts ended
-- before they submitted.
ALTER TABLE reviews DROP COLUMN mode, DROP COLUMN canceled_by;
ALTER TABLE reviews DROP CONSTRAINT reviews_skip_reason_check;
ALTER TABLE reviews ADD CONSTRAINT reviews_skip_reason_check
    CHECK (skip_reason IN ('', 'disabled', 'filtered', 'only_skipped_paths', 'unchanged_patch', 'too_large'));
ALTER TABLE reviews
    ADD COLUMN confidence jsonb,
    ADD COLUMN partial    boolean NOT NULL DEFAULT false;
DROP INDEX reviews_account_id_idx;
CREATE INDEX reviews_account_created_idx ON reviews (account_id, created_at DESC, id DESC);
CREATE INDEX reviews_prior_review_idx ON reviews (prior_review_id) WHERE prior_review_id IS NOT NULL;

-- Runner runs: a follow-up is answered by an agent in a runner of its own
-- kind; a run belongs to a review or to an index generation, never to the
-- other kind's parent; and river_job_id is the River job that started it,
-- so a run left by a replica that died can be found and its Kubernetes Job
-- deleted.
UPDATE runner_runs SET phase = 'failed'
WHERE phase NOT IN ('created', 'fetching', 'parsing', 'writing', 'reviewing', 'done', 'failed');
ALTER TABLE runner_runs ADD CONSTRAINT runner_runs_phase_check
    CHECK (phase IN ('created', 'fetching', 'parsing', 'writing', 'reviewing', 'done', 'failed'));
ALTER TABLE runner_runs DROP CONSTRAINT runner_runs_kind_check;
ALTER TABLE runner_runs ADD CONSTRAINT runner_runs_kind_check CHECK (kind IN ('review', 'index', 'followup', 'task'));
ALTER TABLE runner_runs ADD CONSTRAINT runner_runs_kind_parent_check
    CHECK ((kind = 'review' OR review_id IS NULL) AND (kind = 'index' OR index_run_id IS NULL));
ALTER TABLE runner_runs ADD COLUMN river_job_id bigint;
CREATE INDEX runner_runs_index_run_idx ON runner_runs (index_run_id) WHERE index_run_id IS NOT NULL;
CREATE INDEX runner_runs_unfinished_job_idx ON runner_runs (river_job_id) WHERE finished_at IS NULL;

-- A replica's own sign of life on a River job it is working, so the
-- leader rescues a job whose replica died rather than waiting out River's
-- longest timeout. Not account-scoped, as River's own rows are not.
CREATE TABLE job_heartbeats (
    job_id       bigint      PRIMARY KEY,
    heartbeat_at timestamptz NOT NULL DEFAULT now()
);

-- Context packs: the stages are a list; whether the review built on the
-- prior one, why the runner ran no agent and which condition decided, the
-- rules the prompt was given, and when the retention sweep emptied the
-- bodies.
UPDATE context_packs SET stages = '[]'::jsonb WHERE jsonb_typeof(stages) <> 'array';
ALTER TABLE context_packs ALTER COLUMN stages SET DEFAULT '[]'::jsonb;
ALTER TABLE context_packs
    ADD COLUMN scope        text   NOT NULL DEFAULT 'full' CHECK (scope IN ('full', 'incremental')),
    ADD COLUMN scope_reason text   NOT NULL DEFAULT '',
    ADD COLUMN skip_reason  text   NOT NULL DEFAULT ''
        CHECK (skip_reason IN ('', 'only_skipped_paths', 'unchanged_patch', 'too_large', 'filtered')),
    ADD COLUMN rule_ids     text[] NOT NULL DEFAULT '{}',
    ADD COLUMN swept_at     timestamptz,
    ADD COLUMN skip_detail  text   NOT NULL DEFAULT '';
CREATE INDEX context_packs_unswept_idx ON context_packs (created_at) WHERE swept_at IS NULL;

-- A finding's category, the thumbs up and down on its inline comment, and the
-- review rules it enforces.
ALTER TABLE findings
    ADD COLUMN category       text   NOT NULL DEFAULT '',
    ADD COLUMN reactions_up   int    NOT NULL DEFAULT 0,
    ADD COLUMN reactions_down int    NOT NULL DEFAULT 0,
    ADD COLUMN rules          text[] NOT NULL DEFAULT '{}';

-- The findings a maintainer dismissed by replying "@<bot> dismiss
-- <reason>" in a finding's thread, one per pull request and fingerprint,
-- kept as the finding last read so a later review is told not to raise it
-- again.
CREATE TABLE dismissals (
    account_id      uuid        NOT NULL REFERENCES accounts (id),
    pull_request_id uuid        NOT NULL REFERENCES pull_requests (id),
    fingerprint     text        NOT NULL,
    path            text        NOT NULL DEFAULT '',
    line            int         NOT NULL DEFAULT 0,
    severity        text        NOT NULL DEFAULT '',
    title           text        NOT NULL DEFAULT '',
    explanation     text        NOT NULL DEFAULT '',
    reason          text        NOT NULL DEFAULT '',
    author          text        NOT NULL DEFAULT '',
    comment_id      bigint      NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (pull_request_id, fingerprint)
);
CREATE INDEX dismissals_account_idx ON dismissals (account_id);
ALTER TABLE dismissals ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_isolation ON dismissals
    USING      (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid)
    WITH CHECK (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid);

-- Usage and transcripts: a fallback's call is charged as the review's, a
-- plain completion review's calls are agent steps, and the confidence
-- scorer's and a split review's merge call are their own.
UPDATE usage SET role = 'review' WHERE role = 'fallback';
ALTER TABLE usage DROP CONSTRAINT usage_role_check;
ALTER TABLE usage ADD CONSTRAINT usage_role_check
    CHECK (role IN ('review', 'embedding', 'followup', 'confidence', 'task'));
CREATE INDEX usage_repository_id_idx ON usage (repository_id) WHERE repository_id IS NOT NULL;

UPDATE model_calls SET kind = 'agent_step' WHERE kind IN ('review', 'fallback');
ALTER TABLE model_calls DROP CONSTRAINT model_calls_kind_check;
ALTER TABLE model_calls ADD CONSTRAINT model_calls_kind_check
    CHECK (kind IN ('agent_step', 'followup', 'confidence', 'merge', 'task'));
-- carried_from names the run whose transcript an agent step's delta is
-- against when its run carried that one on; part is the part of a split
-- review the step belongs to, from 1, or 0.
ALTER TABLE model_calls
    ADD COLUMN carried_from uuid REFERENCES runner_runs (id),
    ADD COLUMN part         int  NOT NULL DEFAULT 0;

-- Agent runs: a submit_review the output cap cut off ends a run as
-- truncated; the skills and commands the agent was offered and used, the
-- conversation it carried on, and how each part of a split review ended.
ALTER TABLE agent_runs DROP CONSTRAINT agent_runs_stop_reason_check;
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_stop_reason_check
    CHECK (stop_reason IN ('submitted', 'max_steps', 'budget', 'no_submit', 'truncated', 'canceled', 'error', 'skipped'));
ALTER TABLE agent_runs
    ADD COLUMN skills_offered   text[] NOT NULL DEFAULT '{}',
    ADD COLUMN skills_opened    text[] NOT NULL DEFAULT '{}',
    ADD COLUMN commands_offered text[] NOT NULL DEFAULT '{}',
    ADD COLUMN commands_run     text[] NOT NULL DEFAULT '{}',
    ADD COLUMN continued_from   uuid   REFERENCES runner_runs (id),
    ADD COLUMN parts            jsonb  NOT NULL DEFAULT '[]'::jsonb;

-- A review agent's conversation with its model, written by its runner, so
-- the pull request's next re-review can carry it on while the provider
-- still caches it. It is kept unmasked, since a cached prefix must match
-- byte for byte: the runner keeps none that holds its run's secrets, no
-- dashboard view reads it, and the leader deletes it within hours.
CREATE TABLE agent_conversations (
    runner_run_id uuid        PRIMARY KEY REFERENCES runner_runs (id),
    account_id    uuid        NOT NULL REFERENCES accounts (id),
    session       text        NOT NULL,
    conversation  text        NOT NULL,
    tokens        bigint      NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    model         text        NOT NULL DEFAULT ''
);
CREATE INDEX agent_conversations_created_at_idx ON agent_conversations (created_at);
ALTER TABLE agent_conversations ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_isolation ON agent_conversations
    USING      (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid)
    WITH CHECK (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid);
CREATE POLICY runner_job ON agent_conversations
    USING      (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid)
    WITH CHECK (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid
                AND account_id = (SELECT r.account_id FROM runner_runs r WHERE r.id = runner_run_id));

-- Gateway tokens: a follow-up's token names the comment it answers, and a
-- review only when the pull request has a completed one; a token carries
-- its model's reasoning effort, and the run whose conversation it carries
-- on with that conversation's session. A task's token names its task run
-- and nothing else.
ALTER TABLE gateway_tokens
    ADD COLUMN followup_comment_id bigint,
    ADD COLUMN effort              text NOT NULL DEFAULT '',
    ADD COLUMN continues           uuid REFERENCES runner_runs (id),
    ADD COLUMN session             text NOT NULL DEFAULT '';
ALTER TABLE gateway_tokens DROP CONSTRAINT gateway_tokens_subject_check;
ALTER TABLE gateway_tokens ADD CONSTRAINT gateway_tokens_subject_check
    CHECK (task_run_id IS NULL OR (review_id IS NULL AND followup_comment_id IS NULL));
CREATE INDEX gateway_tokens_review_id_idx ON gateway_tokens (review_id);

-- An index pack's commit is its index run's.
ALTER TABLE index_packs DROP COLUMN commit_sha;

-- An account's audit log reads newest first by id; its follow-ups and index
-- runs are read through their (account_id, created_at DESC, id DESC)
-- indexes, which cover what the plain account_id ones did.
DROP INDEX audit_events_account_at_idx;
CREATE INDEX audit_events_account_id_idx ON audit_events (account_id, id DESC);
DROP INDEX followups_account_id_idx;
DROP INDEX index_runs_account_id_idx;

-- The leader's onboarding feeder asks River's table, every half minute,
-- which repositories have an index job queued, running or lately failed,
-- by the repository id in each job's args. River's own index on args is
-- GIN, which a path lookup cannot use. River's migrations run first, so
-- the table exists here.
CREATE INDEX river_job_index_repository_idx ON river_job ((args->>'repository_id')) WHERE kind = 'index';
