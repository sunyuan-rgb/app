package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/store"
)

// 运营端错误。
var (
	ErrActivityExists = errors.New("已存在已发布活动，不可重复发布")
	ErrActivityInput  = errors.New("活动参数不合法")
)

// OpsService 运营端服务：发布活动与结算核查汇总。
type OpsService struct {
	db  *sql.DB
	clk clock.Clock
}

func NewOpsService(db *sql.DB, clk clock.Clock) *OpsService {
	return &OpsService{db: db, clk: clk}
}

// PublishInput 发布活动输入（题面：发布后规则固定，不可修改）。
type PublishInput struct {
	Name           string    `json:"name"`
	StartAt        time.Time `json:"start_at"`
	EndAt          time.Time `json:"end_at"`
	DailyThreshold int       `json:"daily_threshold"`
	RewardPoints   int       `json:"reward_points"`
}

// PublishActivity 创建并发布活动：校验参数与时间，之后规则固定。
func (s *OpsService) PublishActivity(ctx context.Context, in PublishInput) (*ActivityView, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.DailyThreshold <= 0 || in.RewardPoints <= 0 {
		return nil, ErrActivityInput
	}
	if !in.StartAt.Before(in.EndAt) {
		return nil, errors.New("活动开始时间必须早于结束时间")
	}
	if in.EndAt.Before(s.clk.Now()) {
		return nil, errors.New("活动结束时间不能早于当前时间")
	}
	row := &store.ActivityRow{
		Name:           in.Name,
		StartAt:        in.StartAt,
		EndAt:          in.EndAt,
		DailyThreshold: in.DailyThreshold,
		RewardPoints:   in.RewardPoints,
	}
	ok, err := store.CreateActivity(ctx, s.db, row)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrActivityExists
	}
	return &ActivityView{
		ID:             row.ID,
		Name:           row.Name,
		StartAt:        row.StartAt.In(model.Beijing).Format(time.RFC3339),
		EndAt:          row.EndAt.In(model.Beijing).Format(time.RFC3339),
		DailyThreshold: row.DailyThreshold,
		RewardPoints:   row.RewardPoints,
	}, nil
}

// SummaryView 结算核查汇总（题面规则 8：窗口关闭后核查）。
type SummaryView struct {
	Activity        *ActivityView      `json:"activity"`
	WindowClosed    bool               `json:"window_closed"` // 结束时间 + 24h 迟到窗口是否已过
	Days            []store.DaySummary `json:"days"`
	Inconsistencies []Inconsistency    `json:"inconsistencies"`
	Rewards         []store.RewardRow  `json:"rewards"`
}

// Inconsistency 不一致明细（达标未发奖 / 发奖未达标）。
type Inconsistency struct {
	BizDate string             `json:"biz_date"`
	Type    string             `json:"type"` // reached_not_rewarded / rewarded_not_reached
	Users   []store.UserDayRow `json:"users"`
}

// GetSummary 按活动逐业务日汇总，并列出不一致项的可追溯明细。
func (s *OpsService) GetSummary(ctx context.Context) (*SummaryView, error) {
	activity, err := store.LatestPublishedActivity(ctx, s.db)
	if err != nil {
		return nil, err
	}
	if activity == nil {
		return nil, errors.New("无已发布活动")
	}
	view := &SummaryView{
		WindowClosed:    s.clk.Now().After(activity.EndAt.Add(LateWindow)),
		Inconsistencies: []Inconsistency{},
	}

	start := activity.StartAt.In(model.Beijing)
	end := activity.EndAt.In(model.Beijing)
	// 业务日 = 与活动区间有实际交集的北京日期（结束时刻恰为整点 0 时不计入该日）
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		bizDate := d.Format("2006-01-02")
		sum, err := store.SummarizeDay(ctx, s.db, activity.ID, bizDate, activity.DailyThreshold)
		if err != nil {
			return nil, err
		}
		view.Days = append(view.Days, *sum)

		reached, err := store.ListReachedNotRewarded(ctx, s.db, activity.ID, bizDate, activity.DailyThreshold)
		if err != nil {
			return nil, err
		}
		if len(reached) > 0 {
			view.Inconsistencies = append(view.Inconsistencies,
				Inconsistency{BizDate: bizDate, Type: "reached_not_rewarded", Users: reached})
		}
		rewarded, err := store.ListRewardedNotReached(ctx, s.db, activity.ID, bizDate, activity.DailyThreshold)
		if err != nil {
			return nil, err
		}
		if len(rewarded) > 0 {
			view.Inconsistencies = append(view.Inconsistencies,
				Inconsistency{BizDate: bizDate, Type: "rewarded_not_reached", Users: rewarded})
		}

		rewards, err := store.ListRewardsByDate(ctx, s.db, activity.ID, bizDate)
		if err != nil {
			return nil, err
		}
		view.Rewards = append(view.Rewards, rewards...)
	}
	if view.Days == nil {
		view.Days = []store.DaySummary{}
	}
	if view.Inconsistencies == nil {
		view.Inconsistencies = []Inconsistency{}
	}
	if view.Rewards == nil {
		view.Rewards = []store.RewardRow{}
	}

	view.Activity = &ActivityView{
		ID:             activity.ID,
		Name:           activity.Name,
		StartAt:        activity.StartAt.In(model.Beijing).Format(time.RFC3339),
		EndAt:          activity.EndAt.In(model.Beijing).Format(time.RFC3339),
		DailyThreshold: activity.DailyThreshold,
		RewardPoints:   activity.RewardPoints,
	}
	return view, nil
}
