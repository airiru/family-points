-- 0005_coupons.up.sql —— P9 商品券与核销投票
--
-- 兑换不再是终点：扣分 → 发券 → 用券 → 全员投票核销（docs 9.6）。
-- 券存 item_name / cost 快照，商品改名或删除不影响已发的券。
--
-- 本迁移只做 CREATE TABLE（开发库账号没有 ALTER 权限，见 0002 的说明）。
CREATE TABLE IF NOT EXISTS coupons (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id     BIGINT UNSIGNED NOT NULL,
  member_id     BIGINT UNSIGNED NOT NULL,   -- 持有者 = 兑换者
  redemption_id BIGINT UNSIGNED NOT NULL,   -- 来源兑换单，1:1
  item_name     VARCHAR(64)  NOT NULL,      -- 快照
  cost          INT          NOT NULL,      -- 价格快照
  status        ENUM('unused','pending','redeemed') NOT NULL DEFAULT 'unused',
  reject_count  INT          NOT NULL DEFAULT 0,   -- 被驳回次数
  vote_deadline DATETIME(3)  NULL,          -- 核销投票截止
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  settled_at    DATETIME(3)  NULL,          -- 核销完成时间
  UNIQUE KEY uk_redemption (redemption_id),
  KEY idx_member_status (member_id, status),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 核销投票：与 task_votes 同构，一人一票、投后不可改。
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
