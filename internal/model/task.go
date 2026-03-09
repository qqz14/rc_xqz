package model

import "time"

// 任务状态常量
const (
	StatusPending    = "PENDING"
	StatusDelivering = "DELIVERING"
	StatusSuccess    = "SUCCESS"
	StatusRetrying   = "RETRYING"
	StatusFailed     = "FAILED"
)

// NotificationTask 对应 notification_tasks 表
type NotificationTask struct {
	ID             int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetURL      string     `gorm:"type:varchar(2048);not null" json:"target_url"`
	HTTPMethod     string     `gorm:"type:varchar(10);not null;default:POST" json:"http_method"`
	Headers        string     `gorm:"type:text" json:"headers"`          // JSON 字符串
	Body           string     `gorm:"type:text" json:"body"`
	Status         string     `gorm:"type:varchar(20);not null;default:PENDING;index:idx_status_retry,priority:1" json:"status"`
	RetryCount     int        `gorm:"not null;default:0" json:"retry_count"`
	MaxRetries     int        `gorm:"not null;default:3" json:"max_retries"`
	NextRetryTime  *time.Time `gorm:"index:idx_status_retry,priority:2" json:"next_retry_time"`
	LastHTTPStatus *int       `json:"last_http_status"`
	LastError      string     `gorm:"type:text" json:"last_error"`
	SourceSystem   string     `gorm:"type:varchar(100)" json:"source_system"`
	TraceID        string     `gorm:"type:varchar(64)" json:"trace_id"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (NotificationTask) TableName() string {
	return "notification_tasks"
}
