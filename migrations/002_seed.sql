-- 演示数据：单一固定活动（题面：持续两个业务日）
-- 业务日为 2026-11-11 与 2026-11-12（北京时间），演示期可按教学时钟调整。
-- 时间以 UTC 存储：北京 2026-11-11 00:00 = UTC 2026-11-10 16:00。
INSERT INTO activity (name, start_at, end_at, daily_threshold, reward_points, status)
SELECT '每日阅读三篇文章得积分', '2026-11-10 16:00:00.000', '2026-11-12 16:00:00.000', 3, 30, 1
WHERE NOT EXISTS (SELECT 1 FROM activity WHERE status = 1);
