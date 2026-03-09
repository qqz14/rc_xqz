package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"notification-service/internal/config"
	"notification-service/internal/model"
	"notification-service/internal/repository"
)

// Dispatcher 投递引擎：负责发起 HTTP 请求并处理响应
type Dispatcher struct {
	repo       repository.ITaskRepo
	cfg        *config.Config
	httpClient *http.Client
}

// New 创建 Dispatcher 实例
func New(repo repository.ITaskRepo, cfg *config.Config) *Dispatcher {
	httpClient := &http.Client{
		Timeout: cfg.HTTPTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	return &Dispatcher{
		repo:       repo,
		cfg:        cfg,
		httpClient: httpClient,
	}
}

// Deliver 执行单次 HTTP 投递，并根据结果更新任务状态
func (d *Dispatcher) Deliver(ctx context.Context, task model.NotificationTask) {
	// 乐观锁：将状态从 PENDING/RETRYING 更新为 DELIVERING
	// affected_rows = 0 说明已被其他实例抢占，直接跳过
	affected, err := d.repo.LockForDelivery(ctx, task.ID)
	if err != nil {
		log.Printf("[Dispatcher] LockForDelivery error, task_id=%d: %v", task.ID, err)
		return
	}
	if affected == 0 {
		log.Printf("[Dispatcher] task_id=%d already locked by another instance, skip", task.ID)
		return
	}

	log.Printf("[Dispatcher] delivering task_id=%d to %s", task.ID, task.TargetURL)

	resp, err := d.doHTTPRequest(ctx, task)
	if err != nil {
		// 如果是优雅停机导致的 context 取消，不消耗重试次数，直接退出
		if errors.Is(err, context.Canceled) {
			log.Printf("[Dispatcher] task_id=%d canceled due to shutdown, skip retry", task.ID)
			return
		}
		// 网络错误：可重试
		log.Printf("[Dispatcher] task_id=%d network error: %v", task.ID, err)
		d.handleRetry(ctx, task, 0, err.Error())
		return
	}
	defer resp.Body.Close()

	log.Printf("[Dispatcher] task_id=%d got HTTP %d", task.ID, resp.StatusCode)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// 2xx：投递成功
		if err := d.repo.UpdateStatus(ctx, task.ID, model.StatusSuccess, resp.StatusCode, ""); err != nil {
			log.Printf("[Dispatcher] UpdateStatus(SUCCESS) error, task_id=%d: %v", task.ID, err)
		}

	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// 4xx：确定性错误，直接 FAILED，不重试
		errMsg := fmt.Sprintf("non-retryable error: HTTP %d", resp.StatusCode)
		if err := d.repo.UpdateStatus(ctx, task.ID, model.StatusFailed, resp.StatusCode, errMsg); err != nil {
			log.Printf("[Dispatcher] UpdateStatus(FAILED) error, task_id=%d: %v", task.ID, err)
		}

	default:
		// 5xx 或其他：临时错误，触发重试
		errMsg := fmt.Sprintf("server error: HTTP %d", resp.StatusCode)
		d.handleRetry(ctx, task, resp.StatusCode, errMsg)
	}
}

// doHTTPRequest 构造并发起 HTTP 请求
func (d *Dispatcher) doHTTPRequest(ctx context.Context, task model.NotificationTask) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if task.Body != "" {
		bodyReader = bytes.NewReader([]byte(task.Body))
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, task.HTTPMethod, task.TargetURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	// 设置默认 Content-Type
	req.Header.Set("Content-Type", "application/json")

	// 解析并设置自定义 Headers
	if task.Headers != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(task.Headers), &headers); err == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
		}
	}

	// 传递 trace_id 便于外部系统幂等
	if task.TraceID != "" {
		req.Header.Set("X-Trace-ID", task.TraceID)
	}

	return d.httpClient.Do(req)
}

// handleRetry 计算指数退避时间并更新状态
// 指数退避公式：next_retry_time = NOW() + 2^retryCount * baseInterval
func (d *Dispatcher) handleRetry(ctx context.Context, task model.NotificationTask, httpStatus int, errMsg string) {
	nextRetryCount := task.RetryCount + 1

	if nextRetryCount >= task.MaxRetries {
		// 超过最大重试次数，标记 FAILED
		log.Printf("[Dispatcher] task_id=%d reached max retries(%d), marking FAILED", task.ID, task.MaxRetries)
		if err := d.repo.MarkFailed(ctx, task.ID, httpStatus, errMsg); err != nil {
			log.Printf("[Dispatcher] MarkFailed error, task_id=%d: %v", task.ID, err)
		}
		return
	}

	// 指数退避：2^retryCount * baseInterval
	// 第1次: 2^1 * 10s = 20s
	// 第2次: 2^2 * 10s = 40s
	// 第3次: 2^3 * 10s = 80s
	backoff := time.Duration(math.Pow(2, float64(nextRetryCount))) * d.cfg.BaseRetryInterval
	nextRetryTime := time.Now().Add(backoff)

	log.Printf("[Dispatcher] task_id=%d retry %d/%d, next retry at %s (backoff=%s)",
		task.ID, nextRetryCount, task.MaxRetries, nextRetryTime.Format(time.RFC3339), backoff)

	if err := d.repo.MarkRetrying(ctx, task.ID, nextRetryCount, nextRetryTime, httpStatus, errMsg); err != nil {
		log.Printf("[Dispatcher] MarkRetrying error, task_id=%d: %v", task.ID, err)
	}
}
