package readingsvc

import (
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/readingsvc/web"
)

// RegisterRoutes 注册页面与 API 路由（供 cmd/readingsvc 与集成测试复用）。
func (s *Service) RegisterRoutes(r *gin.Engine, clk clock.Clock) {
	r.SetHTMLTemplate(template.Must(template.ParseFS(web.FS, "*.html")))
	r.GET("/", func(c *gin.Context) { c.HTML(http.StatusOK, "read.html", nil) })
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "now": clk.Now().Format(time.RFC3339)})
	})

	// 用户在阅读服务读文章：POST /read {user_id, article_id, drop}
	r.POST("/read", func(c *gin.Context) {
		var req struct {
			UserID    int64 `json:"user_id" binding:"required"`
			ArticleID int64 `json:"article_id" binding:"required"`
			Drop      bool  `json:"drop"` // true=不投递事件（模拟链路丢失）
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		out, err := s.Read(c.Request.Context(), req.UserID, req.ArticleID, req.Drop)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	})

	// 异常场景触发：POST /simulate {scenario, user_id, article_id, event_id?, occur_at?}
	r.POST("/simulate", func(c *gin.Context) {
		var req SimulateInput
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		out, err := s.Simulate(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	})

	// 可信查询（客服申诉核查证据源）：GET /trusted/records?user_id=&biz_date=
	r.GET("/trusted/records", func(c *gin.Context) {
		uid, err := strconv.ParseInt(c.Query("user_id"), 10, 64)
		if err != nil || uid <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id 不合法"})
			return
		}
		bizDate := c.Query("biz_date")
		if bizDate == "" {
			bizDate = model.BizDate(clk.Now())
		}
		records, err := s.TrustedRecords(c.Request.Context(), uid, bizDate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"records": records})
	})
}
