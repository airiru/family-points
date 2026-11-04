-- 0001_init.up.sql —— v3 初始表结构
-- 与 docs/features.md 第 3 节逐字段一致；改动必须两边同步。
-- 约定：不建外键，一致性由 service 层事务 + 唯一索引保证。

CREATE TABLE IF NOT EXISTS families (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name            VARCHAR(64)  NOT NULL,
  invite_code     CHAR(8)      NOT NULL UNIQUE,
  invite_code_expires_at DATETIME(3) NULL,    -- 邀请码 7 天有效期（5.2.5）
  created_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 被踢出 / 自行退出后的禁入名单（同组不可再次加入）
CREATE TABLE IF NOT EXISTS family_bans (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  reason     ENUM('kicked','left') NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_family_member (family_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 后台管理员账号（网站运营者，不属于任何家庭组，不参与积分）
CREATE TABLE IF NOT EXISTS admins (
  id                   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username             VARCHAR(20)  NOT NULL UNIQUE,
  pass_hash            VARCHAR(60)  NOT NULL,
  must_change_password TINYINT(1)   NOT NULL DEFAULT 1,
  disabled             TINYINT(1)   NOT NULL DEFAULT 0,
  created_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 成员（账号即成员）
CREATE TABLE IF NOT EXISTS members (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id        BIGINT UNSIGNED NULL,
  name             VARCHAR(32)  NOT NULL,
  score            INT          NOT NULL DEFAULT 0,
  frozen_score     INT          NOT NULL DEFAULT 0,
  hidden           TINYINT(1)   NOT NULL DEFAULT 0,
  avatar           VARCHAR(64)  NULL,
  username         VARCHAR(20)  NOT NULL UNIQUE,
  pass_hash        VARCHAR(60)  NOT NULL,
  role             ENUM('leader','member') NOT NULL DEFAULT 'member',
  leader_family_id BIGINT UNSIGNED GENERATED ALWAYS AS (IF(role='leader', family_id, NULL)) STORED,
  created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id),
  UNIQUE KEY uk_leader_per_family (leader_family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 计分规则（系统里只有一种：睡觉规则）
CREATE TABLE IF NOT EXISTS rules (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  points     INT          NOT NULL,
  streaks    JSON         NULL,
  member_ids JSON         NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 兑换物品
CREATE TABLE IF NOT EXISTS items (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  name       VARCHAR(64)  NOT NULL,
  cost       INT          NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 积分记录
CREATE TABLE IF NOT EXISTS records (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  member_name  VARCHAR(32)  NOT NULL,
  title        VARCHAR(128) NOT NULL,
  points       INT          NOT NULL,
  occurred_at  DATETIME(3)  NOT NULL,
  added_at     DATETIME(3)  NOT NULL,
  streak_award JSON         NULL,
  source       ENUM('screen','night','task','transfer','redeem') NOT NULL,
  transfer_id  BIGINT UNSIGNED NULL,
  redemption_id BIGINT UNSIGNED NULL,        -- 指向兑换单，与 transfer_id 对称
  KEY idx_family_time (family_id, occurred_at DESC),
  KEY idx_member_time (member_id, occurred_at DESC),
  KEY idx_transfer (transfer_id),
  KEY idx_redemption (redemption_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 积分赠送（转移关系，便于审计与撤销）
CREATE TABLE IF NOT EXISTS transfers (
  id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id      BIGINT UNSIGNED NOT NULL,
  from_member_id BIGINT UNSIGNED NOT NULL,
  to_member_id   BIGINT UNSIGNED NOT NULL,
  amount         INT          NOT NULL,
  note           VARCHAR(64)  NULL,
  created_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at     DATETIME(3)  NULL,
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_from (from_member_id),
  KEY idx_to (to_member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务（悬赏转移：发布者冻结 → 完成者得）
CREATE TABLE IF NOT EXISTS tasks (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  title         VARCHAR(128) NOT NULL,
  points        INT          NOT NULL,
  publisher_id  BIGINT UNSIGNED NOT NULL,
  escrow_points INT          NOT NULL,
  assignee_id   BIGINT UNSIGNED NULL,
  completed_by  BIGINT UNSIGNED NULL,
  due_at        DATETIME(3)  NULL,
  repeat_rule   VARCHAR(32)  NULL,
  status        ENUM('open','taken','closed') NOT NULL DEFAULT 'open',
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC),
  KEY idx_publisher (publisher_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务提交（submitted → approved / rejected / expired）
CREATE TABLE IF NOT EXISTS task_submissions (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  task_id       BIGINT UNSIGNED NOT NULL,
  member_id     BIGINT UNSIGNED NOT NULL,
  note          VARCHAR(255) NOT NULL,
  status        ENUM('submitted','approved','rejected','expired') NOT NULL DEFAULT 'submitted',
  vote_deadline DATETIME(3)  NULL,
  review_note   VARCHAR(255) NULL,
  reviewed_at   DATETIME(3)  NULL,
  record_id     BIGINT UNSIGNED NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_status (family_id, status, created_at DESC),
  KEY idx_task (task_id),
  KEY idx_task_member (task_id, member_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 任务审核投票（除发布者外全员过半同意）
CREATE TABLE IF NOT EXISTS task_votes (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  submission_id BIGINT UNSIGNED NOT NULL,
  voter_id      BIGINT UNSIGNED NOT NULL,
  vote          ENUM('approve','reject') NOT NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_submission_voter (submission_id, voter_id),
  KEY idx_submission (submission_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 操作审计日志（append-only）
CREATE TABLE IF NOT EXISTS audit_logs (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NULL,
  operator_id   BIGINT UNSIGNED NULL,
  operator_type ENUM('member','device','system') NOT NULL DEFAULT 'member',
  action        VARCHAR(48)  NOT NULL,
  target_type   VARCHAR(24)  NULL,
  target_id     BIGINT UNSIGNED NULL,
  before_json   JSON         NULL,
  after_json    JSON         NULL,
  ip            VARCHAR(45)  NULL,
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_target (target_type, target_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 站内通知
CREATE TABLE IF NOT EXISTS notifications (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  type       VARCHAR(32)  NOT NULL,
  title      VARCHAR(64)  NOT NULL,
  body       VARCHAR(255) NULL,
  ref_type   VARCHAR(24)  NULL,
  ref_id     BIGINT UNSIGNED NULL,
  read_at    DATETIME(3)  NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_member_unread (member_id, read_at, created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间规则（每成员一条）
CREATE TABLE IF NOT EXISTS screen_rules (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id        BIGINT UNSIGNED NOT NULL,
  member_id        BIGINT UNSIGNED NOT NULL UNIQUE,
  threshold_min    INT NOT NULL,
  bonus_points     INT NOT NULL,
  penalty_points   INT NOT NULL,
  enabled          TINYINT(1) NOT NULL DEFAULT 1,
  night_start_hour TINYINT UNSIGNED NULL,
  night_end_hour   TINYINT UNSIGNED NULL,
  night_max_min    INT NULL,
  night_rule_id    BIGINT UNSIGNED NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 屏幕时间结算（幂等载体：UNIQUE(member_id, report_date)）
CREATE TABLE IF NOT EXISTS screen_reports (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id       BIGINT UNSIGNED NOT NULL,
  member_id       BIGINT UNSIGNED NOT NULL,
  report_date     DATE         NOT NULL,
  total_min       INT          NOT NULL,
  night_min       INT          NULL,
  night_ok        TINYINT(1)   NULL,         -- 夜间判定快照；NULL = 当日未做判定
  points          INT          NOT NULL,
  record_id       BIGINT UNSIGNED NULL,
  night_record_id BIGINT UNSIGNED NULL,
  source          ENUM('auto') NOT NULL DEFAULT 'auto',
  revision        INT          NOT NULL DEFAULT 1,
  reported_at     DATETIME(3)  NOT NULL,
  updated_at      DATETIME(3)  NULL,
  UNIQUE KEY uk_member_date (member_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 设备令牌
CREATE TABLE IF NOT EXISTS devices (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id    BIGINT UNSIGNED NOT NULL,
  member_id    BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(32)  NOT NULL,
  token_hash   CHAR(64)     NOT NULL,
  revoked      TINYINT(1)   NOT NULL DEFAULT 0,
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  last_seen_at DATETIME(3)  NULL,
  KEY idx_token (token_hash),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


-- ============================================================
-- 以下四张表原为 0002–0005 的增量迁移，2026-11-04 开发阶段 squash 进来。
-- 现在**只有这一个迁移文件**：改表就改这里 + `migrate -reset` 重建。
-- 等到真正上线、库里有不可丢的真实数据时，才开始写 0002+ 的增量迁移。
-- ============================================================

-- 可吊销的 refresh token（改密 / 登出 / 轮换时旧 token 必须能立即失效）
CREATE TABLE IF NOT EXISTS refresh_tokens (
  id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  subject_type     ENUM('admin','member') NOT NULL,
  subject_id       BIGINT UNSIGNED NOT NULL,
  token_hash       CHAR(64)     NOT NULL UNIQUE,   -- 只存 sha256 hex
  expires_at       DATETIME(3)  NOT NULL,
  revoked_at       DATETIME(3)  NULL,              -- 非空 = 已失效
  replaced_by_hash CHAR(64)     NULL,              -- 轮换出的新 token，便于追溯
  created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_subject (subject_type, subject_id),
  KEY idx_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 兑换单（承载「这张单是否已撤销」的状态，records 是 append-only 流水不承载状态）
CREATE TABLE IF NOT EXISTS redemptions (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  item_id    BIGINT UNSIGNED NULL,          -- 商品可能被删，允许为空
  item_name  VARCHAR(64)  NOT NULL,         -- 快照：商品改名/删除不影响历史
  cost       INT          NOT NULL,         -- 兑换时的价格快照
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at DATETIME(3)  NULL,             -- 预留：撤销功能暂不做（9.4）
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_member (member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 商品券（兑换后生成，与兑换单 1:1；券存快照，商品改动不影响已发的券）
CREATE TABLE IF NOT EXISTS coupons (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  member_id     BIGINT UNSIGNED NOT NULL,   -- 持有者 = 兑换者
  redemption_id BIGINT UNSIGNED NOT NULL,   -- 来源兑换单，1:1
  item_name     VARCHAR(64)  NOT NULL,      -- 快照
  cost          INT          NOT NULL,      -- 价格快照
  status        ENUM('unused','pending','redeemed') NOT NULL DEFAULT 'unused',
  reject_count  INT          NOT NULL DEFAULT 0,
  vote_deadline DATETIME(3)  NULL,          -- 核销投票截止
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  settled_at    DATETIME(3)  NULL,          -- 核销完成时间
  UNIQUE KEY uk_redemption (redemption_id),
  KEY idx_member_status (member_id, status),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 券核销投票（与 task_votes 同构：一人一票、投后不可改）
CREATE TABLE IF NOT EXISTS coupon_votes (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  coupon_id  BIGINT UNSIGNED NOT NULL,
  voter_id   BIGINT UNSIGNED NOT NULL,
  vote       ENUM('approve','reject') NOT NULL,
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_coupon_voter (coupon_id, voter_id),
  KEY idx_coupon (coupon_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
