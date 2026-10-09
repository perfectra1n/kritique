-- kritique joins kritika's schema (0015-0017): the migrations before this
-- one are kritique's own history and stay as they were applied; these three
-- carry a database at 0014 to kritika's schema, keeping kritique's tasks
-- and forges. This one is the dashboard's sign-in model.
--
-- A person who signs in to the dashboard is a user, and "account" now names
-- a forge account (0016). Roles come from sign-in rather than from stored
-- memberships and invites, and accounts come from the configuration file
-- rather than from dashboard-authored specs, so those tables go.
ALTER TABLE accounts RENAME TO users;
ALTER TABLE users RENAME CONSTRAINT accounts_pkey TO users_pkey;
ALTER TABLE users DROP COLUMN last_seen_at;
-- What a user chose for their own dashboard: {"timeZone", "clock",
-- "theme"}, each absent or "" while the browser decides. One document, not
-- a column each, so a new choice is no new migration.
ALTER TABLE users ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE identities RENAME COLUMN account_id TO user_id;
ALTER TABLE identities RENAME CONSTRAINT identities_account_id_fkey TO identities_user_id_fkey;
ALTER INDEX identities_account_id_idx RENAME TO identities_user_id_idx;
ALTER TABLE identities DROP COLUMN email;

-- A session now carries the grant its sign-in decided, which no stored
-- session has: everyone signs in again. It is looked up by the SHA-256 of
-- its cookie value; the value itself is never stored. It records the origin
-- it signed in through, and a session whose sign-in has since moved to
-- another origin is no longer honoured. role is the instance role, and a
-- member reads every account when all_accounts is set and otherwise the
-- forge accounts in accounts, lowercased as "<forge>/<name>". grant_key
-- fingerprints the sign-in configuration that decided the grant, so a
-- session outlives no change to it.
DROP TABLE sessions;
CREATE TABLE sessions (
    token_hash      bytea       PRIMARY KEY,
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider        text        NOT NULL,
    provider_origin text        NOT NULL DEFAULT '',
    role            text        NOT NULL CHECK (role IN ('admin', 'member')),
    all_accounts    boolean     NOT NULL,
    accounts        text[]      NOT NULL,
    grant_key       text        NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

ALTER TABLE audit_events RENAME COLUMN account_id TO user_id;
ALTER TABLE audit_events RENAME CONSTRAINT audit_events_account_id_fkey TO audit_events_user_id_fkey;
CREATE INDEX audit_events_user_id_idx ON audit_events (user_id) WHERE user_id IS NOT NULL;

-- A dashboard-authored tenant spec is the only copy of that configuration,
-- so it is kept in the audit log, where an admin can read it back into the
-- configuration file, before its table goes.
INSERT INTO audit_events (at, user_id, action, target, detail)
SELECT now(), coalesce(updated_by, created_by), 'migration.archive_dashboard_tenant', slug,
       jsonb_build_object('spec', spec, 'revision', revision, 'created_at', created_at, 'updated_at', updated_at)
FROM dashboard_tenants;

DROP TRIGGER kritique_notify_dashboard_tenant ON dashboard_tenants;
DROP FUNCTION kritique_notify_config();
DROP TABLE dashboard_tenants;
DROP TABLE invites;
DROP TABLE memberships;
