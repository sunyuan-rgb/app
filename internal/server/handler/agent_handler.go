package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/service"
)

type AgentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

// ListAppeals 申诉列表。GET /api/v1/agent/appeals?status=0
func (h *AgentHandler) ListAppeals(c *gin.Context) {
	var status *int8
	if raw := c.Query("status"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 8)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status 参数不合法"})
			return
		}
		sv := int8(v)
		status = &sv
	}
	items, err := h.svc.ListAppeals(c.Request.Context(), status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"appeals": items})
}

// GetAppealDetail 申诉核查详情。GET /api/v1/agent/appeals/:id
func (h *AgentHandler) GetAppealDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申诉 ID 不合法"})
		return
	}
	detail, err := h.svc.GetAppealDetail(c.Request.Context(), id)
	if err != nil {
		mapAppealErr(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// RejectAppeal 驳回。POST /api/v1/agent/appeals/:id/reject {conclusion}
func (h *AgentHandler) RejectAppeal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申诉 ID 不合法"})
		return
	}
	var req struct {
		Conclusion string `json:"conclusion" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	if err := h.svc.RejectAppeal(c.Request.Context(), id, c.GetInt64("uid"), req.Conclusion); err != nil {
		mapAppealErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "rejected"})
}

// CompensateAppeal 补入记录。POST /api/v1/agent/appeals/:id/compensate
func (h *AgentHandler) CompensateAppeal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "申诉 ID 不合法"})
		return
	}
	var req struct {
		EventID   string    `json:"event_id" binding:"required"`
		ArticleID int64     `json:"article_id" binding:"required"`
		OccurAt   time.Time `json:"occur_at" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	res, err := h.svc.CompensateAppeal(c.Request.Context(), id, c.GetInt64("uid"),
		req.EventID, req.ArticleID, req.OccurAt)
	if err != nil {
		mapAppealErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func mapAppealErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAppealNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrAppealProcessed):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
