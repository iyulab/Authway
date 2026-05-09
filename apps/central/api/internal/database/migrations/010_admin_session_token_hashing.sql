-- 010: admin_sessions.token → token_hash (security hardening)
-- 기존 세션 전체 무효화: 평문 토큰을 해시로 역변환 불가 → 모든 admin이 재로그인 필요.
-- 인덱스 재생성 + 컬럼 타입을 TEXT → VARCHAR(64) (SHA-256 hex는 항상 64자).

DELETE FROM admin_sessions;

DROP INDEX IF EXISTS idx_admin_sessions_token;

ALTER TABLE admin_sessions RENAME COLUMN token TO token_hash;
ALTER TABLE admin_sessions ALTER COLUMN token_hash TYPE VARCHAR(64);

CREATE UNIQUE INDEX idx_admin_sessions_token_hash ON admin_sessions(token_hash);

COMMENT ON COLUMN admin_sessions.token_hash IS 'SHA-256 hex digest of the session token. Plaintext is never stored in the DB.';
