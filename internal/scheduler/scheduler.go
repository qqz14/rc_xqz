package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"notification-service/internal/config"
	"notification-service/internal/dispatcher"
	"notification-service/internal/model"
	"notification-service/internal/repository"
)

// Scheduler 定时调度器
type Scheduler struct {
	repo       repository.ITaskRepo
	dispatcher *dispatcher.Dispatcher
	cfg        *config.Config
}

// New 创建 Scheduler 实例
func New(repo repository.ITaskRepo, disp *dispatcher.Dispatcher, cfg *config.Config) *Scheduler {
	return &Scheduler{
		repo:       repo,
		dispatcher: disp,
		cfg:        cfg,
	}
}

// Start 启动定时调度器，阻塞直到 ctx 取消
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.ScanInterval)
	defer ticker.Stop()

	log.Printf("[Scheduler] started, scan interval=%s, max_concurrency=%d",
		s.cfg.ScanInterval, s.cfg.MaxConcurrency)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Scheduler] stopped")
			return
		case <-ticker.C:
			s.recoverStuckTasks(ctx) // 先恢复僵尸任务
			s.dispatchPending(ctx)   // 再投递待处理任务
		}
	}
}

// recoverStuckTasks 恢复长时间处于 DELIVERING 状态的僵尸任务
func (s *Scheduler) recoverStuckTasks(ctx context.Context) {
	if err := s.repo.RecoverStuckTasks(ctx, s.cfg.StuckTimeout); err != nil {
		log.Printf("[Scheduler] recoverStuckTasks error: %v", err)
	}
}

// dispatchPending 扫描并并发投递 PENDING/RETRYING 任务
func (s *Scheduler) dispatchPending(ctx context.Context) {
	tasks, err := s.repo.FetchPending(ctx, 100)
	if err != nil {
		log.Printf("[Scheduler] FetchPending error: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	log.Printf("[Scheduler] fetched %d pending tasks", len(tasks))

	var wg sync.WaitGroup
	// 使用信号量控制并发数，避免瞬间打满 DB 连接
	sem := make(chan struct{}, s.cfg.MaxConcurrency)

	for _, task := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(t model.NotificationTask) {
			defer wg.Done()
			defer func() { <-sem }()
			s.dispatcher.Deliver(ctx, t)
		}(task)
	}
	wg.Wait()
}