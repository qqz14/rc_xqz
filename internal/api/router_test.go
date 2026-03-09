package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"notification-service/internal/mock"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRegisterRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mock.NewMockITaskRepo(ctrl)
	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRepo.EXPECT().ResetToRetry(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRepo.EXPECT().ListByStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).AnyTimes()
	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	h := NewHandler(mockRepo)

	router := gin.New()
	RegisterRoutes(router, h)

	// Test if routes are registered correctly
	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/notifications"},
		{"POST", "/api/v1/notifications/1/retry"},
		{"GET", "/api/v1/notifications"},
		{"GET", "/api/v1/notifications/1"},
	}

	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// We just want to make sure it doesn't return 404 Not Found
		// It might return 400 Bad Request because of missing body/params, which is fine
		assert.NotEqual(t, http.StatusNotFound, w.Code, "Route %s %s should be registered", route.method, route.path)
	}
}