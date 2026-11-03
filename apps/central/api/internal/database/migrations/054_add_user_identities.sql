-- ============================================================
-- Migration 054: Sign-in identities — one row per provider account an
-- account signs in with
-- ============================================================
-- An account's links to sign-in providers were four columns on users
-- (google_id, github_id, microsoft_id, apple_id): one provider account per
-- provider, and no room for a second Google account or a kind of sign-in that
-- is not a provider at all. A row here is one provider account linked to an
-- account; an account may have any number of them.
--
-- Within a tenant a provider account identifies exactly one account
-- (tenant_id, provider, subject) — the rule the four partial unique indexes on
-- users keep for their columns. tenant_id repeats the account's tenant so the
-- database can hold that rule.
--
-- Additive: the columns stay and are still what is read; the application writes
-- both, and the rows below are copied from them. An API that does not know the
-- table keeps working, though what it links or unlinks leaves the rows behind
-- the columns until a later release reconciles them — that release switches
-- reading to the rows, and a still later one drops the columns.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

CREATE TABLE IF NOT EXISTS user_identities (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      VARCHAR(32) NOT NULL,
    subject       VARCHAR(255) NOT NULL,
    -- The address the provider reported when the account was linked; the
    -- account's own address does not change with it.
    email_at_link VARCHAR(255) NOT NULL DEFAULT '',
    linked_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_identities_provider CHECK (provider IN ('google', 'github', 'microsoft', 'apple'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_user_identities_tenant_provider_subject
    ON user_identities (tenant_id, provider, subject);
CREATE INDEX IF NOT EXISTS idx_user_identities_user ON user_identities (user_id);

INSERT INTO user_identities (id, tenant_id, user_id, provider, subject, email_at_link, linked_at)
SELECT gen_random_uuid(), u.tenant_id, u.id, l.provider, l.subject, u.email, u.updated_at
FROM users u
CROSS JOIN LATERAL (VALUES
    ('google', u.google_id),
    ('github', u.github_id),
    ('microsoft', u.microsoft_id),
    ('apple', u.apple_id)
) AS l(provider, subject)
WHERE l.subject IS NOT NULL AND l.subject <> ''
ON CONFLICT DO NOTHING;

COMMENT ON TABLE user_identities IS 'A sign-in provider account linked to an account; an account may have several.';
