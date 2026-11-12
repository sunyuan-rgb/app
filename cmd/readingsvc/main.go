// 模拟阅读服务（题目 10）：独立进程。
// 环境变量：APP_DSN、RSVC_ADDR(:8081)、MAIN_SVC_URL、EVENTS_TOKEN、TEACH_CLOCK_OFFSET(可选)
package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"reading-activity/internal/config"
	"reading-activity/internal/readingsvc"
	"reading-activity/internal/store"
)

func main() {
	cfg, err := config.LoadReading()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	clk, err := cfg.Clock()
	if err != nil {
		log.Fatalf("clock: %v", err)
	}

	db, err := store.Open(cfg.DSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	svc := readingsvc.New(db, clk, cfg.MainURL, cfg.EventsToken)

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	svc.RegisterRoutes(r, clk)

	log.Printf("reading svc listening on %s (main=%s, now=%s)", cfg.Addr, cfg.MainURL, clk.Now())
	if err := r.Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
