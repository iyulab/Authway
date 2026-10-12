-- ============================================================
-- Migration 027: Client audience (resource identifiers)
-- ============================================================
-- A client's access tokens carried no `aud`: nothing said which API a token
-- was meant for, so an API could only check the issuer and the signature and
-- accepted the tokens of every other application of the same issuer.
--
-- `audience` lists the resource servers (APIs) a client's access tokens are
-- for. It is synced to the authorization server as the client's allowed
-- audience and granted when the user consents, which puts it in the token's
-- `aud` claim.
--
-- Empty = the client names no resource; its tokens keep an empty `aud`, as
-- before. This migration sets it for no client.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; migration files must contain no transaction-control
-- statements.
-- ============================================================

ALTER TABLE clients ADD COLUMN IF NOT EXISTS audience TEXT[] DEFAULT '{}';

COMMENT ON COLUMN clients.audience IS 'Resource servers this client''s access tokens are for (the aud claim). Propagated to Hydra client.audience.';

-- Verification:
-- SELECT count(*) FROM clients WHERE audience IS NOT NULL AND cardinality(audience) > 0;  -- expect 0
