-- ============================================================
-- Authway dev clean slate — DESTRUCTIVE, run by hand only
-- ============================================================
-- Drops every Authway-owned table so the next application start rebuilds the
-- schema from migrations/000..NNN. There is no undo. Development only.
--
--     psql "$DSN" -f scripts/bootstrap/dev-clean-slate.sql
--
-- This file only DROPS. It deliberately does not recreate anything: the
-- migrations are the single source of truth for schema, and a second copy of
-- the DDL here would silently drift from them. Dropping schema_migrations is
-- part of the reset — without it the migrator would consider every migration
-- applied and leave you with an empty database it refuses to populate.
--
-- Tables are listed explicitly rather than swept from information_schema
-- because Hydra shares this database and owns its own tables.
--
-- History: this used to live at migrations/000_v0_clean_slate.sql, where the
-- migrator was supposed to apply it automatically — a script whose first act is
-- DROP TABLE CASCADE, on every boot. It never ran, because its version collided
-- with a bookkeeping sentinel row and it was always treated as already applied.
-- That accident is the only reason no deployment was ever wiped. Provisioning a
-- blank database is now migrations/000_initial_schema.sql, which is idempotent
-- and creates without dropping. This script must never return to migrations/.
-- ============================================================

BEGIN;

DROP TABLE IF EXISTS webhook_deliveries CASCADE;
DROP TABLE IF EXISTS webhooks CASCADE;
DROP TABLE IF EXISTS system_config CASCADE;
DROP TABLE IF EXISTS invitations CASCADE;
DROP TABLE IF EXISTS magic_link_tokens CASCADE;
DROP TABLE IF EXISTS impersonation_sessions CASCADE;
DROP TABLE IF EXISTS audit_logs CASCADE;
DROP TABLE IF EXISTS user_claims CASCADE;
DROP TABLE IF EXISTS admin_sessions CASCADE;
DROP TABLE IF EXISTS password_resets CASCADE;
DROP TABLE IF EXISTS email_verifications CASCADE;
DROP TABLE IF EXISTS consent_grants CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
DROP TABLE IF EXISTS clients CASCADE;
DROP TABLE IF EXISTS oauth_clients CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;

DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;

-- Reset migration bookkeeping last, so a failure above leaves the recorded
-- state matching the schema that is actually still there.
DROP TABLE IF EXISTS schema_migrations CASCADE;

COMMIT;

-- Next application start applies migrations 000..NNN against the empty database.
