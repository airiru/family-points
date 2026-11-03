-- 0002_auth.up.sql —— P1：可吊销的 refresh token
--
-- 为什么落库而不是纯无状态 JWT：改密码 / 登出 / 轮换时旧 token 必须能立即失效。
-- ⚠️ 本迁移只做 CREATE TABLE —— 开发库账号没有 ALTER 权限（见 docs 24.5），
--    涉及已有表的字段变更必须先补权限，或走「重建表」方案。
CREATE TABLE IF NOT EXISTS refresh_tokens (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  subject_type     ENUM('admin','member') NOT NULL,
  subject_id       BIGINT UNSIGNED NOT NULL,
  token_hash       CHAR(64)     NOT NULL UNIQUE,   -- sha256 hex
  expires_at       DATETIME(3)  NOT NULL,
  revoked_at       DATETIME(3)  NULL,              -- 非空 = 已失效（轮换/改密/登出）
  replaced_by_hash CHAR(64)     NULL,              -- 轮换后的新 token，用于追溯
  created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_subject (subject_type, subject_id),
  KEY idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
