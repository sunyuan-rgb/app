// Package model 定义核心业务实体。
package model

import "time"

// Beijing 业务时区：北京时间。
// 业务日归属一律按事件发生时间换算到该时区。
var Beijing = time.FixedZone("UTC+8", 8*3600)

// BizDate 返回 t 对应的北京业务日（yyyy-mm-dd）。
func BizDate(t time.Time) string {
	return t.In(Beijing).Format("2006-01-02")
}

type ActivityStatus int16

const (
	ActivityDraft     ActivityStatus = 0
	ActivityPublished ActivityStatus = 1
	ActivityFinished  ActivityStatus = 2
)

type EventSource int8

const (
	SourceAuto  EventSource = 1 // 阅读服务自动投递
	SourceAgent EventSource = 2 // 客服补入
)

type EventStatus int8

const (
	EventAccepted EventStatus = 1
	EventRejected EventStatus = 2
)

type Activity struct {
	ID             int64
	Name           string
	StartAt        time.Time
	EndAt          time.Time
	DailyThreshold int
	RewardPoints   int
	Status         ActivityStatus
}

type ReadingEvent struct {
	ID           int64
	EventID      string
	ActivityID   int64
	UserID       int64
	ArticleID    int64
	OccurAt      time.Time
	ReceivedAt   time.Time
	BizDate      string
	Source       EventSource
	Status       EventStatus
	Counted      bool
	RejectReason string
}
