-- 0003_ledger.up.sql —— P4 账目：兑换单 + 记录与兑换单的关联
--
-- 为什么单独建 redemptions 表（而不是只写 records）：
--   撤销兑换需要「这张单是否已撤销」的状态位，records 是 append-only 流水，
--   不适合承载状态。transfers 已经是这个模式，兑换保持一致。
CREATE TABLE IF NOT EXISTS redemptions (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  family_id  BIGINT UNSIGNED NOT NULL,
  member_id  BIGINT UNSIGNED NOT NULL,
  item_id    BIGINT UNSIGNED NULL,          -- 商品可能被删，允许为空
  item_name  VARCHAR(64)  NOT NULL,         -- 快照：商品改名/删除不影响历史
  cost       INT          NOT NULL,         -- 兑换时的价格快照
  created_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  revoked_at DATETIME(3)  NULL,             -- 非空 = 已撤销（积分已退回）
  KEY idx_family_time (family_id, created_at DESC),
  KEY idx_member (member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 记录指向兑换单（与 transfer_id 对称），便于流水回溯
ALTER TABLE records
  ADD COLUMN redemption_id BIGINT UNSIGNED NULL AFTER transfer_id,
  ADD KEY idx_redemption (redemption_id);
