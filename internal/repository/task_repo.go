package repository

import (
	"context"
	"time"

	"notification-service/internal/model"

	"gorm.io/gorm"
)

// ITaskRepo 定义了 notification_tasks 表的所有 DB 操作接口
type ITaskRepo interface {
	Create(ctx context.Context, task *model.NotificationTask) error
	FetchPending(ctx context.Context, limit int) ([]model.NotificationTask, error)
	LockForDelivery(ctx context.Context, id int64) (int64, error)
	UpdateStatus(ctx context.Context, id int64, status string, httpStatus int, lastError string) error
	MarkRetrying(ctx context.Context, id int64, retryCount int, nextRetryTime time.Time, httpStatus int, lastError string) error
	MarkFailed(ctx context.Context, id int64, httpStatus int, lastError string) error
	ResetToRetry(ctx context.Context, id int64) error
	RecoverStuckTasks(ctx context.Context, timeout time.Duration) error
	ListByStatus(ctx context.Context, status string, page, pageSize int) ([]model.NotificationTask, int64, error)
	GetByID(ctx context.Context, id int64) (*model.NotificationTask, error)
}

// TaskRepo 封装 notification_tasks 表的所有 DB 操作
type TaskRepo struct {
	db *gorm.DB
}

// 确保 TaskRepo 实现了 ITaskRepo 接口
var _ ITaskRepo = (*TaskRepo)(nil)

// NewTaskRepo 创建 TaskRepo 实例
func NewTaskRepo(db *gorm.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

// Create 创建新任务（事务落盘）
func (r *TaskRepo) Create(ctx context.Context, task *model.NotificationTask) error {
	now := time.Now()
	task.NextRetryTime = &now
	task.Status = model.StatusPending
	return r.db.WithContext(ctx).Create(task).Error
}

// FetchPending 扫描 PENDING/RETRYING 且 next_retry_time <= NOW() 的任务，最多 limit 条
func (r *TaskRepo) FetchPending(ctx context.Context, limit int) ([]model.NotificationTask, error) {
	var tasks []model.NotificationTask
	err := r.db.WithContext(ctx).
		Where("status IN ? AND next_retry_time <= ?",
			[]string{model.StatusPending, model.StatusRetrying},
			time.Now()).
		Order("next_retry_time ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

// LockForDelivery 乐观锁：将 PENDING/RETRYING 状态更新为 DELIVERING
// 返回受影响行数，0 表示已被其他实例抢占
func (r *TaskRepo) LockForDelivery(ctx context.Context, id int64) (int64, error) {
	result := r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("id = ? AND status IN ?", id, []string{model.StatusPending, model.StatusRetrying}).
		Update("status", model.StatusDelivering)
	return result.RowsAffected, result.Error
}

// UpdateStatus 更新任务状态（成功/4xx失败）
func (r *TaskRepo) UpdateStatus(ctx context.Context, id int64, status string, httpStatus int, lastError string) error {
	updates := map[string]interface{}{
		"status":           status,
		"last_http_status": httpStatus,
		"last_error":       lastError,
	}
	return r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// MarkRetrying 标记任务为 RETRYING，更新重试次数和下次重试时间
func (r *TaskRepo) MarkRetrying(ctx context.Context, id int64, retryCount int, nextRetryTime time.Time, httpStatus int, lastError string) error {
	updates := map[string]interface{}{
		"status":           model.StatusRetrying,
		"retry_count":      retryCount,
		"next_retry_time":  nextRetryTime,
		"last_http_status": httpStatus,
		"last_error":       lastError,
	}
	return r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// MarkFailed 标记任务为最终失败
func (r *TaskRepo) MarkFailed(ctx context.Context, id int64, httpStatus int, lastError string) error {
	updates := map[string]interface{}{
		"status":           model.StatusFailed,
		"last_http_status": httpStatus,
		"last_error":       lastError,
	}
	return r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// ResetToRetry 手动重投：将 FAILED 任务重置为 PENDING
func (r *TaskRepo) ResetToRetry(ctx context.Context, id int64) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":          model.StatusPending,
		"retry_count":     0,
		"next_retry_time": now,
		"last_error":      "",
	}
	result := r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("id = ? AND status = ?", id, model.StatusFailed).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// RecoverStuckTasks 恢复僵尸任务：将超时的 DELIVERING 状态重置为 RETRYING
func (r *TaskRepo) RecoverStuckTasks(ctx context.Context, timeout time.Duration) error {
	now := time.Now()
	deadline := now.Add(-timeout)
	return r.db.WithContext(ctx).Model(&model.NotificationTask{}).
		Where("status = ? AND updated_at < ?", model.StatusDelivering, deadline).
		Updates(map[string]interface{}{
			"status":          model.StatusRetrying,
			"next_retry_time": now,
		}).Error
}

// ListByStatus 按状态查询任务列表（用于人工查询）
func (r *TaskRepo) ListByStatus(ctx context.Context, status string, page, pageSize int) ([]model.NotificationTask, int64, error) {
	var tasks []model.NotificationTask
	var total int64

	query := r.db.WithContext(ctx).Model(&model.NotificationTask{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&tasks).Error
	return tasks, total, err
}

// GetByID 按 ID 查询单个任务
func (r *TaskRepo) GetByID(ctx context.Context, id int64) (*model.NotificationTask, error) {
	var task model.NotificationTask
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}
