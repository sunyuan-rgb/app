package handler_test

import (
	"testing"

	"reading-activity/internal/clock"
	"reading-activity/internal/server/handler"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// 真实测试库 + 直接调用 handler：覆盖认证流 HTTP 层（登录/注册全分支）。
func TestHandler_AuthFlow(t *testing.T) {
	db := testutil.Setup(t)
	// 模拟迁移 005 的预置口径：user1 / 123456
	if _, err := db.Exec(`INSERT INTO user_account (username, role, token, password_hash)
		VALUES ('user1','user','user-token-1', SHA2(CONCAT('user1', ':', '123456'), 256))`); err != nil {
		t.Fatal(err)
	}
	authH := handler.NewAuthHandler(service.NewAuthService(db, clock.System{}))

	// 登录成功 200：返回会话令牌
	c, w := testCtx("POST", "/x", `{"username":"user1","password":"123456"}`)
	authH.Login(c)
	if w.Code != 200 {
		t.Fatalf("login ok => %d %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); len(body) < 20 || !contains(body, `"token"`) {
		t.Fatalf("login body missing token: %s", body)
	}

	// 口令错误 401（不泄露账号存在性）
	c, w = testCtx("POST", "/x", `{"username":"user1","password":"wrong"}`)
	authH.Login(c)
	if w.Code != 401 {
		t.Fatalf("login wrong pass => %d", w.Code)
	}
	// 账号不存在同样 401
	c, w = testCtx("POST", "/x", `{"username":"ghost","password":"123456"}`)
	authH.Login(c)
	if w.Code != 401 {
		t.Fatalf("login ghost => %d", w.Code)
	}

	// 参数不合法 400
	c, w = testCtx("POST", "/x", `not-json`)
	authH.Login(c)
	if w.Code != 400 {
		t.Fatalf("login bad json => %d", w.Code)
	}

	// 注册成功 201（user 角色 + 令牌）
	c, w = testCtx("POST", "/x", `{"username":"new_user","password":"pass123"}`)
	authH.Register(c)
	if w.Code != 201 {
		t.Fatalf("register ok => %d %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !contains(body, `"role":"user"`) {
		t.Fatalf("register body role mismatch: %s", body)
	}

	// 重名注册 409
	c, w = testCtx("POST", "/x", `{"username":"new_user","password":"pass456"}`)
	authH.Register(c)
	if w.Code != 409 {
		t.Fatalf("register dup => %d", w.Code)
	}

	// 弱口令 400
	c, w = testCtx("POST", "/x", `{"username":"another1","password":"12345"}`)
	authH.Register(c)
	if w.Code != 400 {
		t.Fatalf("register weak pass => %d", w.Code)
	}

	// 非法账号名 400
	c, w = testCtx("POST", "/x", `{"username":"bad name!","password":"pass123"}`)
	authH.Register(c)
	if w.Code != 400 {
		t.Fatalf("register bad name => %d", w.Code)
	}

	// 注册令牌可直接登录（会话闭环）
	c, w = testCtx("POST", "/x", `{"username":"new_user","password":"pass123"}`)
	authH.Login(c)
	if w.Code != 200 {
		t.Fatalf("register->login => %d", w.Code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
