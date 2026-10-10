-- ============================================================
-- Migration 026: Drop impersonation_sessions
-- ============================================================
-- Admin impersonation was removed: it issued tokens that nothing accepted, so
-- the feature recorded sessions and did nothing else. No code maps this table
-- any more. It held no rows on every deployment checked before this migration
-- was applied; check yours first if it may differ.
--
-- Destructive, so it ships in a deployment of its own, after the application
-- that stopped using the table. Dropping the table drops its indexes with it.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

DROP TABLE IF EXISTS impersonation_sessions;
