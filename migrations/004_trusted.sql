-- 模拟阅读服务自己的可信阅读记录（客服申诉核查的可信证据源，
-- 与主服务的 reading_event 相互独立：事件可能在投递链路中丢失）
CREATE TABLE IF NOT EXISTS trusted_record (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_id   VARCHAR(64)  NOT NULL COMMENT '投递给主服务的事件编号',
    user_id    BIGINT       NOT NULL,
    article_id BIGINT       NOT NULL,
    occur_at   DATETIME(3)  NOT NULL COMMENT '阅读发生时间',
    biz_date   DATE         NOT NULL COMMENT '北京时间业务日',
    created_at DATETIME(3)  DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_event (event_id),
    KEY idx_user_date (user_id, biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='模拟阅读服务可信记录';
