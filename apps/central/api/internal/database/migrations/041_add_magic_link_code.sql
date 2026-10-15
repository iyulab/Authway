-- ============================================================
-- Migration 041: A code alongside each emailed sign-in link
-- ============================================================
-- An emailed sign-in link only completes the sign-in in the browser that
-- started it: the authorization server ties the login to that browser. The
-- email now also carries a short code, typed into the screen that started
-- the sign-in, for the reader who opened the mail on another device.
--
-- code_sealed holds the code sealed with the at-rest key (the same key that
-- seals TOTP secrets) — a short code is too easy to recover from a plain
-- hash. code_attempts counts wrong codes entered for the link's sign-in;
-- after five the code stops working (the link itself still does).
--
-- Additive: links issued before this migration have no code.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

ALTER TABLE magic_link_tokens ADD COLUMN IF NOT EXISTS code_sealed TEXT NOT NULL DEFAULT '';
ALTER TABLE magic_link_tokens ADD COLUMN IF NOT EXISTS code_attempts INTEGER NOT NULL DEFAULT 0;
