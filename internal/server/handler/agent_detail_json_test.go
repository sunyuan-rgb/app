package handler_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/clock"
	"reading-activity/internal/model"
	"reading-activity/internal/server/handler"
	"reading-activity/internal/service"
	"reading-activity/internal/testutil"
)

// 回归测试（D-12）：申诉详情 JSON 字段必须为小写蛇形（与前端约定一致），
// 防止 model/struct 缺少 json tag 导致 PascalCase 字段再次出现在响应中。
func TestAppealDetailJSONKeysAreSnakeCase(t *testing.T) {
	db := testutil.Setup(t)
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, model.Beijing)

	if _, err := db.Exec(`INSERT INTO activity (name, start_at, end_at, daily_threshold, reward_points, status)
		VALUES ('活动', '2026-11-11 00:00:00', '2026-11-13 00:00:00', 3, 30, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO appeal (user_id, activity_id, biz_date, content, status)
		VALUES (1, 1, '2026-11-12', '水电费', 0)`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	agentSvc := service.NewAgentService(db, clock.Fixed{T: now}, service.NewEventService(db, clock.Fixed{T: now}), nil)
	agentH := handler.NewAgentHandler(agentSvc)

	c, w := testCtx("GET", "/x", "")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	agentH.GetAppealDetail(c)
	if w.Code != 200 {
		t.Fatalf("detail => %d %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appeal, ok := body["appeal"].(map[string]any)
	if !ok {
		t.Fatalf("missing appeal object: %s", w.Body.String())
	}
	for _, key := range []string{"id", "user_id", "biz_date", "content", "status", "conclusion", "operator_id", "created_at"} {
		if _, ok := appeal[key]; !ok {
			t.Fatalf("appeal 缺少小写字段 %q，实际键: %s", key, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), `"ID"`) || strings.Contains(w.Body.String(), `"UserID"`) {
		t.Fatalf("响应含 PascalCase 字段（缺 json tag）: %s", w.Body.String())
	}
}
