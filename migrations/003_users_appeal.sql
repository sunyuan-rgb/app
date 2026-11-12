-- 用户账号（预置账号，角色可信：通过令牌识别身份，禁止请求参数指定用户编号）
CREATE TABLE IF NOT EXISTS user_account (
    id       BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(64) NOT NULL COMMENT '账号名',
    role     VARCHAR(16) NOT NULL DEFAULT 'user' COMMENT 'user用户/agent客服/ops运营',
    token    VARCHAR(64) NOT NULL COMMENT '访问令牌(教学环境固定预置)',
    UNIQUE KEY uk_username (username),
    UNIQUE KEY uk_token (token)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '预置账号';

-- 申诉"同一用户同一天仅一条未结申诉"：
-- 用生成列 open_flag（未结=1，已结=NULL）+ 唯一键实现数据库级约束
ALTER TABLE appeal
    ADD COLUMN open_flag TINYINT
        GENERATED ALWAYS AS (IF(status = 0, 1, NULL)) STORED,
    ADD UNIQUE KEY uk_user_day_open (user_id, biz_date, open_flag);

-- 预置账号：5 名用户 + 1 客服 + 1 运营（令牌为教学固定值，仅演示环境使用）
INSERT INTO user_account (username, role, token) VALUES
    ('user1', 'user',  'user-token-1'),
    ('user2', 'user',  'user-token-2'),
    ('user3', 'user',  'user-token-3'),
    ('user4', 'user',  'user-token-4'),
    ('user5', 'user',  'user-token-5'),
    ('agent1', 'agent', 'agent-token-1'),
    ('ops1',  'ops',   'ops-token-1')
ON DUPLICATE KEY UPDATE username = VALUES(username);
