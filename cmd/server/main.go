package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reading-activity/internal/config"
	"reading-activity/internal/server"
	"reading-activity/internal/service"
	"reading-activity/internal/store"
)

func main() {
	cfg, err := config.LoadServer()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	clk, err := cfg.Clock()
	if err != nil {
		log.Fatalf("clock: %v", err)
	}

	// 模拟阅读服务可信查询客户端（未配置则为 nil，核查详情中注明）
	var reader service.ReadingServiceClient
	if cfg.ReadingSvcURL != "" {
		reader = service.NewHTTPReadingService(cfg.ReadingSvcURL)
	}

	db, err := store.Open(cfg.DSN)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		// 允许先启动服务（如冒烟检查），业务接口在数据库不可用时报错
		log.Printf("WARN db ping failed: %v", err)
	}

	r := server.New(server.Deps{
		DB:          db,
		Clk:         clk,
		EventsToken: cfg.EventsToken,
		Reader:      reader,
	})

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
	log.Println("server exited")
}
