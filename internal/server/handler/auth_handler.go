package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/service"
)

// AuthHandler 注册/登录接口（公开路由，不经过账号鉴权中间件）。
type AuthHandler struct {
	svc *service.AuthService
}

// NewAuthHandler 创建认证处理器。
func NewAuthHandler(svc *service.AuthService) *AuthHandler { return &AuthHandler{svc: svc} }

type authReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authResp struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
	UserID   int64  `json:"user_id"`
}

// Login 登录：校验口令，返回会话令牌。
func (h *AuthHandler) Login(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	acc, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, authResp{Token: acc.Token, Username: acc.Username, Role: acc.Role, UserID: acc.ID})
}

// Register 注册：创建 user 角色账号并直接下发会话令牌。
func (h *AuthHandler) Register(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不合法: " + err.Error()})
		return
	}
	acc, err := h.svc.Register(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrUsernameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrWeakPassword), errors.Is(err, service.ErrUsernameInvalid):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusCreated, authResp{Token: acc.Token, Username: acc.Username, Role: acc.Role, UserID: acc.ID})
}
