package config

import (
	"testing"
	"time"
)

func TestLoadServerDefaults(t *testing.T) {
	c, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.EventsToken != "teach-events-token" {
		t.Fatalf("默认值不符: %+v", c)
	}
	if c.ReadingSvcURL != "" {
		t.Fatalf("未配置时 ReadingSvcURL 应为空: %q", c.ReadingSvcURL)
	}
	clk, err := c.Clock()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := clk.(interface{ Now() time.Time }); !ok {
		t.Fatal("应返回有效时钟")
	}
}

func TestLoadServerOverride(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("EVENTS_TOKEN", "tok-1")
	t.Setenv("READING_SVC_URL", "http://localhost:8081")
	c, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":9090" || c.EventsToken != "tok-1" || c.ReadingSvcURL != "http://localhost:8081" {
		t.Fatalf("覆盖不符: %+v", c)
	}
}

func TestLoadReadingDefaults(t *testing.T) {
	c, err := LoadReading()
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8081" || c.MainURL != "http://localhost:8080" {
		t.Fatalf("默认值不符: %+v", c)
	}
}

func TestClockOffset(t *testing.T) {
	t.Setenv("TEACH_CLOCK_OFFSET", "24h")
	c, err := LoadReading()
	if err != nil {
		t.Fatal(err)
	}
	clk, err := c.Clock()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	got := clk.Now()
	if got.Sub(now) < 23*time.Hour || got.Sub(now) > 25*time.Hour {
		t.Fatalf("教学时钟偏移应约 24h: %v vs %v", got, now)
	}
}

func TestClockOffsetInvalid(t *testing.T) {
	t.Setenv("TEACH_CLOCK_OFFSET", "not-a-duration")
	if _, err := LoadServer(); err == nil {
		t.Fatal("非法偏移应报错")
	}
	t.Setenv("TEACH_CLOCK_OFFSET", "0s")
	if _, err := LoadServer(); err == nil {
		t.Fatal("零偏移应报错")
	}
	t.Setenv("TEACH_CLOCK_OFFSET", "bad-value")
	if _, err := LoadReading(); err == nil {
		t.Fatal("阅读服务同样校验偏移")
	}
}
