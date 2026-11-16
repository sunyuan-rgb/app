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

// ErrAppealExists 当天已有未结申诉。
var ErrAppealExists = errors.New("当天已有未结申诉")

// UserService 用户端服务：活动视图、每日进度、积分、申诉。
type UserService struct {
	db  *sql.DB
	clk clock.Clock
}

func NewUserService(db *sql.DB, clk clock.Clock) *UserService {
	return &UserService{db: db, clk: clk}
}

// ActivityView 活动信息（用户视角，仅展示已发布活动）。
type ActivityView struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	DailyThreshold int    `json:"daily_threshold"`
	RewardPoints   int    `json:"reward_points"`
}

// GetActivities 获取全部已发布活动（多活动并存，按开始时间升序）。
func (s *UserService) GetActivities(ctx context.Context) ([]*ActivityView, error) {
	acts, err := store.ListPublishedActivities(ctx, s.db)
	if err != nil {
		return nil, err
	}
	out := make([]*ActivityView, 0, len(acts))
	for _, a := range acts {
		out = append(out, &ActivityView{
			ID:             a.ID,
			Name:           a.Name,
			StartAt:        a.StartAt.In(model.Beijing).Format(time.RFC3339),
			EndAt:          a.EndAt.In(model.Beijing).Format(time.RFC3339),
			DailyThreshold: a.DailyThreshold,
			RewardPoints:   a.RewardPoints,
		})
	}
	return out, nil
}

// activityForBizDate 选取业务日落在其期间内的已发布活动
// （发布校验保证时间不重叠；同业务日命中多个时取开始最晚的一个）。
func activityForBizDate(acts []*model.Activity, bizDate string) *model.Activity {
	var best *model.Activity
	for _, a := range acts {
		if bizDate < model.BizDate(a.StartAt) || bizDate > model.BizDate(a.EndAt) {
			continue
		}
		if best == nil || a.StartAt.After(best.StartAt) {
			best = a
		}
	}
	return best
}

// ProgressView 每日进度视图。
type ProgressView struct {
	BizDate       string  `json:"biz_date"`
	Articles      []int64 `json:"articles"`
	DistinctCount int     `json:"distinct_count"`
	Threshold     int     `json:"threshold"`
	Reached       bool    `json:"reached"`
	Rewarded      bool    `json:"rewarded"`
	RewardPoints  int     `json:"reward_points,omitempty"`
}

// GetProgress 查询用户某业务日进度；dateStr 为空取教学时钟当天（北京时间）。
func (s *UserService) GetProgress(ctx context.Context, userID int64, dateStr string) (*ProgressView, error) {
	bizDate, err := normalizeBizDate(dateStr, s.clk.Now())
	if err != nil {
		return nil, err
	}
	articles, err := store.ListProgressArticles(ctx, s.db, userID, bizDate)
	if err != nil {
		return nil, err
	}
	if articles == nil {
		articles = []int64{}
	}
	view := &ProgressView{
		BizDate:       bizDate,
		Articles:      articles,
		DistinctCount: len(articles),
	}
	if acts, err := store.ListPublishedActivities(ctx, s.db); err != nil {
		return nil, err
	} else if a := activityForBizDate(acts, bizDate); a != nil {
		view.Threshold = a.DailyThreshold
		view.Reached = len(articles) >= a.DailyThreshold
		if points, rewarded, err := store.GetReward(ctx, s.db, userID, a.ID, bizDate); err != nil {
			return nil, err
		} else if rewarded {
			view.Rewarded = true
			view.RewardPoints = points
		}
	}
	return view, nil
}

// PointsView 积分余额与流水。
type PointsView struct {
	Balance int              `json:"balance"`
	Txns    []store.PointTxn `json:"txns"`
}

// GetPoints 查询积分余额与最近流水。
func (s *UserService) GetPoints(ctx context.Context, userID int64) (*PointsView, error) {
	balance, err := store.GetBalance(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	txns, err := store.ListPointTxn(ctx, s.db, userID, 50)
	if err != nil {
		return nil, err
	}
	return &PointsView{Balance: balance, Txns: txns}, nil
}

// AppealView 申诉提交结果。
type AppealView struct {
	ID      int64  `json:"id"`
	BizDate string `json:"biz_date"`
	Status  string `json:"status"`
}

// SubmitAppeal 提交申诉：内容必填、业务日须在活动期间、同日仅一条未结（数据库唯一键兜底）。
func (s *UserService) SubmitAppeal(ctx context.Context, userID int64, dateStr, content string) (*AppealView, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("申诉内容不能为空")
	}
	if len(content) > 1024 {
		return nil, errors.New("申诉内容过长（上限 1024 字）")
	}
	bizDate, err := normalizeBizDate(dateStr, s.clk.Now())
	if err != nil {
		return nil, err
	}
	acts, err := store.ListPublishedActivities(ctx, s.db)
	if err != nil {
		return nil, err
	}
	activity := activityForBizDate(acts, bizDate)
	if activity == nil {
		return nil, errors.New("该业务日不在任何已发布活动期间")
	}

	ok, id, err := store.InsertAppeal(ctx, s.db, userID, activity.ID, bizDate, content)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrAppealExists
	}
	return &AppealView{ID: id, BizDate: bizDate, Status: "待处理"}, nil
}

// normalizeBizDate 校验/归一业务日参数（yyyy-mm-dd），为空取当前北京业务日。
func normalizeBizDate(dateStr string, now time.Time) (string, error) {
	if strings.TrimSpace(dateStr) == "" {
		return model.BizDate(now), nil
	}
	t, err := time.ParseInLocation("2006-01-02", dateStr, model.Beijing)
	if err != nil {
		return "", errors.New("日期格式不合法，应为 yyyy-mm-dd")
	}
	return t.Format("2006-01-02"), nil
}
