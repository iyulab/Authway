-- ============================================================
-- Migration 024: Drop columns and tables no code reads or writes
-- ============================================================
-- Each of these outlived the code that used it. Nothing maps them and nothing
-- queries them. All but one were empty (system_config: only the rows 007
-- seeded) on every deployment checked before this migration was applied.
--
-- clients: the per-client logout policy (004). The authorization server checks
-- post_logout_redirect_uri against the client's registered
-- post_logout_redirect_uris and decides where a logout lands; Authway no
-- longer stores or applies a policy of its own, and has not since 0.5.0. These
-- columns still held the values clients were configured with before then;
-- export them first if a deployment needs a record of that configuration.
--
-- magic_link_tokens: the OAuth parameters a link used to carry (011), the Hydra
-- login challenge and the boolean used flag (006). A link now binds to its login
-- flow (023, login_flow) and records redemption in used_at (011).
--
-- audit_logs.description / metadata (000): superseded by details (009).
-- webhooks.last_* / webhook_deliveries.error, duration_ms (006): superseded by
-- the delivery rows' success / error_message (021); the summary columns were
-- never written.
--
-- sessions (000): end-user sessions are the authorization server's; no code
-- ever wrote this table. system_config (007): seeded provider settings that
-- nothing reads — providers are configured through the environment.
--
-- Destructive, so it ships in a deployment of its own, after the application
-- that stopped using these. IF EXISTS keeps it safe on databases where an
-- earlier tooling generation never created a column or the constraint.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE clients DROP CONSTRAINT IF EXISTS check_logout_redirect_policy;
DROP INDEX IF EXISTS idx_clients_logout_policy;

ALTER TABLE clients
    DROP COLUMN IF EXISTS logout_redirect_policy,
    DROP COLUMN IF EXISTS default_logout_uri,
    DROP COLUMN IF EXISTS allow_wildcard_logout;

ALTER TABLE magic_link_tokens
    DROP COLUMN IF EXISTS client_id,
    DROP COLUMN IF EXISTS redirect_uri,
    DROP COLUMN IF EXISTS state,
    DROP COLUMN IF EXISTS login_challenge,
    DROP COLUMN IF EXISTS used;

ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS metadata;

ALTER TABLE webhooks
    DROP COLUMN IF EXISTS last_triggered_at,
    DROP COLUMN IF EXISTS last_status_code,
    DROP COLUMN IF EXISTS last_error;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS error,
    DROP COLUMN IF EXISTS duration_ms;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS system_config;
