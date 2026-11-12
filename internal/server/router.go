// Package server 组装 Gin 路由与中间件。
package server

import (
	"database/sql"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/server/handler"
	"reading-activity/internal/server/web"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
)

// Deps 路由依赖。
type Deps struct {
	DB          *sql.DB
	Clk         clock.Clock
	EventsToken string                       // 模拟阅读服务接入令牌（假设 A-01）
	Reader      service.ReadingServiceClient // 模拟阅读服务可信查询客户端，可为 nil
}

// New 创建并配置 Gin 引擎。
func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 内嵌页面
	r.SetHTMLTemplate(template.Must(template.ParseFS(web.FS, "*.html")))
	r.GET("/", func(c *gin.Context) { c.HTML(http.StatusOK, "index.html", nil) })
	r.GET("/user", func(c *gin.Context) { c.HTML(http.StatusOK, "user.html", nil) })
	r.GET("/agent", func(c *gin.Context) { c.HTML(http.StatusOK, "agent.html", nil) })
	r.GET("/ops", func(c *gin.Context) { c.HTML(http.StatusOK, "ops.html", nil) })

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	eventSvc := service.NewEventService(d.DB, d.Clk)
	eventHandler := handler.NewEventHandler(eventSvc)

	// 阅读服务事件接入（须鉴权）
	events := r.Group("/api/v1/events", requireToken(d.EventsToken))
	{
		events.POST("", eventHandler.Create)
	}

	// 用户端（令牌解析身份，用户编号不可由请求参数指定）
	userSvc := service.NewUserService(d.DB, d.Clk)
	userHandler := handler.NewUserHandler(userSvc)
	user := r.Group("/api/v1/users", requireAccount(d.DB, "user"))
	{
		user.GET("/activity", userHandler.Activity)
		user.GET("/progress", userHandler.Progress)
		user.GET("/points", userHandler.Points)
		user.POST("/appeals", userHandler.SubmitAppeal)
	}

	// 客服端（申诉核查、驳回、补入）
	agentSvc := service.NewAgentService(d.DB, d.Clk, eventSvc, d.Reader)
	agentHandler := handler.NewAgentHandler(agentSvc)
	agent := r.Group("/api/v1/agent", requireAccount(d.DB, "agent"))
	{
		agent.GET("/appeals", agentHandler.ListAppeals)
		agent.GET("/appeals/:id", agentHandler.GetAppealDetail)
		agent.POST("/appeals/:id/reject", agentHandler.RejectAppeal)
		agent.POST("/appeals/:id/compensate", agentHandler.CompensateAppeal)
	}

	// 运营端（发布活动、结算核查）
	opsSvc := service.NewOpsService(d.DB, d.Clk)
	opsHandler := handler.NewOpsHandler(opsSvc)
	ops := r.Group("/api/v1/ops", requireAccount(d.DB, "ops"))
	{
		ops.POST("/activity", opsHandler.PublishActivity)
		ops.GET("/summary", opsHandler.GetSummary)
	}

	return r
}

// requireToken 校验固定令牌（假设 A-01：模拟阅读服务采用固定 Token 鉴权）。
func requireToken(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" || c.GetHeader("X-Source-Token") != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "鉴权失败"})
			return
		}
		c.Next()
	}
}

// requireAccount 按 Bearer 令牌解析预置账号身份并校验角色，
// 将账号 ID 写入上下文（uid）。身份由服务端判定，客户端不可指定用户编号。
func requireAccount(db *sql.DB, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "缺少访问令牌"})
			return
		}
		acc, err := store.GetAccountByToken(c.Request.Context(), db, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "鉴权失败: " + err.Error()})
			return
		}
		if acc == nil || acc.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问"})
			return
		}
		c.Set("uid", acc.ID)
		c.Next()
	}
}
