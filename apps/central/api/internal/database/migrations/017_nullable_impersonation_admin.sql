-- ============================================================
-- Migration 017: Nullable impersonation admin (system-actor sessions)
-- ============================================================
-- Same defect class as migration 016, in a second place. The impersonation
-- handler attributes an admin-API-key request to the hard-coded UUID
-- 00000000-0000-0000-0000-000000000001, which no users row has, while
-- admin_id was NOT NULL REFERENCES users(id) — so starting an impersonation
-- session with the admin API key could never succeed.
--
-- That UUID is also tenant.DefaultTenantID and was the invitation inviter: one
-- constant carrying three unrelated meanings. 016 removed the invitation use;
-- this removes the impersonation one. NULL admin_id = the system actor.
--
-- ON DELETE CASCADE -> SET NULL for the same reason as 016: impersonation
-- sessions are audit history, and deleting an admin must not erase the record
-- that they impersonated someone.
--
-- admin_email is left NOT NULL and carries 'system' for these sessions, so the
-- audit trail still names an actor even when no user row backs it.
--
-- Constraint name verified against pg_constraint on a live database
-- (impersonation_sessions_admin_id_fkey, confdeltype='c').
--
-- NOTE: No BEGIN/COMMIT here — RunMigrations wraps the whole run in one
-- transaction and an inner COMMIT would defeat its all-or-nothing guarantee.
-- ============================================================

ALTER TABLE impersonation_sessions ALTER COLUMN admin_id DROP NOT NULL;

ALTER TABLE impersonation_sessions DROP CONSTRAINT IF EXISTS impersonation_sessions_admin_id_fkey;
ALTER TABLE impersonation_sessions ADD CONSTRAINT impersonation_sessions_admin_id_fkey
    FOREIGN KEY (admin_id) REFERENCES users(id) ON DELETE SET NULL;

COMMENT ON COLUMN impersonation_sessions.admin_id IS 'Admin user who started the session. NULL = the system actor (admin API key), which has no user row; admin_email then reads ''system''.';

-- ============================================================
-- Migration Complete: impersonation_sessions.admin_id is nullable
-- ============================================================
--
-- Verification:
-- SELECT is_nullable FROM information_schema.columns
--   WHERE table_name='impersonation_sessions' AND column_name='admin_id';  -- YES
-- SELECT conname, confdeltype FROM pg_constraint
--   WHERE conrelid='impersonation_sessions'::regclass AND contype='f';
--   -- impersonation_sessions_admin_id_fkey must read 'n' (SET NULL)
