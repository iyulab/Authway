-- Migration: Align audit_logs schema with Run-3 P4 wiring
-- Version: 009
-- Date: 2026-04-16
-- Purpose: Add actor_type / details / error_msg columns expected by AuditLog GORM model.
--          Migration 006 created the table with description/metadata, but commits
--          6d14e54..27ccf9c (P4 audit wiring) introduced new columns without a
--          corresponding migration. Without 009 the API panics with:
--          "column \"actor_type\" of relation \"audit_logs\" does not exist".

ALTER TABLE audit_logs
    ADD COLUMN IF NOT EXISTS actor_type VARCHAR(50),
    ADD COLUMN IF NOT EXISTS details    JSONB,
    ADD COLUMN IF NOT EXISTS error_msg  TEXT;

-- Backfill: 기존 metadata(jsonb)를 details로 복사 (양쪽 공존 허용,
-- 운영 안정 후 후속 마이그레이션에서 metadata/description 정리 예정).
UPDATE audit_logs
   SET details = metadata
 WHERE details IS NULL
   AND metadata IS NOT NULL;

-- audit_logs는 append-only 이력이므로 tenant/actor 삭제와 독립적으로 보존되어야 함.
-- 또한 시스템 이벤트(예: 비인증 admin 라우트)에서 actor_id/tenant_id가
-- uuid.Nil(00000000-...)로 들어와 FK 위반이 나는 문제가 있어 FK를 제거한다.
ALTER TABLE audit_logs
    DROP CONSTRAINT IF EXISTS audit_logs_tenant_id_fkey,
    DROP CONSTRAINT IF EXISTS audit_logs_actor_id_fkey;
