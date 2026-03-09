package dispatcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"notification-service/internal/config"
	"notification-service/internal/mock"
	"notification-service/internal/model"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDispatcher_Deliver(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	cfg := &config.Config{
		HTTPTimeout:       2 * time.Second,
		BaseRetryInterval: 10 * time.Second,
	}

	tests := []struct {
		name           string
		task           model.NotificationTask
		mockSetup      func(*mock.MockITaskRepo)
		mockServerResp func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "lock_failed",
			task: model.NotificationTask{ID: 1},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(1)).Return(int64(0), errors.New("db error"))
			},
		},
		{
			name: "already_locked_by_others",
			task: model.NotificationTask{ID: 2},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(2)).Return(int64(0), nil)
			},
		},
		{
			name: "http_200_success",
			task: model.NotificationTask{
				ID:         3,
				HTTPMethod: "POST",
				Body:       `{"key":"value"}`,
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(3)).Return(int64(1), nil)
				m.EXPECT().UpdateStatus(gomock.Any(), int64(3), model.StatusSuccess, 200, "").Return(nil)
			},
			mockServerResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		{
			name: "http_400_failed",
			task: model.NotificationTask{
				ID:         4,
				HTTPMethod: "POST",
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(4)).Return(int64(1), nil)
				m.EXPECT().UpdateStatus(gomock.Any(), int64(4), model.StatusFailed, 400, "non-retryable error: HTTP 400").Return(nil)
			},
			mockServerResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
		},
		{
			name: "http_500_retry",
			task: model.NotificationTask{
				ID:         5,
				HTTPMethod: "POST",
				RetryCount: 0,
				MaxRetries: 3,
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(5)).Return(int64(1), nil)
				m.EXPECT().MarkRetrying(gomock.Any(), int64(5), 1, gomock.Any(), 500, "server error: HTTP 500").Return(nil)
			},
			mockServerResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name: "http_500_max_retries_reached",
			task: model.NotificationTask{
				ID:         6,
				HTTPMethod: "POST",
				RetryCount: 2,
				MaxRetries: 3,
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(6)).Return(int64(1), nil)
				m.EXPECT().MarkFailed(gomock.Any(), int64(6), 500, "server error: HTTP 500").Return(nil)
			},
			mockServerResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
		},
		{
			name: "network_error_retry",
			task: model.NotificationTask{
				ID:         7,
				HTTPMethod: "POST",
				TargetURL:  "http://invalid-url-that-does-not-exist.local",
				RetryCount: 0,
				MaxRetries: 3,
			},
			mockSetup: func(m *mock.MockITaskRepo) {
				m.EXPECT().LockForDelivery(gomock.Any(), int64(7)).Return(int64(1), nil)
				m.EXPECT().MarkRetrying(gomock.Any(), int64(7), 1, gomock.Any(), 0, gomock.Any()).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockRepo := mock.NewMockITaskRepo(ctrl)
			if tt.mockSetup != nil {
				tt.mockSetup(mockRepo)
			}

			var server *httptest.Server
			if tt.mockServerResp != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.mockServerResp))
				defer server.Close()
				tt.task.TargetURL = server.URL
			}

			d := New(mockRepo, cfg)
			d.Deliver(context.Background(), tt.task)
		})
	}
}

func TestDispatcher_doHTTPRequest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mock.NewMockITaskRepo(ctrl)
	cfg := &config.Config{HTTPTimeout: 2 * time.Second}
	d := New(mockRepo, cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		assert.Equal(t, "trace-123", r.Header.Get("X-Trace-ID"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	task := model.NotificationTask{
		TargetURL:  server.URL,
		HTTPMethod: "POST",
		Headers:    `{"Authorization": "Bearer token"}`,
		TraceID:    "trace-123",
		Body:       `{"msg": "hello"}`,
	}

	resp, err := d.doHTTPRequest(context.Background(), task)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}