-- ============================================================
-- Migration 057: Drop the retired provider-link, picture, logo and color columns
-- ============================================================
-- users.google_id, users.github_id, users.microsoft_id, users.apple_id
--   An account's links to sign-in providers are rows of user_identities
--   (migration 054, read since 055). The previous deployment stopped writing
--   these four and kept them mapped, read-only, until this migration. The
--   partial unique indexes uq_users_tenant_{google,github,microsoft,apple}_id
--   go with their columns.
-- users.picture
--   From the first schema; never read, and every insert wrote NULL into it. An
--   account's picture is users.avatar_url.
-- tenants.logo, tenants.primary_color
--   Kept and returned as given, shown by no screen; gone from the tenant in the
--   API. A client's own logo (clients.logo, shown on the sign-in and consent
--   screens) is not this column. primary_color has a default, so every tenant
--   holds a value there.
--
-- What it deletes: the provider ids that migration 055 had made equal to the
-- user_identities rows (and anything an older API wrote after that), and the
-- tenant logo and color values nobody could read. Check the target first, as
-- migration 043 asked of its column:
--   SELECT count(*) FILTER (WHERE google_id IS NOT NULL) AS google,
--          count(*) FILTER (WHERE github_id IS NOT NULL) AS github,
--          count(*) FILTER (WHERE microsoft_id IS NOT NULL) AS microsoft,
--          count(*) FILTER (WHERE apple_id IS NOT NULL) AS apple,
--          count(*) FILTER (WHERE picture IS NOT NULL) AS picture
--   FROM users;
--   SELECT count(*) FILTER (WHERE logo IS NOT NULL AND logo <> '') AS logo,
--          count(*) FILTER (WHERE primary_color IS NOT NULL AND primary_color <> '') AS color
--   FROM tenants;
--
-- Destructive, so it ships in a deployment of its own, after the application
-- that stopped writing the columns. Once it is applied, the application that
-- ran before it still reads and writes some of these columns: roll forward, do
-- not roll back to that image.
--
-- There are no down migrations. To have the columns back as they were defined
-- (the values are gone; the links are still the user_identities rows):
--   ALTER TABLE users ADD COLUMN google_id VARCHAR(255), ADD COLUMN github_id VARCHAR(255),
--       ADD COLUMN microsoft_id VARCHAR(255), ADD COLUMN apple_id VARCHAR(255),
--       ADD COLUMN picture TEXT;
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE users
    DROP COLUMN IF EXISTS google_id,
    DROP COLUMN IF EXISTS github_id,
    DROP COLUMN IF EXISTS microsoft_id,
    DROP COLUMN IF EXISTS apple_id,
    DROP COLUMN IF EXISTS picture;

ALTER TABLE tenants
    DROP COLUMN IF EXISTS logo,
    DROP COLUMN IF EXISTS primary_color;
