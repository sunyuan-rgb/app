-- 题目 10 跨环节一致性核查（确定性查询，全部可归档）
-- 依据：业务日按事件发生时间的北京日期（UTC 存储，+8h 换算）；
--       去重/幂等依赖唯一键 uk_user_date_article / uk_user_activity_date / uk_event_id。
-- 运行：docker exec dev-mysql mysql -uroot -p<密码> reading_activity < scripts/consistency_queries.sql

-- Q1 文章去重：同一用户同一业务日同一文章不得有多条进度（应为 0 行）
SELECT 'Q1_article_dup' AS check_item, user_id, biz_date, article_id, COUNT(*) AS cnt
FROM daily_progress GROUP BY user_id, biz_date, article_id HAVING COUNT(*) > 1;

-- Q2 发奖幂等：同一用户同一活动同一业务日不得有多条奖励（应为 0 行）
SELECT 'Q2_reward_dup' AS check_item, user_id, activity_id, biz_date, COUNT(*) AS cnt
FROM reward GROUP BY user_id, activity_id, biz_date HAVING COUNT(*) > 1;

-- Q3 余额与流水一致：balance ≠ 流水合计的用户（应为 0 行）
SELECT 'Q3_balance_mismatch' AS check_item, b.user_id, b.balance,
       IFNULL(SUM(t.points), 0) AS txn_sum
FROM user_balance b LEFT JOIN point_txn t ON t.user_id = b.user_id
GROUP BY b.user_id, b.balance HAVING b.balance <> IFNULL(SUM(t.points), 0);

-- Q4 业务日归属：biz_date 必须等于 occur_at 的北京日期（应为 0 行）
SELECT 'Q4_bizdate_mismatch' AS check_item, event_id, biz_date,
       DATE(occur_at + INTERVAL 8 HOUR) AS expected_bj_date
FROM reading_event WHERE biz_date <> DATE(occur_at + INTERVAL 8 HOUR);

-- Q5 计数一致性：counted=1 的事件必须有对应进度行（应为 0 行）
SELECT 'Q5_counted_without_progress' AS check_item, e.event_id, e.user_id, e.biz_date, e.article_id
FROM reading_event e
LEFT JOIN daily_progress p
  ON p.user_id = e.user_id AND p.biz_date = e.biz_date AND p.article_id = e.article_id
WHERE e.status = 1 AND e.counted = 1 AND p.id IS NULL;

-- Q6 发奖与达标对应：当日不同文章数 ≥ 门槛但未发奖，或 < 门槛却发奖（应为 0 行）
SELECT 'Q6_reward_vs_threshold' AS check_item, g.user_id, g.biz_date, g.distinct_articles, r.id AS reward_id
FROM (
    SELECT p.user_id, p.biz_date, COUNT(DISTINCT p.article_id) AS distinct_articles
    FROM daily_progress p JOIN activity a ON a.status = 1
    WHERE p.biz_date BETWEEN DATE(a.start_at + INTERVAL 8 HOUR) AND DATE(a.end_at + INTERVAL 8 HOUR)
    GROUP BY p.user_id, p.biz_date
) g LEFT JOIN reward r
  ON r.user_id = g.user_id AND r.biz_date = g.biz_date AND r.activity_id = (SELECT id FROM activity WHERE status = 1)
WHERE (g.distinct_articles >= (SELECT daily_threshold FROM activity WHERE status = 1) AND r.id IS NULL)
   OR (g.distinct_articles <  (SELECT daily_threshold FROM activity WHERE status = 1) AND r.id IS NOT NULL);

-- Q7 冲突留痕：同编号换内容的冲突请求（列示，供核查处理结果）
SELECT 'Q7_event_conflicts' AS check_item, event_id, payload, received_at
FROM event_conflict ORDER BY id;

-- Q8 拒绝事件留痕：按拒绝原因统计（列示）
SELECT 'Q8_rejected_events' AS check_item, reject_reason, COUNT(*) AS cnt
FROM reading_event WHERE status = 2 GROUP BY reject_reason;

-- Q9 越界迟到窗口：受理事件中 接收-发生 > 24h（应为 0 行）
SELECT 'Q9_late_window_violation' AS check_item, event_id,
       TIMESTAMPDIFF(MINUTE, occur_at, received_at) AS late_minutes
FROM reading_event WHERE status = 1 AND TIMESTAMPDIFF(MINUTE, occur_at, received_at) > 24 * 60;

-- Q10 活动期外受理：受理事件的发生时间不在 [start_at, end_at]（应为 0 行）
SELECT 'Q10_outside_activity' AS check_item, event_id, occur_at
FROM reading_event e JOIN activity a ON a.status = 1
WHERE e.status = 1 AND (e.occur_at < a.start_at OR e.occur_at > a.end_at);

-- Q11 未结申诉唯一：同一用户同一业务日不得有多条待处理申诉（应为 0 行）
SELECT 'Q11_open_appeal_dup' AS check_item, user_id, biz_date, COUNT(*) AS cnt
FROM appeal WHERE status = 0 GROUP BY user_id, biz_date HAVING COUNT(*) > 1;

-- Q12 孤儿流水：ref_type='reward' 的流水必须指向存在的奖励行（应为 0 行）
SELECT 'Q12_orphan_reward_txn' AS check_item, t.id, t.user_id, t.ref_id
FROM point_txn t LEFT JOIN reward r ON r.id = t.ref_id
WHERE t.ref_type = 'reward' AND r.id IS NULL;

-- Q13 总览（交叉核对用：达标人数 vs 发奖人数、余额合计 vs 流水合计）
SELECT 'Q13_overview' AS check_item,
       (SELECT COUNT(*) FROM daily_progress) AS progress_rows,
       (SELECT COUNT(DISTINCT CONCAT(user_id, '-', biz_date)) FROM daily_progress) AS user_days,
       (SELECT COUNT(*) FROM reward) AS reward_rows,
       (SELECT COUNT(*) FROM reading_event WHERE status = 1) AS accepted_events,
       (SELECT COUNT(*) FROM reading_event WHERE status = 2) AS rejected_events,
       (SELECT IFNULL(SUM(points), 0) FROM point_txn) AS txn_total,
       (SELECT IFNULL(SUM(balance), 0) FROM user_balance) AS balance_total,
       (SELECT COUNT(*) FROM event_conflict) AS conflict_rows;
