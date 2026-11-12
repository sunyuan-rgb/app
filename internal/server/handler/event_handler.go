// Package handler 提供 HTTP 接口层，只做参数绑定与结果映射，不含业务规则。
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/model"
	"reading-activity/internal/service"
)

type EventHandler struct {
	svc *service.EventService
}

func NewEventHandler(svc *service.EventService) *EventHandler {
	return &EventHandler{svc: svc}
}

// Create 接收模拟阅读服务投递的阅读事件。
// POST /api/v1/events
func (h *EventHandler) Create(c *gin.Context) {
	var req struct {
		EventID   string    `json:"event_id" binding:"required"`
		UserID    int64     `json:"user_id" binding:"required"`
		ArticleID int64     `json:"article_id" binding:"required"`
		OccurAt   time.Time `json:"occur_at" binding:"required" format:"RFC3339"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}

	res, err := h.svc.HandleEvent(c.Request.Context(), service.EventInput{
		EventID:   req.EventID,
		UserID:    req.UserID,
		ArticleID: req.ArticleID,
		OccurAt:   req.OccurAt,
	}, model.SourceAuto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "处理失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}
