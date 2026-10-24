-- ============================================================
-- Migration 050: clients.client_secret defaults to empty
-- ============================================================
-- The column held a client's secret itself until secrets were hashed
-- (client_secret_hash). Every row's secret has been hashed and the column
-- emptied; the application no longer reads or writes it, nor
-- allow_email_login. With a default, rows the application inserts without
-- naming the column still satisfy NOT NULL, until a later migration drops
-- both columns (docs/DATABASE.md, «Changes that cannot be undone»).
--
-- To undo: ALTER TABLE clients ALTER COLUMN client_secret DROP DEFAULT;
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE clients ALTER COLUMN client_secret SET DEFAULT '';
