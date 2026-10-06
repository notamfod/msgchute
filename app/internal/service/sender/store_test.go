package sender

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
)

// newMockDB creates sqlx.DB and sqlmock for tests
func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(db, "postgres")
	return sqlxDB, mock
}

// setupStore creates WorkerStore with mocks
func setupStore(
	ctx context.Context,
	db *sqlx.DB,
	taskRepo taskRepo,
	taskResultRepo taskResultRepo,
	messageRepo messageRepo,
) *WorkerStore {
	return &WorkerStore{
		ctx:            ctx,
		db:             db,
		taskRepo:       taskRepo,
		taskResultRepo: taskResultRepo,
		messageRepo:    messageRepo,
	}
}

func TestWorkerStore_GetTasks_Success(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	taskRepo := new(MockTaskRepo)
	resultRepo := new(MockTaskResultRepo)
	msgRepo := new(MockMessageRepo)

	store := setupStore(ctx, db, taskRepo, resultRepo, msgRepo)

	msgID1 := uuid.New()
	msgID2 := uuid.New()
	taskID1 := uuid.New()
	taskID2 := uuid.New()
	now := time.Now()

	task1 := dto.Task{
		ID:        taskID1,
		MessageID: msgID1,
		Status:    retrier.StatusPending,
		Worker:    "sms",
		NextRun:   now.Add(-time.Hour),
	}
	task2 := dto.Task{
		ID:        taskID2,
		MessageID: msgID2,
		Status:    retrier.StatusPending,
		Worker:    "email",
		NextRun:   now.Add(-time.Minute),
	}

	msg1 := dto.Message{ID: msgID1, Subject: "subj1", Body: "body1", BodySource: dto.MessageBodySourceProvided}
	msg2 := dto.Message{ID: msgID2, Subject: "subj2", Body: "body2"}

	taskMap := map[uuid.UUID][]dto.Task{
		msgID1: {task1},
		msgID2: {task2},
	}

	// Expect ReleaseHungTasks
	taskRepo.On("ReleaseHungTasks", mock.Anything).Return(nil).Once()

	taskRepo.On("List", mock.Anything, mock.MatchedBy(func(filter dto.TaskFilter) bool {
		return len(filter.Statuses) == 1 &&
			filter.Statuses[0] == retrier.StatusPending &&
			filter.NextRunBefore != nil
	})).Return(taskMap, nil)

	msgRepo.On("GetByIDs", mock.Anything, mock.MatchedBy(func(ids []uuid.UUID) bool {
		if len(ids) != 2 {
			return false
		}
		return (ids[0] == msgID1 && ids[1] == msgID2) || (ids[0] == msgID2 && ids[1] == msgID1)
	})).Return([]*dto.Message{&msg1, &msg2}, nil)

	taskRepo.On("Lock", mock.Anything, mock.Anything).Return(nil).Once()

	sqlMock.ExpectBegin()
	sqlMock.ExpectCommit()

	tasks, err := store.GetTasks()
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	found := 0
	for _, task := range tasks {
		if task.ID == taskID1 {
			assert.Equal(t, retrier.StatusPending, task.Status)
			assert.Equal(t, "sms", task.Worker)
			var payloadMsg dto.Message
			err = sonic.Unmarshal(task.Payload, &payloadMsg)
			require.NoError(t, err)
			assert.Equal(t, msg1.ID, payloadMsg.ID)
			assert.Equal(t, dto.MessageBodySourceProvided, payloadMsg.BodySource)
			found++
		} else if task.ID == taskID2 {
			assert.Equal(t, retrier.StatusPending, task.Status)
			assert.Equal(t, "email", task.Worker)
			var payloadMsg dto.Message
			err = sonic.Unmarshal(task.Payload, &payloadMsg)
			require.NoError(t, err)
			assert.Equal(t, msg2.ID, payloadMsg.ID)
			found++
		}
	}
	assert.Equal(t, 2, found)

	taskRepo.AssertExpectations(t)
	msgRepo.AssertExpectations(t)
	require.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_GetTasks_EmptyList(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	taskRepo.On("ReleaseHungTasks", mock.Anything).Return(nil).Once()
	taskRepo.On("List", mock.Anything, mock.Anything).Return(map[uuid.UUID][]dto.Task{}, nil)

	store := setupStore(context.Background(), db, taskRepo, new(MockTaskResultRepo), new(MockMessageRepo))

	sqlMock.ExpectBegin()
	sqlMock.ExpectCommit()

	tasks, err := store.GetTasks()
	assert.NoError(t, err)
	assert.Empty(t, tasks)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_GetTasks_ListError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	taskRepo.On("ReleaseHungTasks", mock.Anything).Return(nil).Once()
	taskRepo.On("List", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	store := setupStore(context.Background(), db, taskRepo, new(MockTaskResultRepo), new(MockMessageRepo))

	sqlMock.ExpectBegin()
	sqlMock.ExpectRollback()

	tasks, err := store.GetTasks()
	require.Error(t, err)
	assert.Nil(t, tasks)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_GetTasks_GetByIDsError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	msgRepo := new(MockMessageRepo)

	store := setupStore(context.Background(), db, taskRepo, new(MockTaskResultRepo), msgRepo)

	msgID := uuid.New()
	task := dto.Task{ID: uuid.New(), MessageID: msgID, Status: retrier.StatusPending}
	taskMap := map[uuid.UUID][]dto.Task{msgID: {task}}

	taskRepo.On("ReleaseHungTasks", mock.Anything).Return(nil).Once()
	taskRepo.On("List", mock.Anything, mock.Anything).Return(taskMap, nil)
	msgRepo.On("GetByIDs", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	sqlMock.ExpectBegin()
	sqlMock.ExpectRollback()

	tasks, err := store.GetTasks()
	require.Error(t, err)
	assert.Nil(t, tasks)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_SaveTask_Success(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	resultRepo := new(MockTaskResultRepo)
	store := setupStore(context.Background(), db, taskRepo, resultRepo, new(MockMessageRepo))

	taskID := uuid.New()
	origTask := &dto.Task{ID: taskID, Status: retrier.StatusPending, Retries: 0}
	updatedTask := &dto.Task{ID: taskID, Status: retrier.StatusSuccess, Retries: 1}
	result := &retrier.TaskExecutionResult{
		ID:     uuid.New(),
		TaskID: taskID,
		Status: retrier.StatusSuccess,
		RunAt:  time.Now(),
	}

	taskRepo.On("GetByID", mock.Anything, taskID).Return(origTask, nil)
	taskRepo.On("Update", mock.Anything, mock.MatchedBy(func(t *dto.Task) bool {
		return t.ID == taskID &&
			t.Status == retrier.StatusSuccess &&
			t.Retries == 1
	})).Return(updatedTask, nil)
	resultRepo.On("Create", mock.Anything, mock.MatchedBy(func(r *dto.TaskExecutionResult) bool {
		return r.TaskID == taskID &&
			r.Status == retrier.StatusSuccess &&
			r.ID == result.ID
	})).Return(nil)

	sqlMock.ExpectBegin()
	sqlMock.ExpectCommit()

	err := store.SaveTask(&retrier.Task{
		ID:      taskID,
		Status:  retrier.StatusSuccess,
		Retries: 1,
	}, result)

	require.NoError(t, err)
	taskRepo.AssertExpectations(t)
	resultRepo.AssertExpectations(t)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_SaveTask_GetByIDError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	store := setupStore(context.Background(), db, taskRepo, new(MockTaskResultRepo), new(MockMessageRepo))

	taskID := uuid.New()
	taskRepo.On("GetByID", mock.Anything, taskID).Return(nil, assert.AnError)

	sqlMock.ExpectBegin()
	sqlMock.ExpectRollback()

	err := store.SaveTask(&retrier.Task{ID: taskID}, &retrier.TaskExecutionResult{})
	require.Error(t, err)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_SaveTask_UpdateError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	store := setupStore(context.Background(), db, taskRepo, new(MockTaskResultRepo), new(MockMessageRepo))

	taskID := uuid.New()
	origTask := &dto.Task{ID: taskID}
	taskRepo.On("GetByID", mock.Anything, taskID).Return(origTask, nil)
	taskRepo.On("Update", mock.Anything, mock.Anything).Return(nil, assert.AnError)

	sqlMock.ExpectBegin()
	sqlMock.ExpectRollback()

	err := store.SaveTask(&retrier.Task{ID: taskID}, &retrier.TaskExecutionResult{})
	require.Error(t, err)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestWorkerStore_SaveTask_CreateResultError(t *testing.T) {
	db, sqlMock := newMockDB(t)
	defer db.Close()

	taskRepo := new(MockTaskRepo)
	resultRepo := new(MockTaskResultRepo)
	store := setupStore(context.Background(), db, taskRepo, resultRepo, new(MockMessageRepo))

	taskID := uuid.New()
	origTask := &dto.Task{ID: taskID}
	updatedTask := &dto.Task{ID: taskID}
	taskRepo.On("GetByID", mock.Anything, taskID).Return(origTask, nil)
	taskRepo.On("Update", mock.Anything, mock.Anything).Return(updatedTask, nil)
	resultRepo.On("Create", mock.Anything, mock.Anything).Return(assert.AnError)

	sqlMock.ExpectBegin()
	sqlMock.ExpectRollback()

	err := store.SaveTask(&retrier.Task{ID: taskID}, &retrier.TaskExecutionResult{})
	require.Error(t, err)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
