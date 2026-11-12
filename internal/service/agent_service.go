package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/store"
)

// 客服端错误。
var (
	ErrAppealNotFound  = errors.New("申诉不存在")
	ErrAppealProcessed = errors.New("申诉已处理，不可重复操作")
)

// AgentService 客服端服务：申诉核查、驳回、补入。
type AgentService struct {
	db     *sql.DB
	clk    clock.Clock
	events *EventService
	reader ReadingServiceClient
}

func NewAgentService(db *sql.DB, clk clock.Clock, events *EventService, reader ReadingServiceClient) *AgentService {
	return &AgentService{db: db, clk: clk, events: events, reader: reader}
}

// AppealItem 申诉列表项。
type AppealItem struct {
	ID      int64  `json:"id"`
	UserID  int64  `json:"user_id"`
	BizDate string `json:"biz_date"`
	Content string `json:"content"`
	Status  int8   `json:"status"`
}

// ListAppeals 列出申诉；status 参数为 nil 表示全部。
func (s *AgentService) ListAppeals(ctx context.Context, status *int8) ([]AppealItem, error) {
	appeals, err := store.ListAppeals(ctx, s.db, status)
	if err != nil {
		return nil, err
	}
	items := []AppealItem{}
	for _, a := range appeals {
		items = append(items, AppealItem{ID: a.ID, UserID: a.UserID, BizDate: a.BizDate, Content: a.Content, Status: a.Status})
	}
	return items, nil
}

// AppealDetail 申诉核查详情：申诉 + 已接收事件 + 本地进度/发奖 + 阅读服务可信记录。
type AppealDetail struct {
	Appeal         store.Appeal     `json:"appeal"`
	AcceptedEvents []store.EventRow `json:"accepted_events"`
	Progress       *ProgressView    `json:"progress"`
	RewardID       int64            `json:"reward_id,omitempty"`
	RewardPoints   int              `json:"reward_points,omitempty"`
	TrustedRecords []ReadingRecord  `json:"trusted_records"`
	TrustedNote    string           `json:"trusted_note,omitempty"`
}

// GetAppealDetail 汇总核查证据（题面规则 6：结合已接收事件与阅读服务可信查询）。
func (s *AgentService) GetAppealDetail(ctx context.Context, appealID int64) (*AppealDetail, error) {
	appeal, err := store.GetAppealByID(ctx, s.db, appealID)
	if err != nil {
		return nil, err
	}
	if appeal == nil {
		return nil, ErrAppealNotFound
	}
	detail := &AppealDetail{Appeal: *appeal}

	events, err := store.ListEventsByUserDate(ctx, s.db, appeal.UserID, appeal.BizDate)
	if err != nil {
		return nil, err
	}
	detail.AcceptedEvents = events

	progress, err := s.progressOf(ctx, appeal.UserID, appeal.BizDate)
	if err != nil {
		return nil, err
	}
	detail.Progress = progress

	if rewardID, points, rewarded, err := store.GetRewardDetail(ctx, s.db, appeal.UserID, appeal.ActivityID, appeal.BizDate); err != nil {
		return nil, err
	} else if rewarded {
		detail.RewardID = rewardID
		detail.RewardPoints = points
	}

	if s.reader != nil {
		records, err := s.reader.QueryRecords(ctx, appeal.UserID, appeal.BizDate)
		if err != nil {
			detail.TrustedNote = "阅读服务可信查询失败: " + err.Error()
		} else {
			detail.TrustedRecords = records
		}
	} else {
		detail.TrustedNote = "模拟阅读服务未配置（READING_SVC_URL）"
	}
	return detail, nil
}

// RejectAppeal 驳回申诉：必须写明依据（题面规则 6），留操作者与时间。
func (s *AgentService) RejectAppeal(ctx context.Context, appealID, operatorID int64, conclusion string) error {
	conclusion = strings.TrimSpace(conclusion)
	if conclusion == "" {
		return errors.New("驳回必须写明依据")
	}
	appeal, err := store.GetAppealByID(ctx, s.db, appealID)
	if err != nil {
		return err
	}
	if appeal == nil {
		return ErrAppealNotFound
	}
	ok, err := store.ResolveAppeal(ctx, s.db, appealID, store.AppealRejected,
		"驳回："+conclusion, operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAppealProcessed
	}
	return nil
}

// CompensateResult 补入处理结果。
type CompensateResult struct {
	HandleResult *HandleResult `json:"handle_result"`
	Resolved     bool          `json:"resolved"`
	RewardID     int64         `json:"reward_id,omitempty"`
	TxnNote      string        `json:"txn_note,omitempty"`
}

// CompensateAppeal 客服补入缺失有效记录：复用统一事件接入路径（source=客服），
// 同样受活动期、迟到窗口与去重约束；发奖唯一键保证与自动入账并发只发一次。
func (s *AgentService) CompensateAppeal(ctx context.Context, appealID, operatorID int64,
	eventID string, articleID int64, occurAt time.Time) (*CompensateResult, error) {
	appeal, err := store.GetAppealByID(ctx, s.db, appealID)
	if err != nil {
		return nil, err
	}
	if appeal == nil {
		return nil, ErrAppealNotFound
	}
	if appeal.Status != store.AppealOpen {
		return nil, ErrAppealProcessed
	}
	if strings.TrimSpace(eventID) == "" || articleID <= 0 {
		return nil, errors.New("补入参数不合法：event_id 与 article_id 必填")
	}

	res, err := s.events.HandleEvent(ctx, EventInput{
		EventID:   eventID,
		UserID:    appeal.UserID,
		ArticleID: articleID,
		OccurAt:   occurAt,
	}, model.SourceAgent) // source=客服补入，复用统一校验与发奖规则
	if err != nil {
		return nil, err
	}
	out := &CompensateResult{HandleResult: res}

	// 仅当补入直接触发发奖时自动结案；其余情况由客服依据结果决定是否驳回
	if res.Status == ResultRewarded {
		rewardID, points, rewarded, err := store.GetRewardDetail(ctx, s.db, appeal.UserID, appeal.ActivityID, appeal.BizDate)
		if err != nil {
			return nil, err
		}
		if rewarded {
			out.Resolved = true
			out.RewardID = rewardID
			out.TxnNote = fmt.Sprintf("已补入记录 %s 并发奖 %d 分（reward #%d），流水与余额同事务",
				eventID, points, rewardID)
			ok, err := store.ResolveAppeal(ctx, s.db, appealID, store.AppealCompensed,
				fmt.Sprintf("补入有效记录 %s，触发统一发奖规则：reward #%d（%d 分）", eventID, rewardID, points),
				operatorID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrAppealProcessed
			}
		}
	}
	return out, nil
}

// progressOf 复用用户端进度逻辑构造核查视图。
func (s *AgentService) progressOf(ctx context.Context, userID int64, bizDate string) (*ProgressView, error) {
	articles, err := store.ListProgressArticles(ctx, s.db, userID, bizDate)
	if err != nil {
		return nil, err
	}
	if articles == nil {
		articles = []int64{}
	}
	view := &ProgressView{BizDate: bizDate, Articles: articles, DistinctCount: len(articles)}
	activity, err := store.LatestPublishedActivity(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if activity != nil {
		view.Threshold = activity.DailyThreshold
		view.Reached = len(articles) >= activity.DailyThreshold
		if points, rewarded, err := store.GetReward(ctx, s.db, userID, activity.ID, bizDate); err != nil {
			return nil, err
		} else if rewarded {
			view.Rewarded = true
			view.RewardPoints = points
		}
	}
	return view, nil
}
