-- ============================================================
-- Migration 037: Webhook signing secrets in the Standard Webhooks form
-- ============================================================
-- Deliveries are signed as Standard Webhooks specifies: the secret is
-- "whsec_" followed by the base64 of the key bytes. Secrets issued before
-- were 64 hex characters, and the HMAC key was those characters themselves.
-- This rewrites each such secret into the new form over the same key bytes,
-- so a receiver keeps verifying with the secret it holds once it writes it
-- as "whsec_" + base64(<old secret>) — the same value this stores.
--
-- Rows already in the new form are left as they are. To go back:
--   UPDATE webhooks SET secret = convert_from(decode(substr(secret, 7), 'base64'), 'UTF8')
--   WHERE secret LIKE 'whsec_%';
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

UPDATE webhooks
SET secret = 'whsec_' || replace(encode(convert_to(secret, 'UTF8'), 'base64'), E'\n', '')
WHERE secret NOT LIKE 'whsec_%';
