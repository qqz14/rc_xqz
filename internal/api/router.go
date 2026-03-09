package api

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine, h *Handler) {
	v1 := r.Group("/api/v1")
	{
		// 提交通知请求
		v1.POST("/notifications", h.Submit)

		// 查询任务列表（支持 ?status=FAILED&page=1&page_size=20）
		v1.GET("/notifications", h.ListTasks)

		// 查询单个任务详情
		v1.GET("/notifications/:id", h.GetTask)

		// 手动重投 FAILED 任务
		v1.POST("/notifications/:id/retry", h.ManualRetry)
	}
}
