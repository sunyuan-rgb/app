package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/service"
)

type OpsHandler struct {
	svc *service.OpsService
}

func NewOpsHandler(svc *service.OpsService) *OpsHandler {
	return &OpsHandler{svc: svc}
}

// PublishActivity 发布活动。POST /api/v1/ops/activity
func (h *OpsHandler) PublishActivity(c *gin.Context) {
	var req service.PublishInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	view, err := h.svc.PublishActivity(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrActivityExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"activity": view})
}

// GetSummary 结算核查汇总。GET /api/v1/ops/summary
func (h *OpsHandler) GetSummary(c *gin.Context) {
	view, err := h.svc.GetSummary(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}
