// Package service 实现核心业务逻辑。
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/store"
)

// 迟到窗口：事件最多允许迟到 24 小时（题面规则 3）。
const LateWindow = 24 * time.Hour

// ResultStatus 业务处理结果。
type ResultStatus string

const (
	ResultAccepted   ResultStatus = "accepted"    // 事件受理
	ResultCounted    ResultStatus = ""            // （内部）
	ResultDupEvent   ResultStatus = "duplicated"  // 同事件重复投递（幂等成功）
	ResultDupArticle ResultStatus = "dup_article" // 该文章当日已计
	ResultRejected   ResultStatus = "rejected"    // 校验拒绝
	ResultConflict   ResultStatus = "conflict"    // 同编号换内容
	ResultRewarded   ResultStatus = "rewarded"    // 受理并触发发奖
)

// EventInput 事件接入输入。
type EventInput struct {
	EventID   string
	UserID    int64
	ArticleID int64
	OccurAt   time.Time
}

// HandleResult 事件处理结果（可作为查询结果供核查）。
type HandleResult struct {
	Status       ResultStatus `json:"status"`
	BizDate      string       `json:"biz_date,omitempty"`
	Counted      bool         `json:"counted"`
	RewardPoints int          `json:"reward_points,omitempty"`
	Reason       string       `json:"reason,omitempty"`
}

// EventService 阅读事件接入服务。
type EventService struct {
	db  *sql.DB
	clk clock.Clock
}

func NewEventService(db *sql.DB, clk clock.Clock) *EventService {
	return &EventService{db: db, clk: clk}
}

// HandleEvent 处理一条阅读事件：校验 → 去重 → 计进度 → 达标发奖。
// 校验拒绝的事件同样持久化留痕（题面要求可查询处理结果）。
func (s *EventService) HandleEvent(ctx context.Context, in EventInput, source model.EventSource) (*HandleResult, error) {
	now := s.clk.Now()

	// --- 事务外可判定的校验 ---
	if in.OccurAt.After(now) {
		return s.reject(ctx, in, now, source, "发生时间晚于接收时间")
	}
	if now.Sub(in.OccurAt) > LateWindow {
		return s.reject(ctx, in, now, source, "超过 24 小时迟到窗口")
	}

	activity, err := store.LatestPublishedActivity(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if activity == nil {
		return s.reject(ctx, in, now, source, "无进行中的已发布活动")
	}
	if in.OccurAt.Before(activity.StartAt) || in.OccurAt.After(activity.EndAt) {
		return s.reject(ctx, in, now, source, "发生时间不在活动期间")
	}

	bizDate := model.BizDate(in.OccurAt)

	// --- 事务：事件留痕 + 进度去重 + 达标发奖 ---
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ev := &model.ReadingEvent{
		EventID:    in.EventID,
		ActivityID: activity.ID,
		UserID:     in.UserID,
		ArticleID:  in.ArticleID,
		OccurAt:    in.OccurAt,
		ReceivedAt: now,
		BizDate:    bizDate,
		Source:     source,
		Status:     model.EventAccepted,
	}
	inserted, err := store.InsertEvent(ctx, tx, ev)
	if err != nil {
		return nil, err
	}
	if !inserted {
		// event_id 已存在：内容相同 → 幂等返回；内容不同 → 冲突留痕
		existing, err := store.GetEventByID(ctx, tx, in.EventID)
		if err != nil {
			return nil, err
		}
		if existing != nil && sameContent(existing, in) {
			return &HandleResult{
				Status:  ResultDupEvent,
				BizDate: existing.BizDate,
				Counted: existing.Counted,
				Reason:  "重复事件编号，按首次处理结果返回",
			}, tx.Commit()
		}
		payload, _ := json.Marshal(in)
		if err := store.InsertEventConflict(ctx, tx, in.EventID, string(payload), now); err != nil {
			return nil, err
		}
		return &HandleResult{
			Status: ResultConflict,
			Reason: "事件编号已存在但内容不一致，已记录冲突",
		}, tx.Commit()
	}

	// 文章级去重：同用户同业务日同文章只计一次
	progressInserted, err := store.InsertProgress(ctx, tx, in.UserID, bizDate, in.ArticleID, in.EventID)
	if err != nil {
		return nil, err
	}
	if progressInserted {
		if _, err := tx.ExecContext(ctx,
			`UPDATE reading_event SET counted = 1 WHERE event_id = ?`, in.EventID); err != nil {
			return nil, err
		}
	}

	result := &HandleResult{Status: ResultAccepted, BizDate: bizDate, Counted: progressInserted}
	if !progressInserted {
		result.Status = ResultDupArticle
		result.Reason = "该文章当日已计入，不重复计数"
	}

	// 达标判断：本次新增使不同文章数达到门槛及以上时都尝试发奖。
	// 用 >= 而非 ==：若历史某日进度已达标但发奖失败（或客服批量补入造成跳档），
	// 后续任何一次新增都应补触发发奖；是否已发过由 reward 唯一键幂等兜底。
	if progressInserted {
		count, err := store.CountArticles(ctx, tx, in.UserID, bizDate)
		if err != nil {
			return nil, err
		}
		if count >= activity.DailyThreshold {
			ok, err := store.InsertReward(ctx, tx, in.UserID, activity.ID, bizDate, activity.RewardPoints)
			if err != nil {
				return nil, err
			}
			if ok {
				// 发奖记录 id 与流水、余额同事务写入，保证强一致
				var rewardID int64
				if err := tx.QueryRowContext(ctx,
					`SELECT id FROM reward WHERE user_id = ? AND activity_id = ? AND biz_date = ?`,
					in.UserID, activity.ID, bizDate).Scan(&rewardID); err != nil {
					return nil, err
				}
				if err := store.InsertPointTxn(ctx, tx, in.UserID, activity.RewardPoints, "reward", rewardID); err != nil {
					return nil, err
				}
				if err := store.AddBalance(ctx, tx, in.UserID, activity.RewardPoints); err != nil {
					return nil, err
				}
				result.Status = ResultRewarded
				result.RewardPoints = activity.RewardPoints
			}
		}
	}

	return result, tx.Commit()
}

// reject 持久化被拒绝的事件（留痕可查），并返回拒绝结果。
func (s *EventService) reject(ctx context.Context, in EventInput, now time.Time,
	source model.EventSource, reason string) (*HandleResult, error) {
	var activityID int64
	activity, err := store.LatestPublishedActivity(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if activity != nil {
		activityID = activity.ID
	}

	ev := &model.ReadingEvent{
		EventID:      in.EventID,
		ActivityID:   activityID,
		UserID:       in.UserID,
		ArticleID:    in.ArticleID,
		OccurAt:      in.OccurAt,
		ReceivedAt:   now,
		BizDate:      model.BizDate(in.OccurAt),
		Source:       source,
		Status:       model.EventRejected,
		RejectReason: reason,
	}
	if _, err := store.InsertEvent(ctx, s.db, ev); err != nil {
		return nil, err
	}
	return &HandleResult{Status: ResultRejected, Reason: fmt.Sprintf("事件已拒绝并留痕：%s", reason)}, nil
}

func sameContent(e *model.ReadingEvent, in EventInput) bool {
	return e.UserID == in.UserID && e.ArticleID == in.ArticleID && e.OccurAt.Equal(in.OccurAt)
}
