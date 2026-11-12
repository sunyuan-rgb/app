package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ReadingRecord 模拟阅读服务的可信阅读记录。
type ReadingRecord struct {
	EventID   string    `json:"event_id"`
	UserID    int64     `json:"user_id"`
	ArticleID int64     `json:"article_id"`
	OccurAt   time.Time `json:"occur_at"`
}

// ReadingServiceClient 模拟阅读服务的可信查询接口（题面规则 6：客服核查证据来源之一）。
type ReadingServiceClient interface {
	// QueryRecords 按用户与业务日查询可信阅读记录。
	QueryRecords(ctx context.Context, userID int64, bizDate string) ([]ReadingRecord, error)
}

// HTTPReadingService 基于HTTP 的模拟阅读服务客户端。
type HTTPReadingService struct {
	BaseURL string
	HTTP    *http.Client
}

func NewHTTPReadingService(baseURL string) *HTTPReadingService {
	return &HTTPReadingService{BaseURL: baseURL, HTTP: &http.Client{Timeout: 3 * time.Second}}
}

// QueryRecords GET {base}/trusted/records?user_id=&biz_date=
func (h *HTTPReadingService) QueryRecords(ctx context.Context, userID int64, bizDate string) ([]ReadingRecord, error) {
	url := fmt.Sprintf("%s/trusted/records?user_id=%d&biz_date=%s", h.BaseURL, userID, bizDate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("阅读服务返回 %d", resp.StatusCode)
	}
	var out struct {
		Records []ReadingRecord `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Records, nil
}
