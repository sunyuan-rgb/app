// Package readingsvc 模拟阅读服务：用户阅读的可信记录源 + 向主服务投递事件 +
// 异常场景触发器（重复/换内容/未来/跨日迟到/超窗/丢失投递）。
package readingsvc

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
)

// Service 模拟阅读服务。
type Service struct {
	db      *sql.DB
	mainURL string // 主服务地址
	token   string // 主服务事件接入令牌
	clk     clock.Clock
	httpc   *http.Client
}

func New(db *sql.DB, clk clock.Clock, mainURL, token string) *Service {
	return &Service{db: db, clk: clk, mainURL: mainURL, token: token,
		httpc: &http.Client{Timeout: 5 * time.Second}}
}

// Read 用户阅读一篇文章：先落可信记录（事实已发生），再向主服务投递事件。
// drop=true 模拟"链路积压/消息丢失"：可信记录存在但事件未送达（申诉补发演示用）。
func (s *Service) Read(ctx context.Context, userID, articleID int64, drop bool) (map[string]any, error) {
	now := s.clk.Now()
	eventID := fmt.Sprintf("rs-%d-%d", userID, now.UnixNano())
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO trusted_record (event_id, user_id, article_id, occur_at, biz_date)
		VALUES (?, ?, ?, ?, ?)`,
		eventID, userID, articleID, now.UTC(), model.BizDate(now)); err != nil {
		return nil, err
	}
	out := map[string]any{
		"event_id":  eventID,
		"occur_at":  now.Format(time.RFC3339),
		"biz_date":  model.BizDate(now),
		"delivered": !drop,
	}
	if !drop {
		out["delivery"] = json.RawMessage(s.deliver(ctx, eventID, userID, articleID, now))
	}
	return out, nil
}

// deliver 向主服务投递事件，原样返回主服务响应体（含拒绝结果）。
func (s *Service) deliver(ctx context.Context, eventID string, userID, articleID int64, occurAt time.Time) []byte {
	payload, _ := json.Marshal(map[string]any{
		"event_id":   eventID,
		"user_id":    userID,
		"article_id": articleID,
		"occur_at":   occurAt.Format(time.RFC3339),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.mainURL+"/api/v1/events", bytes.NewReader(payload))
	if err != nil {
		return mustJSONError("build request: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Source-Token", s.token)
	resp, err := s.httpc.Do(req)
	if err != nil {
		return mustJSONError("deliver failed: " + err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body
}

// SimulateInput 异常场景触发输入。
type SimulateInput struct {
	Scenario  string `json:"scenario"` // duplicate/conflict/future/late_cross_day/too_late/cross_day_normal
	UserID    int64  `json:"user_id"`
	ArticleID int64  `json:"article_id"`
	EventID   string `json:"event_id,omitempty"`
	OccurAt   string `json:"occur_at,omitempty"` // scenario=custom 时使用
}

// Simulate 构造并投递异常事件（不写可信记录——这些是投递链路异常，不是真实阅读）。
func (s *Service) Simulate(ctx context.Context, in SimulateInput) (map[string]any, error) {
	now := s.clk.Now()
	newID := func() string {
		if in.EventID != "" {
			return in.EventID
		}
		return fmt.Sprintf("sim-%s-%d", in.Scenario, now.UnixNano())
	}
	var eid string
	var occur time.Time
	var deliverTwice bool
	var secondArticle int64

	switch in.Scenario {
	case "duplicate": // 同编号同内容重复投递
		eid = newID()
		occur = now.Add(-time.Hour)
		deliverTwice = true
	case "conflict": // 同编号不同内容
		eid = newID()
		occur = now.Add(-time.Hour)
		secondArticle = in.ArticleID + 1000
	case "future": // 发生时间晚于接收时间
		eid = newID()
		occur = now.Add(2 * time.Hour)
	case "late_cross_day": // 跨日迟到（≤24h，应归发生日=昨日）
		eid = newID()
		b := now.In(model.Beijing)
		occur = time.Date(b.Year(), b.Month(), b.Day()-1, 23, 59, 0, 0, model.Beijing)
	case "cross_day_normal": // 昨日 23:30 事件（对照：同样归昨日）
		eid = newID()
		b := now.In(model.Beijing)
		occur = time.Date(b.Year(), b.Month(), b.Day()-1, 23, 30, 0, 0, model.Beijing)
	case "too_late": // 超过 24h 窗口
		eid = newID()
		occur = now.Add(-25 * time.Hour)
	case "custom": // 自定义发生时间（配合教学时钟演示活动结束后窗口内事件）
		eid = newID()
		if in.OccurAt == "" {
			return nil, fmt.Errorf("custom 场景需提供 occur_at")
		}
		t, err := time.Parse(time.RFC3339, in.OccurAt)
		if err != nil {
			return nil, fmt.Errorf("occur_at 格式不合法: %w", err)
		}
		occur = t
	default:
		return nil, fmt.Errorf("未知场景: %s", in.Scenario)
	}

	result := map[string]any{
		"scenario":   in.Scenario,
		"event_id":   eid,
		"occur_at":   occur.Format(time.RFC3339),
		"deliveries": []json.RawMessage{},
	}
	deliveries := result["deliveries"].([]json.RawMessage)
	deliveries = append(deliveries, s.deliver(ctx, eid, in.UserID, in.ArticleID, occur))
	if deliverTwice {
		deliveries = append(deliveries, s.deliver(ctx, eid, in.UserID, in.ArticleID, occur))
	}
	if secondArticle > 0 {
		deliveries = append(deliveries, s.deliver(ctx, eid, in.UserID, secondArticle, occur))
	}
	result["deliveries"] = deliveries
	return result, nil
}

// mustJSONError 构造含错误信息的 JSON 响应体（正确转义，避免错误文本破坏 JSON 结构）。
func mustJSONError(msg string) []byte {
	b, err := json.Marshal(map[string]string{"error": msg})
	if err != nil {
		return []byte(`{"error":"deliver failed"}`)
	}
	return b
}

// TrustedRecord 可信阅读记录。
type TrustedRecord struct {
	EventID   string    `json:"event_id"`
	UserID    int64     `json:"user_id"`
	ArticleID int64     `json:"article_id"`
	OccurAt   time.Time `json:"occur_at"`
	BizDate   string    `json:"biz_date"`
}

// TrustedRecords 按用户与业务日返回可信阅读记录（客服核查的可信查询）。
func (s *Service) TrustedRecords(ctx context.Context, userID int64, bizDate string) ([]TrustedRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, user_id, article_id, occur_at, biz_date
		FROM trusted_record WHERE user_id = ? AND biz_date = ? ORDER BY occur_at`,
		userID, bizDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrustedRecord
	for rows.Next() {
		var r TrustedRecord
		var occur, bizDate time.Time
		if err := rows.Scan(&r.EventID, &r.UserID, &r.ArticleID, &occur, &bizDate); err != nil {
			return nil, err
		}
		r.OccurAt = occur
		r.BizDate = bizDate.Format("2006-01-02")
		list = append(list, r)
	}
	if list == nil {
		list = []TrustedRecord{}
	}
	return list, rows.Err()
}
