-- ============================================================
-- Migration 035: Documents a tenant's users accept at sign-in
-- ============================================================
-- A tenant names the documents its users accept — terms of service, a
-- privacy policy — by kind, one current version per kind. A sign-in owes the
-- current version of every kind the user has not accepted yet; what each user
-- accepted, and when, is kept.
--
-- tenant_terms keeps every version ever current: an acceptance points at the
-- version it was for. retired_at is set when a version stops being the
-- current one (a newer version, or the kind is no longer asked for).
--
-- Additive: an API that does not know the tables never asks for terms.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

CREATE TABLE IF NOT EXISTS tenant_terms (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind         VARCHAR(40) NOT NULL,
    version      VARCHAR(40) NOT NULL,
    url          TEXT NOT NULL,
    title        VARCHAR(200),
    published_at TIMESTAMPTZ NOT NULL,
    retired_at   TIMESTAMPTZ,
    CONSTRAINT tenant_terms_version_unique UNIQUE (tenant_id, kind, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS tenant_terms_current
    ON tenant_terms (tenant_id, kind) WHERE retired_at IS NULL;

COMMENT ON TABLE tenant_terms IS 'Versions of the documents a tenant''s users accept at sign-in. One current (retired_at NULL) per tenant and kind.';

CREATE TABLE IF NOT EXISTS user_terms_acceptances (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    terms_id    UUID NOT NULL REFERENCES tenant_terms(id) ON DELETE CASCADE,
    client_id   VARCHAR(255),
    accepted_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT user_terms_acceptances_unique UNIQUE (user_id, terms_id)
);

CREATE INDEX IF NOT EXISTS idx_user_terms_acceptances_user ON user_terms_acceptances (user_id, accepted_at DESC);

COMMENT ON TABLE user_terms_acceptances IS 'Which version of a tenant document a user accepted, when, and signing in to which client.';
