package config

import "time"

// Config 服务配置
type Config struct {
	// 数据库配置
	DBPath string // SQLite 数据库文件路径，默认 data.db

	// 调度器配置
	ScanInterval   time.Duration // 扫表间隔，默认 5s
	MaxConcurrency int           // 最大并发投递数，默认 10
	StuckTimeout   time.Duration // 僵尸任务超时时间，默认 60s

	// 重试配置
	BaseRetryInterval time.Duration // 指数退避基础间隔，默认 10s

	// HTTP 客户端配置
	HTTPTimeout time.Duration // 单次请求超时，默认 10s

	// 服务端口
	Port string // HTTP 监听端口，默认 :8080
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		DBPath:            "data.db",
		ScanInterval:      5 * time.Second,
		MaxConcurrency:    10,
		StuckTimeout:      60 * time.Second,
		BaseRetryInterval: 10 * time.Second,
		HTTPTimeout:       10 * time.Second,
		Port:              ":8080",
	}
}
