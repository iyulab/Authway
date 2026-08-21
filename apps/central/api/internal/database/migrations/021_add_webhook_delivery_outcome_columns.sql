-- ============================================================
-- Migration 021: Webhook Delivery Outcome Columns
-- ============================================================
-- The GORM model webhook.WebhookDelivery declares Success (bool) and
-- ErrorMessage (string), which GORM maps to columns `success` and
-- `error_message` — neither of which migration 006 ever created (it has
-- `error` and `duration_ms` instead, which the model does not use). Every
-- INSERT the delivery loop in pkg/webhook/service.go issues therefore fails
-- with SQLSTATE 42703 ("column success does not exist"), and since every
-- call site discards db.Create's return error, this failed silently: no
-- webhook delivery has ever been recorded to webhook_deliveries against the
-- migrated schema, regardless of whether the HTTP POST to the target URL
-- itself succeeded.
--
-- Same defect class as migration 018 (email token bookkeeping): a model
-- declared columns no migration created, undetected because the schema
-- contract test (internal/database/schema_contract_test.go) covers
-- webhook.Webhook but never enrolled webhook.WebhookDelivery. Fixed alongside
-- this migration.
--
-- Guarded with IF NOT EXISTS so the file is safe on databases that were ever
-- provisioned via AutoMigrate (dev) as well as migrated ones. Additive only —
-- the pre-existing `error`/`duration_ms` columns are left in place (unused by
-- the model, but dropping them is out of scope for a column-add fix).
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction; an inner COMMIT would leak-commit that outer tx and
-- defeat its all-or-nothing guarantee. Migration files must contain no
-- transaction-control statements.
-- ============================================================

ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS success BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS error_message TEXT;

COMMENT ON COLUMN webhook_deliveries.success IS 'Whether the delivery attempt received a 2xx response.';
COMMENT ON COLUMN webhook_deliveries.error_message IS 'Transport/HTTP error for a failed attempt; NULL on success.';

-- ============================================================
-- Verification query:
-- SELECT column_name FROM information_schema.columns
-- WHERE table_name = 'webhook_deliveries' AND column_name IN ('success', 'error_message');
-- Expected: 2 rows
-- ============================================================
