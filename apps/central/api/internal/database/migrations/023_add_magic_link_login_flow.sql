-- ============================================================
-- Migration 023: Bind magic links to the login flow they sign in to
-- ============================================================
-- A magic link is a step of a login flow: redeeming it completes that flow,
-- so the application receives ordinary OAuth tokens. The flow id is a Hydra
-- login challenge (1-2 KB), kept here rather than in the emailed URL.
--
-- Additive and nullable: code that does not know the column ignores it.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE magic_link_tokens ADD COLUMN IF NOT EXISTS login_flow TEXT;
