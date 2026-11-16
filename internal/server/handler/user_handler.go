package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Activity 已发布活动列表（多活动并存）。GET /api/v1/users/activity
func (h *UserHandler) Activity(c *gin.Context) {
	views, err := h.svc.GetActivities(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"activities": views})
}

// Progress 每日进度。GET /api/v1/users/progress?date=yyyy-mm-dd
func (h *UserHandler) Progress(c *gin.Context) {
	userID := c.GetInt64("uid")
	view, err := h.svc.GetProgress(c.Request.Context(), userID, c.Query("date"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// Points 积分余额与流水。GET /api/v1/users/points
func (h *UserHandler) Points(c *gin.Context) {
	userID := c.GetInt64("uid")
	view, err := h.svc.GetPoints(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}

// SubmitAppeal 提交申诉。POST /api/v1/users/appeals {date, content}
func (h *UserHandler) SubmitAppeal(c *gin.Context) {
	userID := c.GetInt64("uid")
	var req struct {
		Date    string `json:"date"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	view, err := h.svc.SubmitAppeal(c.Request.Context(), userID, req.Date, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrAppealExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, view)
}
