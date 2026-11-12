-- 题目 10 一致性核查查询（配合 reading-activity-consistency-check 技能使用）
-- 用法：docker exec -i dev-mysql mysql -uroot -p<密码> reading_activity < scripts/consistency_queries.sql
-- 每个查询输出一个可归档的证据结果集；[@uid]/[@date] 替换为目标用户与业务日。

-- Q1 发奖幂等：同一用户同活动同业务日是否存在多条发奖（预期 0 行）
SELECT user_id, activity_id, biz_date, COUNT(*) AS reward_rows
FROM reward GROUP BY user_id, activity_id, biz_date HAVING COUNT(*) > 1;

-- Q2 余额一致：余额 ≠ 流水合计的用户（预期 0 行）
SELECT b.user_id, b.balance, IFNULL(t.sum_points, 0) AS txn_sum
FROM user_balance b
LEFT JOIN (SELECT user_id, SUM(points) AS sum_points FROM point_txn GROUP BY user_id) t
  ON t.user_id = b.user_id
WHERE b.balance <> IFNULL(t.sum_points, 0);

-- Q3 每日进度去重：同用户同日同文章重复计入（预期 0 行，唯一键兜底验证）
SELECT user_id, biz_date, article_id, COUNT(*) AS c
FROM daily_progress GROUP BY user_id, biz_date, article_id HAVING COUNT(*) > 1;

-- Q4 达标未发奖 / 未达标已发奖：进度 distinct 文章数与 reward 的对应关系（预期两侧均 0 行）
SELECT p.user_id, p.biz_date, p.distinct_articles
FROM (SELECT user_id, biz_date, COUNT(DISTINCT article_id) AS distinct_articles
      FROM daily_progress GROUP BY user_id, biz_date) p
LEFT JOIN reward r ON r.user_id = p.user_id AND r.biz_date = p.biz_date
WHERE (p.distinct_articles >= 3 AND r.id IS NULL) OR (p.distinct_articles < 3 AND r.id IS NOT NULL);

-- Q5 归日正确：accepted 且 counted 的事件，biz_date ≠ 发生时间北京日期（预期 0 行）
SELECT event_id, occur_at, biz_date FROM reading_event
WHERE status = 1 AND counted = 1
  AND biz_date <> DATE(CONVERT_TZ(occur_at, '+00:00', '+08:00'));

-- Q6 迟到窗口违规：接受的事件迟到超过 24 小时（预期 0 行）
SELECT event_id, occur_at, received_at, TIMESTAMPDIFF(HOUR, occur_at, received_at) AS late_hours
FROM reading_event
WHERE status = 1 AND TIMESTAMPDIFF(HOUR, occur_at, received_at) > 24;

-- Q7 事件编号内容冲突记录：同编号不同内容的冲突是否留痕（应能看到冲突行）
SELECT event_id, payload, received_at FROM event_conflict ORDER BY received_at DESC LIMIT 20;

-- Q8 活动期外受理：接受的事件发生时间不在任一已发布活动区间（预期 0 行）
SELECT e.event_id, e.occur_at FROM reading_event e
LEFT JOIN activity a ON a.status = 1 AND e.occur_at BETWEEN a.start_at AND a.end_at
WHERE e.status = 1 AND a.id IS NULL;

-- Q9 指定用户某日核查面板：事件数 ≠ 有效文章数 ≠ 发奖 的差异一目了然
SELECT
  (SELECT COUNT(*) FROM reading_event WHERE user_id = @uid AND biz_date = @date AND status = 1) AS accepted_events,
  (SELECT COUNT(DISTINCT article_id) FROM daily_progress WHERE user_id = @uid AND biz_date = @date) AS distinct_articles,
  (SELECT COUNT(*) FROM reward WHERE user_id = @uid AND biz_date = @date) AS rewards,
  (SELECT IFNULL(SUM(points), 0) FROM point_txn WHERE user_id = @uid
     AND created_at >= CONCAT(@date, ' 00:00:00') AND created_at < DATE_ADD(CONCAT(@date, ' 00:00:00'), INTERVAL 1 DAY)) AS txn_points;

-- Q10 客服越权检查：是否存在绕过流水直接改余额的痕迹（余额变化无对应流水）
SELECT b.user_id, b.balance, IFNULL(t.sum_points, 0) AS txn_sum
FROM user_balance b
LEFT JOIN (SELECT user_id, SUM(points) AS sum_points FROM point_txn GROUP BY user_id) t
  ON t.user_id = b.user_id
WHERE b.balance <> IFNULL(t.sum_points, 0);
