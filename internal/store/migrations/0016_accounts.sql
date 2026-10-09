-- Tenants become forge accounts and installations become connections
-- (kritika's model). An account is "<forge>/<name>", the forge account an
-- installation served; a connection is one forge credential serving the
-- accounts it lists. The code derives the ids of accounts, connections and
-- repositories from their names (configfile.AccountID, Connection.ID,
-- RepositoryID) and addresses rows by them without a lookup, so every
-- existing row is re-keyed to the id its name derives, and every row that
-- carried a tenant now carries the account its repository belongs to.
--
-- kritique's forges stay allowed beside GitHub (0010_gitea).

-- UUIDv5 (SHA-1) as github.com/google/uuid's NewSHA1 computes it. Core
-- Postgres has no SHA-1 and an extension may not be installable by the
-- migrating role, so it is computed here, in session-temporary functions
-- dropped again at the end.
CREATE FUNCTION pg_temp.kritique_sha1(msg bytea) RETURNS bytea
LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE
    m    bytea    := msg || '\x80'::bytea;
    mask bigint   := 4294967295;
    h    bigint[] := ARRAY[1732584193, 4023233417, 2562383102, 271733878, 3285377520];
    w    bigint[];
    a bigint; b bigint; c bigint; d bigint; e bigint; f bigint; k bigint; t bigint;
    off  int;
BEGIN
    WHILE length(m) % 64 <> 56 LOOP
        m := m || '\x00'::bytea;
    END LOOP;
    m := m || int8send(length(msg)::bigint * 8);
    FOR blk IN 0 .. length(m) / 64 - 1 LOOP
        w := array_fill(0::bigint, ARRAY[80]);
        FOR i IN 0 .. 15 LOOP
            off := blk * 64 + i * 4;
            w[i + 1] := (get_byte(m, off)::bigint << 24) | (get_byte(m, off + 1)::bigint << 16)
                      | (get_byte(m, off + 2)::bigint << 8) | get_byte(m, off + 3)::bigint;
        END LOOP;
        FOR i IN 16 .. 79 LOOP
            t := w[i - 2] # w[i - 7] # w[i - 13] # w[i - 15];
            w[i + 1] := ((t << 1) | (t >> 31)) & mask;
        END LOOP;
        a := h[1]; b := h[2]; c := h[3]; d := h[4]; e := h[5];
        FOR i IN 0 .. 79 LOOP
            IF i < 20 THEN
                f := (b & c) | ((b # mask) & d); k := 1518500249;
            ELSIF i < 40 THEN
                f := b # c # d; k := 1859775393;
            ELSIF i < 60 THEN
                f := (b & c) | (b & d) | (c & d); k := 2400959708;
            ELSE
                f := b # c # d; k := 3395469782;
            END IF;
            t := ((((a << 5) | (a >> 27)) & mask) + f + e + k + w[i + 1]) & mask;
            e := d; d := c; c := ((b << 30) | (b >> 2)) & mask; b := a; a := t;
        END LOOP;
        h[1] := (h[1] + a) & mask; h[2] := (h[2] + b) & mask; h[3] := (h[3] + c) & mask;
        h[4] := (h[4] + d) & mask; h[5] := (h[5] + e) & mask;
    END LOOP;
    RETURN substr(int8send(h[1]), 5) || substr(int8send(h[2]), 5) || substr(int8send(h[3]), 5)
        || substr(int8send(h[4]), 5) || substr(int8send(h[5]), 5);
END;
$$;

-- An id under configfile's namespace, NewSHA1(NameSpaceURL,
-- "https://github.com/perfectra1n/kritique").
CREATE FUNCTION pg_temp.kritique_id(name text) RETURNS uuid
LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE
    hash bytea := pg_temp.kritique_sha1(uuid_send('5e862cfd-c44f-55f7-96ee-373805f53e80'::uuid) || convert_to(name, 'UTF8'));
BEGIN
    hash := set_byte(hash, 6, (get_byte(hash, 6) & 15) | 80);
    hash := set_byte(hash, 8, (get_byte(hash, 8) & 63) | 128);
    RETURN encode(substr(hash, 1, 16), 'hex')::uuid;
END;
$$;

-- Each installation's connection and account.
CREATE TEMP TABLE kritique_installation_map ON COMMIT DROP AS
SELECT id AS installation_id, tenant_id, name, forge, account, enabled, disabled_at, created_at, updated_at,
       pg_temp.kritique_id('connection:' || name) AS connection_id,
       pg_temp.kritique_id('account:' || lower(forge || '/' || account)) AS account_id
FROM installations;

-- Each tenant's account, for a row that names no repository: the account of
-- the installation holding most of its repositories. A tenant with no
-- installation never held a repository.
CREATE TEMP TABLE kritique_tenant_map ON COMMIT DROP AS
SELECT DISTINCT ON (m.tenant_id) m.tenant_id, m.account_id
FROM kritique_installation_map m
ORDER BY m.tenant_id, (SELECT count(*) FROM repositories r WHERE r.installation_id = m.installation_id) DESC, m.name;

CREATE TEMP TABLE kritique_repository_map ON COMMIT DROP AS
SELECT r.id AS old_id, m.account_id,
       pg_temp.kritique_id('repository:' || m.account_id || ':' || lower(r.name)) AS new_id
FROM repositories r JOIN kritique_installation_map m ON m.installation_id = r.installation_id;

DO $$
DECLARE
    dup text;
BEGIN
    SELECT string_agg(DISTINCT r.name, ', ') INTO dup
    FROM kritique_repository_map m JOIN repositories r ON r.id = m.old_id
    WHERE m.new_id IN (SELECT new_id FROM kritique_repository_map GROUP BY new_id HAVING count(*) > 1);
    IF dup IS NOT NULL THEN
        RAISE EXCEPTION 'kritique: repositories % are each held by more than one installation of the same forge account; disable and delete all but one row of each before upgrading', dup;
    END IF;
END;
$$;

-- An account is a forge account, <forge>/<name>. Its id derives from the
-- forge and the lowercased name, so every role can address it without a
-- lookup; it exists while a connection serves it.
CREATE TABLE accounts (
    id          uuid        PRIMARY KEY,
    forge       text        NOT NULL CHECK (forge IN ('github', 'gitlab', 'forgejo', 'gitea')),
    name        text        NOT NULL,
    enabled     boolean     NOT NULL DEFAULT true,
    disabled_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
INSERT INTO accounts (id, forge, name, enabled, disabled_at, created_at, updated_at)
SELECT m.account_id, min(m.forge), min(m.account),
       bool_or(m.enabled AND t.enabled),
       CASE WHEN NOT bool_or(m.enabled AND t.enabled) THEN max(coalesce(t.disabled_at, m.disabled_at)) END,
       min(least(m.created_at, t.created_at)), max(greatest(m.updated_at, t.updated_at))
FROM kritique_installation_map m JOIN tenants t ON t.id = m.tenant_id
GROUP BY m.account_id;

-- A connection is one forge credential serving the accounts it lists. It
-- belongs to no account, so it carries no row-level security.
CREATE TABLE connections (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text        NOT NULL UNIQUE,
    forge           text        NOT NULL CHECK (forge IN ('github', 'gitlab', 'forgejo', 'gitea')),
    -- The accounts the connection serves, as a public GitHub App
    -- installed on several organizations does.
    accounts        text[]      NOT NULL DEFAULT '{}',
    enabled         boolean     NOT NULL DEFAULT true,
    disabled_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- When the connection's webhook last delivered a request kritique
    -- verified, so the dashboard can tell a connection whose forge sends
    -- webhooks from one kritique only polls, and when one last arrived with
    -- no signature at all, which a GitHub App with no webhook secret sends.
    -- The listener writes each at most once a minute.
    last_webhook_at          timestamptz,
    last_unsigned_webhook_at timestamptz
);
INSERT INTO connections (id, name, forge, accounts, enabled, disabled_at, created_at, updated_at)
SELECT connection_id, name, forge, ARRAY[account], enabled, disabled_at, created_at, updated_at
FROM kritique_installation_map;

-- Every foreign key into the re-keyed tables goes while their ids change;
-- those into repositories come back as they were, and every account_id
-- gets one into accounts below.
CREATE TEMP TABLE kritique_repository_fkeys ON COMMIT DROP AS
SELECT conrelid::regclass AS tbl, conname, pg_get_constraintdef(oid) AS def,
       (SELECT attname FROM pg_attribute WHERE attrelid = conrelid AND attnum = conkey[1]) AS col
FROM pg_constraint
WHERE contype = 'f' AND confrelid = 'repositories'::regclass;

DO $$
DECLARE
    fk record;
BEGIN
    FOR fk IN
        SELECT conrelid::regclass AS tbl, conname FROM pg_constraint
        WHERE contype = 'f' AND confrelid IN ('tenants'::regclass, 'installations'::regclass, 'repositories'::regclass)
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', fk.tbl, fk.conname);
    END LOOP;
END;
$$;

-- Leases are held for seconds by running jobs; the slots refill.
DELETE FROM model_leases;

-- Every tenant_id becomes an account_id, first the tenant's account, then,
-- below, the account of the repository the row belongs to.
DO $$
DECLARE
    t record;
BEGIN
    FOR t IN
        SELECT c.oid::regclass AS tbl FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = current_schema() AND c.relkind = 'r' AND a.attname = 'tenant_id' AND NOT a.attisdropped
          AND c.relname NOT IN ('installations', 'poll_state')
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME COLUMN tenant_id TO account_id', t.tbl);
        EXECUTE format('UPDATE %s x SET account_id = m.account_id FROM kritique_tenant_map m WHERE x.account_id = m.tenant_id', t.tbl);
    END LOOP;
END;
$$;

UPDATE repositories r SET id = m.new_id, account_id = m.account_id
FROM kritique_repository_map m WHERE r.id = m.old_id;
ALTER TABLE repositories DROP COLUMN installation_id;

DO $$
DECLARE
    fk record;
BEGIN
    FOR fk IN SELECT * FROM kritique_repository_fkeys LOOP
        EXECUTE format('UPDATE %s x SET %I = m.new_id FROM kritique_repository_map m WHERE x.%I = m.old_id',
            fk.tbl, fk.col, fk.col);
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s', fk.tbl, fk.conname, fk.def);
    END LOOP;
END;
$$;

-- A row's account is its repository's, which differs from its tenant's
-- only where a tenant held several forge accounts.
UPDATE pull_requests x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE index_runs x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE usage x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE gateway_tokens x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE task_events x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE task_runs x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
UPDATE reviews x SET account_id = p.account_id FROM pull_requests p WHERE p.id = x.pull_request_id AND x.account_id <> p.account_id;
UPDATE followups x SET account_id = p.account_id FROM pull_requests p WHERE p.id = x.pull_request_id AND x.account_id <> p.account_id;
UPDATE sticky_comments x SET account_id = p.account_id FROM pull_requests p WHERE p.id = x.pull_request_id AND x.account_id <> p.account_id;
UPDATE findings x SET account_id = p.account_id FROM reviews p WHERE p.id = x.review_id AND x.account_id <> p.account_id;
UPDATE usage x SET account_id = p.account_id FROM reviews p WHERE x.repository_id IS NULL AND p.id = x.review_id AND x.account_id <> p.account_id;
UPDATE runner_runs x SET account_id = p.account_id FROM reviews p WHERE p.id = x.review_id AND x.account_id <> p.account_id;
UPDATE runner_runs x SET account_id = p.account_id FROM index_runs p WHERE p.id = x.index_run_id AND x.account_id <> p.account_id;
UPDATE runner_runs x SET account_id = p.account_id FROM task_runs p WHERE p.runner_run_id = x.id AND x.account_id <> p.account_id;
UPDATE context_packs x SET account_id = p.account_id FROM runner_runs p WHERE p.id = x.runner_run_id AND x.account_id <> p.account_id;
UPDATE index_packs x SET account_id = p.account_id FROM runner_runs p WHERE p.id = x.runner_run_id AND x.account_id <> p.account_id;
UPDATE index_staging x SET account_id = p.account_id FROM runner_runs p WHERE p.id = x.runner_run_id AND x.account_id <> p.account_id;
UPDATE agent_runs x SET account_id = p.account_id FROM runner_runs p WHERE p.id = x.runner_run_id AND x.account_id <> p.account_id;
UPDATE model_calls x SET account_id = p.account_id FROM reviews p WHERE p.id = x.review_id AND x.account_id <> p.account_id;
UPDATE model_calls x SET account_id = p.account_id FROM runner_runs p WHERE x.review_id IS NULL AND p.id = x.runner_run_id AND x.account_id <> p.account_id;
UPDATE model_calls x SET account_id = p.account_id FROM task_runs p WHERE x.review_id IS NULL AND x.runner_run_id IS NULL AND p.id = x.task_run_id AND x.account_id <> p.account_id;
DO $$
BEGIN
    IF to_regclass('index_chunks') IS NOT NULL THEN
        UPDATE index_chunks x SET account_id = p.account_id FROM repositories p WHERE p.id = x.repository_id AND x.account_id <> p.account_id;
    END IF;
END;
$$;

-- A task event names the connection its delivery came through.
ALTER TABLE task_events RENAME COLUMN installation_id TO connection_id;
UPDATE task_events x SET connection_id = m.connection_id
FROM kritique_installation_map m WHERE x.connection_id = m.installation_id;
ALTER TABLE task_events ADD CONSTRAINT task_events_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES connections (id);

-- One row per account: when the leader last listed its open pull
-- requests, so a restarted leader resumes where the previous one stopped.
CREATE TEMP TABLE kritique_poll_state ON COMMIT DROP AS
SELECT m.account_id, max(p.last_polled_at) AS last_polled_at, max(p.updated_at) AS updated_at
FROM poll_state p JOIN kritique_installation_map m ON m.installation_id = p.installation_id
GROUP BY m.account_id;
DROP TABLE poll_state;
CREATE TABLE poll_state (
    account_id     uuid        PRIMARY KEY REFERENCES accounts (id),
    last_polled_at timestamptz NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now()
);
INSERT INTO poll_state SELECT account_id, last_polled_at, updated_at FROM kritique_poll_state;
ALTER TABLE poll_state ENABLE ROW LEVEL SECURITY;

-- An audit event of a tenant that served no forge account keeps no account.
UPDATE audit_events SET account_id = NULL WHERE account_id NOT IN (SELECT id FROM accounts);

DROP TABLE installations;
DROP TABLE tenants;

ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
CREATE POLICY account_isolation ON accounts
    USING      (id = NULLIF(current_setting('app.account_id', true), '')::uuid)
    WITH CHECK (id = NULLIF(current_setting('app.account_id', true), '')::uuid);

-- Every account_id references its account, indexes are named for it, and
-- row-level security keys on app.account_id.
DO $$
DECLARE
    t record;
BEGIN
    FOR t IN
        SELECT c.oid::regclass AS tbl, c.relname FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = current_schema() AND c.relkind = 'r' AND a.attname = 'account_id' AND NOT a.attisdropped
          AND c.relname <> 'poll_state'
    LOOP
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I FOREIGN KEY (account_id) REFERENCES accounts (id)',
            t.tbl, t.relname || '_account_id_fkey');
    END LOOP;
    FOR t IN
        SELECT c.relname AS idx FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = current_schema() AND c.relkind = 'i' AND c.relname LIKE '%tenant%'
    LOOP
        EXECUTE format('ALTER INDEX %I RENAME TO %I', t.idx, replace(t.idx, 'tenant', 'account'));
    END LOOP;
    FOR t IN SELECT tablename FROM pg_policies WHERE schemaname = current_schema() AND policyname = 'tenant_isolation' LOOP
        EXECUTE format('DROP POLICY tenant_isolation ON %I', t.tablename);
    END LOOP;
    FOR t IN
        SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relrowsecurity AND c.relname <> 'accounts'
    LOOP
        EXECUTE format($p$CREATE POLICY account_isolation ON %I
            USING      (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid)
            WITH CHECK (account_id = NULLIF(current_setting('app.account_id', true), '')::uuid)$p$, t.relname);
    END LOOP;
END;
$$;

-- What a runner writes must carry its run's own account: the subquery
-- resolves through the runner's own-row policy on runner_runs, so a row
-- under any other account is refused rather than left where no account's
-- reads or sweeps would ever find it.
DROP POLICY runner_job ON context_packs;
DROP POLICY runner_job ON index_packs;
DROP POLICY runner_job ON index_staging;
DROP POLICY runner_job ON agent_runs;
CREATE POLICY runner_job ON context_packs
    USING      (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid)
    WITH CHECK (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid
                AND account_id = (SELECT r.account_id FROM runner_runs r WHERE r.id = runner_run_id));
CREATE POLICY runner_job ON index_packs
    USING      (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid)
    WITH CHECK (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid
                AND account_id = (SELECT r.account_id FROM runner_runs r WHERE r.id = runner_run_id));
CREATE POLICY runner_job ON index_staging
    USING      (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid)
    WITH CHECK (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid
                AND account_id = (SELECT r.account_id FROM runner_runs r WHERE r.id = runner_run_id));
CREATE POLICY runner_job ON agent_runs
    USING      (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid)
    WITH CHECK (runner_run_id = NULLIF(current_setting('app.runner_job_id', true), '')::uuid
                AND account_id = (SELECT r.account_id FROM runner_runs r WHERE r.id = runner_run_id));

-- The dashboard's live events name the account.
CREATE OR REPLACE FUNCTION kritique_notify_event() RETURNS trigger AS $$
DECLARE
    row_json jsonb := to_jsonb(NEW);
BEGIN
    PERFORM pg_notify('kritique_events', jsonb_build_object(
        'account_id', row_json ->> 'account_id',
        'kind', TG_ARGV[0],
        'id', row_json ->> 'id',
        'review_id', row_json ->> 'review_id'
    )::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- A renamed column keeps the name of its NOT NULL constraint (Postgres 18
-- names them), so each takes the name a fresh table would give it: in two
-- passes, since users still held the accounts_* names a new accounts table
-- needs.
DO $$
DECLARE
    c record;
BEGIN
    FOR c IN
        SELECT k.conrelid::regclass AS tbl, k.conname, rel.relname || '_' || a.attname || '_not_null' AS want
        FROM pg_constraint k JOIN pg_class rel ON rel.oid = k.conrelid JOIN pg_namespace n ON n.oid = rel.relnamespace
        JOIN pg_attribute a ON a.attrelid = k.conrelid AND a.attnum = k.conkey[1]
        WHERE n.nspname = current_schema() AND k.contype = 'n' AND rel.relname NOT LIKE 'river\_%'
          AND k.conname <> rel.relname || '_' || a.attname || '_not_null'
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME CONSTRAINT %I TO %I', c.tbl, c.conname, 'kritique_tmp_' || md5(c.want));
    END LOOP;
    FOR c IN
        SELECT k.conrelid::regclass AS tbl, k.conname, rel.relname || '_' || a.attname || '_not_null' AS want
        FROM pg_constraint k JOIN pg_class rel ON rel.oid = k.conrelid JOIN pg_namespace n ON n.oid = rel.relnamespace
        JOIN pg_attribute a ON a.attrelid = k.conrelid AND a.attnum = k.conkey[1]
        WHERE n.nspname = current_schema() AND k.contype = 'n' AND k.conname LIKE 'kritique\_tmp\_%'
    LOOP
        EXECUTE format('ALTER TABLE %s RENAME CONSTRAINT %I TO %I', c.tbl, c.conname, c.want);
    END LOOP;
END;
$$;

DROP FUNCTION pg_temp.kritique_id(text);
DROP FUNCTION pg_temp.kritique_sha1(bytea);
