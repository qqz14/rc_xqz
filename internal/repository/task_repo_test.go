package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"notification-service/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	sqlDB, mock, err := sqlmock.New()
	assert.NoError(t, err)

	mock.ExpectQuery("select sqlite_version()").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("3.31.1"))

	dialector := sqlite.Dialector{Conn: sqlDB}
	db, err := gorm.Open(dialector, &gorm.Config{
		SkipDefaultTransaction: true,
	})
	assert.NoError(t, err)

	return db, mock, sqlDB
}

func TestTaskRepo_Create(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)
	task := &model.NotificationTask{
		TargetURL: "http://example.com",
	}

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `notification_tasks`")).
		WithArgs(
			sqlmock.AnyArg(), // TargetURL
			sqlmock.AnyArg(), // HTTPMethod
			sqlmock.AnyArg(), // Headers
			sqlmock.AnyArg(), // Body
			model.StatusPending, // Status
			sqlmock.AnyArg(), // RetryCount
			sqlmock.AnyArg(), // MaxRetries
			sqlmock.AnyArg(), // NextRetryTime
			sqlmock.AnyArg(), // LastHTTPStatus
			sqlmock.AnyArg(), // LastError
			sqlmock.AnyArg(), // SourceSystem
			sqlmock.AnyArg(), // TraceID
			sqlmock.AnyArg(), // CreatedAt
			sqlmock.AnyArg(), // UpdatedAt
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(context.Background(), task)
	assert.NoError(t, err)
	assert.Equal(t, model.StatusPending, task.Status)
	assert.NotNil(t, task.NextRetryTime)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_FetchPending(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	rows := sqlmock.NewRows([]string{"id", "status"}).
		AddRow(1, model.StatusPending).
		AddRow(2, model.StatusRetrying)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `notification_tasks` WHERE status IN (?,?) AND next_retry_time <= ? ORDER BY next_retry_time ASC LIMIT 10")).
		WithArgs(model.StatusPending, model.StatusRetrying, sqlmock.AnyArg()).
		WillReturnRows(rows)

	tasks, err := repo.FetchPending(context.Background(), 10)
	assert.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_LockForDelivery(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `status`=?,`updated_at`=? WHERE id = ? AND status IN (?,?)")).
		WithArgs(model.StatusDelivering, sqlmock.AnyArg(), 1, model.StatusPending, model.StatusRetrying).
		WillReturnResult(sqlmock.NewResult(1, 1))

	affected, err := repo.LockForDelivery(context.Background(), 1)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_UpdateStatus(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `last_error`=?,`last_http_status`=?,`status`=?,`updated_at`=? WHERE id = ?")).
		WithArgs("error msg", 400, model.StatusFailed, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.UpdateStatus(context.Background(), 1, model.StatusFailed, 400, "error msg")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_MarkRetrying(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)
	nextTime := time.Now()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `last_error`=?,`last_http_status`=?,`next_retry_time`=?,`retry_count`=?,`status`=?,`updated_at`=? WHERE id = ?")).
		WithArgs("error msg", 500, nextTime, 1, model.StatusRetrying, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.MarkRetrying(context.Background(), 1, 1, nextTime, 500, "error msg")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_MarkFailed(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `last_error`=?,`last_http_status`=?,`status`=?,`updated_at`=? WHERE id = ?")).
		WithArgs("error msg", 500, model.StatusFailed, sqlmock.AnyArg(), 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.MarkFailed(context.Background(), 1, 500, "error msg")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_ResetToRetry(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `last_error`=?,`next_retry_time`=?,`retry_count`=?,`status`=?,`updated_at`=? WHERE id = ? AND status = ?")).
		WithArgs("", sqlmock.AnyArg(), 0, model.StatusPending, sqlmock.AnyArg(), 1, model.StatusFailed).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.ResetToRetry(context.Background(), 1)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_RecoverStuckTasks(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `notification_tasks` SET `next_retry_time`=?,`status`=?,`updated_at`=? WHERE status = ? AND updated_at < ?")).
		WithArgs(sqlmock.AnyArg(), model.StatusRetrying, sqlmock.AnyArg(), model.StatusDelivering, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 2))

	err := repo.RecoverStuckTasks(context.Background(), 60*time.Second)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_ListByStatus(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	countRows := sqlmock.NewRows([]string{"count"}).AddRow(2)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `notification_tasks` WHERE status = ?")).
		WithArgs(model.StatusFailed).
		WillReturnRows(countRows)

	taskRows := sqlmock.NewRows([]string{"id", "status"}).
		AddRow(1, model.StatusFailed).
		AddRow(2, model.StatusFailed)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `notification_tasks` WHERE status = ? ORDER BY created_at DESC LIMIT 10")).
		WithArgs(model.StatusFailed).
		WillReturnRows(taskRows)

	tasks, total, err := repo.ListByStatus(context.Background(), model.StatusFailed, 1, 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, tasks, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTaskRepo_GetByID(t *testing.T) {
	db, mock, sqlDB := setupMockDB(t)
	defer sqlDB.Close()

	repo := NewTaskRepo(db)

	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(1, model.StatusPending)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `notification_tasks` WHERE id = ? ORDER BY `notification_tasks`.`id` LIMIT 1")).
		WithArgs(1).
		WillReturnRows(rows)

	task, err := repo.GetByID(context.Background(), 1)
	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, int64(1), task.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}