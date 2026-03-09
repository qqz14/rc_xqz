package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"notification-service/internal/api"
	"notification-service/internal/config"
	"notification-service/internal/dispatcher"
	"notification-service/internal/model"
	"notification-service/internal/repository"
	"notification-service/internal/scheduler"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// 1. 加载配置
	cfg := config.DefaultConfig()

	// 2. 初始化 SQLite 数据库
	db, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	log.Printf("database connected: %s", cfg.DBPath)

	// 3. AutoMigrate 自动建表
	if err := db.AutoMigrate(&model.NotificationTask{}); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	log.Println("database migration completed")

	// 4. 初始化各层依赖
	repo := repository.NewTaskRepo(db)
	disp := dispatcher.New(repo, cfg)
	sched := scheduler.New(repo, disp, cfg)
	handler := api.NewHandler(repo)

	// 5. 初始化 Gin 路由
	r := gin.Default()
	api.RegisterRoutes(r, handler)

	// 6. 启动后台调度器（独立 goroutine）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)
	log.Println("scheduler started in background")

	// 7. 启动 HTTP 服务器
	srv := &http.Server{
		Addr:    cfg.Port,
		Handler: r,
	}

	go func() {
		log.Printf("HTTP server listening on %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 8. 优雅关闭：等待系统信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")
	cancel() // 停止调度器

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}
	log.Println("server exited")
}