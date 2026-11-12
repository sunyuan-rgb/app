-- 题目 10：资讯 App 阅读活动 初始表结构
-- 说明：幂等约束全部依赖唯一键，应用层判断仅作辅助。

CREATE TABLE IF NOT EXISTS activity (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(128)  NOT NULL COMMENT '活动名称',
    start_at        DATETIME(3)   NOT NULL COMMENT '开始时间(UTC存储)',
    end_at          DATETIME(3)   NOT NULL COMMENT '结束时间(UTC存储)',
    daily_threshold INT           NOT NULL DEFAULT 3 COMMENT '每日达标所需不同文章数',
    reward_points   INT           NOT NULL DEFAULT 30 COMMENT '达标奖励积分',
    status          TINYINT       NOT NULL DEFAULT 0 COMMENT '0草稿 1已发布 2已结束',
    created_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '阅读活动';

CREATE TABLE IF NOT EXISTS reading_event (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_id      VARCHAR(64)  NOT NULL COMMENT '全局唯一事件编号',
    activity_id   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    user_id       BIGINT UNSIGNED NOT NULL,
    article_id    BIGINT UNSIGNED NOT NULL,
    occur_at      DATETIME(3)  NOT NULL COMMENT '发生时间(UTC存储)',
    received_at   DATETIME(3)  NOT NULL COMMENT '接收时间(UTC存储)',
    biz_date      DATE         NOT NULL COMMENT '业务日=发生时间的北京日期',
    source        TINYINT      NOT NULL DEFAULT 1 COMMENT '1自动投递 2客服补入',
    status        TINYINT      NOT NULL DEFAULT 1 COMMENT '1已接受 2已拒绝',
    counted       TINYINT      NOT NULL DEFAULT 0 COMMENT '是否计入进度(去重后)',
    reject_reason VARCHAR(255) NOT NULL DEFAULT '',
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_event_id (event_id),
    KEY idx_user_biz_date (user_id, biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '阅读事件(含拒绝记录)';

CREATE TABLE IF NOT EXISTS event_conflict (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_id    VARCHAR(64)  NOT NULL,
    payload     VARCHAR(1024) NOT NULL COMMENT '冲突请求内容摘要',
    received_at DATETIME(3)  NOT NULL,
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_event_id (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '同编号换内容的冲突记录';

CREATE TABLE IF NOT EXISTS daily_progress (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    biz_date   DATE            NOT NULL,
    article_id BIGINT UNSIGNED NOT NULL,
    event_id   VARCHAR(64)     NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_user_date_article (user_id, biz_date, article_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '用户每日阅读进度(文章去重载体)';

CREATE TABLE IF NOT EXISTS reward (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    activity_id BIGINT UNSIGNED NOT NULL,
    biz_date    DATE            NOT NULL,
    points      INT             NOT NULL,
    created_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_user_activity_date (user_id, activity_id, biz_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '发奖记录(每日只发一次的幂等载体)';

CREATE TABLE IF NOT EXISTS point_txn (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id    BIGINT UNSIGNED NOT NULL,
    type       TINYINT         NOT NULL COMMENT '1奖励发放',
    points     INT             NOT NULL,
    ref_type   VARCHAR(32)     NOT NULL COMMENT '关联类型: reward/appeal',
    ref_id     BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '积分流水';

CREATE TABLE IF NOT EXISTS user_balance (
    user_id    BIGINT UNSIGNED PRIMARY KEY,
    balance    INT         NOT NULL DEFAULT 0,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '积分余额';

CREATE TABLE IF NOT EXISTS appeal (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT UNSIGNED NOT NULL,
    activity_id   BIGINT UNSIGNED NOT NULL,
    biz_date      DATE            NOT NULL COMMENT '申诉的业务日',
    content       VARCHAR(1024)   NOT NULL COMMENT '申诉描述',
    status        TINYINT         NOT NULL DEFAULT 0 COMMENT '0待处理 1已补发 2已驳回',
    conclusion    VARCHAR(1024)   NOT NULL DEFAULT '' COMMENT '客服处理结论与依据',
    operator_id   BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '处理客服',
    handled_at    DATETIME(3)     NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    KEY idx_user_open (user_id, activity_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '用户申诉';
