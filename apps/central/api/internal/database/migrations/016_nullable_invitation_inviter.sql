-- ============================================================
-- Migration 016: Nullable invitation inviter (system-actor invitations)
-- ============================================================
-- Onboarding is invitation-only, and the only admin-side surface for creating
-- a user is POST /api/v1/invitations. That endpoint, when called with the admin
-- API key, has no signed-in user to attribute the invitation to — it used a
-- hard-coded UUID (00000000-0000-0000-0000-000000000001) that no users row ever
-- had. `inviter_id NOT NULL REFERENCES users(id)` therefore made every admin-key
-- invitation fail, so a fresh instance/tenant could never get its first user:
-- creating a user required an invitation, and an invitation required a user.
--
-- The fix is to let the schema express what was already true — the inviter may
-- be the system, not a user. NULL inviter_id = created by the system actor.
--
-- ON DELETE is also changed CASCADE -> SET NULL. CASCADE meant deleting a user
-- silently destroyed every invitation they had ever sent, including accepted
-- ones that are audit-relevant history. SET NULL keeps the record and drops
-- only the attribution, which is the same shape as accepted_user_id already had.
--
-- Constraint name verified against pg_constraint on a live database
-- (invitations_inviter_id_fkey, confdeltype='c'), not assumed.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and defeat
-- its all-or-nothing guarantee. Migration files must contain no
-- transaction-control statements.
-- ============================================================

ALTER TABLE invitations ALTER COLUMN inviter_id DROP NOT NULL;

ALTER TABLE invitations DROP CONSTRAINT IF EXISTS invitations_inviter_id_fkey;
ALTER TABLE invitations ADD CONSTRAINT invitations_inviter_id_fkey
    FOREIGN KEY (inviter_id) REFERENCES users(id) ON DELETE SET NULL;

-- Align the column default with the application's own default (invitation
-- service falls back to 'member'). 'user' was a value no code path ever wrote.
ALTER TABLE invitations ALTER COLUMN role SET DEFAULT 'member';

COMMENT ON COLUMN invitations.inviter_id IS 'User who sent the invitation. NULL = created by the system actor (admin API key), which has no user row.';

-- ============================================================
-- Migration Complete: invitations.inviter_id is nullable
-- ============================================================
--
-- Verification queries:
-- SELECT is_nullable, column_default FROM information_schema.columns
--   WHERE table_name='invitations' AND column_name IN ('inviter_id','role');
--   -- expect inviter_id YES, role default 'member'
--
-- SELECT conname, confdeltype FROM pg_constraint
--   WHERE conrelid='invitations'::regclass AND contype='f';
--   -- expect invitations_inviter_id_fkey with confdeltype='n' (SET NULL)
