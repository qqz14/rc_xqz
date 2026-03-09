package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"notification-service/internal/mock"
	"notification-service/internal/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestHandler_Submit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	tests := []struct {
		name           string
		reqBody        interface{}
		mockSetup      func(*mock.MockITaskRepo)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "invalid_json",
			reqBody: "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "missing_required_fields",
			reqBody: SubmitRequest{
				HTTPMethod: "POST",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "repo_create_error",
			reqBody: SubmitRequest{
				TargetURL:  "http://example.com",
				HTTPMethod: "POST",
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "success",
			reqBody: SubmitRequest{
				TargetURL:  "http://example.com",
				HTTPMethod: "POST",
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx interface{}, task *model.NotificationTask) error {
					task.ID = 123
					return nil
				})
			},
			expectedStatus: http.StatusAccepted,
			expectedBody:   `{"task_id":123}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mock.NewMockITaskRepo(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(mockRepo)
			}

			h := NewHandler(mockRepo)
			router := gin.New()
			router.POST("/submit", h.Submit)

			var reqBodyBytes []byte
			if str, ok := tt.reqBody.(string); ok {
				reqBodyBytes = []byte(str)
			} else {
				reqBodyBytes, _ = json.Marshal(tt.reqBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/submit", bytes.NewReader(reqBodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
		})
	}
}

func TestHandler_ManualRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	tests := []struct {
		name           string
		taskID         string
		mockSetup      func(*mock.MockITaskRepo)
		expectedStatus int
	}{
		{
			name:   "invalid_task_id",
			taskID: "abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "repo_error",
			taskID: "123",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().ResetToRetry(gomock.Any(), int64(123)).Return(errors.New("db error"))
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "success",
			taskID: "123",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().ResetToRetry(gomock.Any(), int64(123)).Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mock.NewMockITaskRepo(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(mockRepo)
			}

			h := NewHandler(mockRepo)
			router := gin.New()
			router.POST("/retry/:id", h.ManualRetry)

			req := httptest.NewRequest(http.MethodPost, "/retry/"+tt.taskID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandler_ListTasks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	tests := []struct {
		name           string
		query          string
		mockSetup      func(*mock.MockITaskRepo)
		expectedStatus int
	}{
		{
			name:  "success_default_pagination",
			query: "",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().ListByStatus(gomock.Any(), "", 1, 20).Return([]model.NotificationTask{}, int64(0), nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "success_with_status_and_pagination",
			query: "?status=FAILED&page=2&page_size=10",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().ListByStatus(gomock.Any(), "FAILED", 2, 10).Return([]model.NotificationTask{}, int64(0), nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "repo_error",
			query: "",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().ListByStatus(gomock.Any(), "", 1, 20).Return(nil, int64(0), errors.New("db error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mock.NewMockITaskRepo(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(mockRepo)
			}

			h := NewHandler(mockRepo)
			router := gin.New()
			router.GET("/tasks", h.ListTasks)

			req := httptest.NewRequest(http.MethodGet, "/tasks"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestHandler_GetTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	tests := []struct {
		name           string
		taskID         string
		mockSetup      func(*mock.MockITaskRepo)
		expectedStatus int
	}{
		{
			name:   "invalid_task_id",
			taskID: "abc",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "repo_error",
			taskID: "123",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().GetByID(gomock.Any(), int64(123)).Return(nil, errors.New("db error"))
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:   "success",
			taskID: "123",
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().GetByID(gomock.Any(), int64(123)).Return(&model.NotificationTask{ID: 123}, nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := mock.NewMockITaskRepo(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(mockRepo)
			}

			h := NewHandler(mockRepo)
			router := gin.New()
			router.GET("/tasks/:id", h.GetTask)

			req := httptest.NewRequest(http.MethodGet, "/tasks/"+tt.taskID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}