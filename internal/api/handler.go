package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"notification-service/internal/model"
	"notification-service/internal/repository"

	"github.com/gin-gonic/gin"
)

// Handler 处理 HTTP 请求
type Handler struct {
	repo repository.ITaskRepo
}

// NewHandler 创建 Handler 实例
func NewHandler(repo repository.ITaskRepo) *Handler {
	return &Handler{repo: repo}
}

// SubmitRequest 业务系统提交的通知请求
type SubmitRequest struct {
	TargetURL    string            `json:"target_url" binding:"required,url"`
	HTTPMethod   string            `json:"http_method" binding:"required,oneof=POST PUT PATCH"`
	Headers      map[string]string `json:"headers"`
	Body         string            `json:"body"`
	SourceSystem string            `json:"source_system"`
	TraceID      string            `json:"trace_id"`
	MaxRetries   int               `json:"max_retries"` // 可选，默认 3
}

// Submit 接收通知请求，事务落盘后返回 202 Accepted
func (h *Handler) Submit(c *gin.Context) {
	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 序列化 headers 为 JSON 字符串
	headersJSON := ""
	if len(req.Headers) > 0 {
		b, err := json.Marshal(req.Headers)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid headers format"})
			return
		}
		headersJSON = string(b)
	}

	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	task := &model.NotificationTask{
		TargetURL:    req.TargetURL,
		HTTPMethod:   req.HTTPMethod,
		Headers:      headersJSON,
		Body:         req.Body,
		SourceSystem: req.SourceSystem,
		TraceID:      req.TraceID,
		MaxRetries:   maxRetries,
	}

	if err := h.repo.Create(c.Request.Context(), task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task"})
		return
	}

	// 202 Accepted：已接受，异步处理
	c.JSON(http.StatusAccepted, gin.H{"task_id": task.ID})
}

// ManualRetry 手动重投 FAILED 任务
func (h *Handler) ManualRetry(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	if err := h.repo.ResetToRetry(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found or not in FAILED status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task reset to PENDING"})
}

// ListTasks 查询任务列表，支持按 status 过滤
func (h *Handler) ListTasks(c *gin.Context) {
	status := c.Query("status")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	tasks, total, err := h.repo.ListByStatus(c.Request.Context(), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"tasks":     tasks,
	})
}

// GetTask 查询单个任务详情
func (h *Handler) GetTask(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task id"})
		return
	}

	task, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}