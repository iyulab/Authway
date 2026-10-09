-- ============================================================
-- Migration 025: Record failed audit events as failed
-- ============================================================
-- audit_logs.success was mapped with a GORM default of true, and GORM leaves a
-- field that declares a default out of the INSERT when it holds its zero value.
-- Every event logged as a failure was therefore stored with success = true:
-- failed sign-ins, failed MFA, refused admin calls. The mapping no longer
-- declares a default, so new rows are written as logged.
--
-- This corrects the rows already written. An event is a failure when its
-- action names one or when it carries an error message; both are written only
-- on the failure path.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

UPDATE audit_logs
SET success = false
WHERE success = true
  AND (action IN ('user.login_failed', 'user.mfa_failed')
       OR COALESCE(error_msg, '') <> '');
