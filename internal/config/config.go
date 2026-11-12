// Package config 服务启动配置：环境变量驱动（教学环境），可测试。
package config

import (
	"fmt"
	"os"
	"time"

	"reading-activity/internal/clock"
)

// Config 进程启动配置。
type Config struct {
	Addr          string        // 监听地址
	DSN           string        // 数据库连接串
	EventsToken   string        // 事件接入令牌（假设 A-01）
	MainURL       string        // 阅读服务投递目标（主服务地址）
	ReadingSvcURL string        // 主服务可信查询客户端地址（空表示未配置）
	ClockOffset   time.Duration // 教学时钟偏移；0 表示使用系统时间
}

// LoadServer 主服务配置：APP_ADDR / APP_DSN / EVENTS_TOKEN / READING_SVC_URL / TEACH_CLOCK_OFFSET。
func LoadServer() (Config, error) {
	c := Config{
		Addr:          envOr(os.Getenv, "APP_ADDR", ":8080"),
		DSN:           envOr(os.Getenv, "APP_DSN", "root@tcp(127.0.0.1:3306)/reading_activity?parseTime=true&loc=UTC"),
		EventsToken:   envOr(os.Getenv, "EVENTS_TOKEN", "teach-events-token"),
		ReadingSvcURL: os.Getenv("READING_SVC_URL"),
	}
	var err error
	if c.ClockOffset, err = parseOffset(os.Getenv("TEACH_CLOCK_OFFSET")); err != nil {
		return c, err
	}
	return c, nil
}

// LoadReading 模拟阅读服务配置：RSVC_ADDR / APP_DSN / MAIN_SVC_URL / EVENTS_TOKEN / TEACH_CLOCK_OFFSET。
// 口令一律通过 APP_DSN 注入（见 .env.example），默认值不含任何口令。
func LoadReading() (Config, error) {
	c := Config{
		Addr:        envOr(os.Getenv, "RSVC_ADDR", ":8081"),
		DSN:         envOr(os.Getenv, "APP_DSN", "reading@tcp(127.0.0.1:3306)/reading_activity?parseTime=true&loc=UTC"),
		MainURL:     envOr(os.Getenv, "MAIN_SVC_URL", "http://localhost:8080"),
		EventsToken: envOr(os.Getenv, "EVENTS_TOKEN", "teach-events-token"),
	}
	var err error
	if c.ClockOffset, err = parseOffset(os.Getenv("TEACH_CLOCK_OFFSET")); err != nil {
		return c, err
	}
	return c, nil
}

// Clock 按配置返回时钟：偏移非零为教学时钟（真实时间 + 偏移），
// 用于演示"活动结束后窗口内迟到事件"等跨日场景，须在报告中注明。
func (c Config) Clock() (clock.Clock, error) {
	if c.ClockOffset == 0 {
		return clock.System{}, nil
	}
	return clock.Shift{Offset: c.ClockOffset}, nil
}

func envOr(get func(string) string, key, def string) string {
	if v := get(key); v != "" {
		return v
	}
	return def
}

func parseOffset(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d == 0 {
		return 0, fmt.Errorf("TEACH_CLOCK_OFFSET 不合法: %q", v)
	}
	return d, nil
}
