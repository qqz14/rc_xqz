package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"notification-service/internal/config"
	"notification-service/internal/dispatcher"
	"notification-service/internal/mock"
	"notification-service/internal/model"

	"go.uber.org/mock/gomock"
)

func TestScheduler_Start(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mock.NewMockITaskRepo(ctrl)
	cfg := &config.Config{
		ScanInterval:   10 * time.Millisecond,
		MaxConcurrency: 2,
	}

	// We don't want to actually make HTTP requests, so we mock the repo calls inside dispatcher
	mockRepo.EXPECT().RecoverStuckTasks(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	
	// First tick returns some tasks
	mockRepo.EXPECT().FetchPending(gomock.Any(), 100).Return([]model.NotificationTask{
		{ID: 1},
		{ID: 2},
	}, nil).Times(1)

	// Subsequent ticks return empty
	mockRepo.EXPECT().FetchPending(gomock.Any(), 100).Return([]model.NotificationTask{}, nil).AnyTimes()

	// Mock dispatcher behavior
	mockRepo.EXPECT().LockForDelivery(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()

	disp := dispatcher.New(mockRepo, cfg)
	s := New(mockRepo, disp, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	s.Start(ctx)
}

func TestScheduler_dispatchPending_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mock.NewMockITaskRepo(ctrl)
	cfg := &config.Config{
		MaxConcurrency: 2,
	}

	mockRepo.EXPECT().FetchPending(gomock.Any(), 100).Return(nil, errors.New("db error"))

	disp := dispatcher.New(mockRepo, cfg)
	s := New(mockRepo, disp, cfg)

	s.dispatchPending(context.Background())
}

func TestScheduler_recoverStuckTasks_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockRepo := mock.NewMockITaskRepo(ctrl)
	cfg := &config.Config{}

	mockRepo.EXPECT().RecoverStuckTasks(gomock.Any(), gomock.Any()).Return(errors.New("db error"))

	disp := dispatcher.New(mockRepo, cfg)
	s := New(mockRepo, disp, cfg)

	s.recoverStuckTasks(context.Background())
}