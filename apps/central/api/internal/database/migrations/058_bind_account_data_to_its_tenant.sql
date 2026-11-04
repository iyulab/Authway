-- ============================================================
-- Migration 058: What belongs to an account belongs to an account of its own
-- tenant, and the database holds that
-- ============================================================
-- user_identities (054), user_passkeys (056), user_claims (001),
-- delegation_grants (044) and approval_requests (046) each repeat the tenant of
-- the account they belong to, because the rules that make them safe are per
-- tenant: a provider account identifies one account within a tenant, a
-- credential id is unique within a tenant, a claim or a delegation is read for
-- the tenant of a sign-in. The repeat was only a column, though. Nothing stopped
-- a row from naming one tenant and an account of another, so tenant isolation of
-- these tables rested on every writer passing the account's own tenant.
--
-- The key below is the account's (tenant_id, id), and each of the five tables
-- now points at it with its own (tenant_id, user_id). The single-column foreign
-- keys to users(id) are replaced by it: a row still goes with its account when
-- the account is deleted, and can no longer name an account of another tenant.
-- (The console tables, whose accounts live in the operator's tenant while the
-- row names the tenant they work in, are not like this and are left as they are.)
--
-- What to check first: a row that already names the wrong tenant would make a
-- foreign key fail, and the whole run with it. Every one of these is written
-- from the account's own tenant, so there should be none; check the target:
--   SELECT 'user_identities', count(*) FROM user_identities x JOIN users u ON u.id = x.user_id WHERE x.tenant_id <> u.tenant_id
--   UNION ALL SELECT 'user_passkeys', count(*) FROM user_passkeys x JOIN users u ON u.id = x.user_id WHERE x.tenant_id <> u.tenant_id
--   UNION ALL SELECT 'user_claims', count(*) FROM user_claims x JOIN users u ON u.id = x.user_id WHERE x.tenant_id <> u.tenant_id
--   UNION ALL SELECT 'delegation_grants', count(*) FROM delegation_grants x JOIN users u ON u.id = x.user_id WHERE x.tenant_id <> u.tenant_id
--   UNION ALL SELECT 'approval_requests', count(*) FROM approval_requests x JOIN users u ON u.id = x.user_id WHERE x.tenant_id <> u.tenant_id;
-- All must be 0. A non-zero count is a finding of its own — resolve those rows
-- before applying this, not by loosening it.
--
-- Additive in effect: no data changes, and a row the application writes today
-- already satisfies it. Adding the foreign keys takes a lock on the tables while
-- they are checked. Rolling the application back to the image before this one is
-- safe — it writes the same rows.
--
-- There are no down migrations. To go back to the single-column keys, per table
-- (the name of the old key is given):
--   ALTER TABLE <table> DROP CONSTRAINT fk_<table>_account,
--       ADD CONSTRAINT <old name> FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
-- with <old name> = <table>_user_id_fkey, except user_claims: fk_user_claims_user.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

-- id alone already identifies an account; the pair exists so a table that
-- repeats the tenant can reference both.
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_tenant_id_id ON users (tenant_id, id);

ALTER TABLE user_identities DROP CONSTRAINT IF EXISTS user_identities_user_id_fkey;
ALTER TABLE user_identities DROP CONSTRAINT IF EXISTS fk_user_identities_account;
ALTER TABLE user_identities
    ADD CONSTRAINT fk_user_identities_account
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE user_passkeys DROP CONSTRAINT IF EXISTS user_passkeys_user_id_fkey;
ALTER TABLE user_passkeys DROP CONSTRAINT IF EXISTS fk_user_passkeys_account;
ALTER TABLE user_passkeys
    ADD CONSTRAINT fk_user_passkeys_account
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE user_claims DROP CONSTRAINT IF EXISTS fk_user_claims_user;
ALTER TABLE user_claims DROP CONSTRAINT IF EXISTS user_claims_user_id_fkey;
ALTER TABLE user_claims DROP CONSTRAINT IF EXISTS fk_user_claims_account;
ALTER TABLE user_claims
    ADD CONSTRAINT fk_user_claims_account
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE delegation_grants DROP CONSTRAINT IF EXISTS delegation_grants_user_id_fkey;
ALTER TABLE delegation_grants DROP CONSTRAINT IF EXISTS fk_delegation_grants_account;
ALTER TABLE delegation_grants
    ADD CONSTRAINT fk_delegation_grants_account
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;

ALTER TABLE approval_requests DROP CONSTRAINT IF EXISTS approval_requests_user_id_fkey;
ALTER TABLE approval_requests DROP CONSTRAINT IF EXISTS fk_approval_requests_account;
ALTER TABLE approval_requests
    ADD CONSTRAINT fk_approval_requests_account
    FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE;
