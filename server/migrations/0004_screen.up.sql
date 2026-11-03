-- 0004_screen.up.sql —— P5 屏幕时间结算所需的快照字段
--
-- 为什么要 night_ok：连续天数由 screen_reports **回溯计算**（决策 7：不落计数器），
-- 而「改配置不追溯历史」要求历史判定结果必须被快照下来，
-- 不能用当前配置回头重算过去每一天。
ALTER TABLE screen_reports
  ADD COLUMN night_ok TINYINT(1) NULL AFTER night_min;
-- NULL = 该日未做夜间判定（未启用夜间规则）
