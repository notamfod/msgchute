package sender

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/devian2011/msgchute/internal/dto"
)

type queueTestGenerator struct{}

func (queueTestGenerator) GenerateMessage(*dto.Message) (string, string, error) {
	return "", "test", nil
}

func TestQueueValidateTaggedMessage(t *testing.T) {
	queue := NewQueue(context.Background(), nil, nil, nil, map[string]*ProviderConfig{
		"mail":    {Provider: "smtp"},
		"unknown": {Provider: "custom"},
	}, queueTestGenerator{})
	if err := queue.Validate(&dto.Message{Transport: "mail", Tag: "order"}); err != nil {
		t.Fatal(err)
	}
	if err := queue.Validate(&dto.Message{Transport: "mail", Tag: "other"}); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := queue.Validate(&dto.Message{Transport: "unknown", Tag: "order"}); !errors.Is(err, ErrUnknownPreferenceChannel) {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := queue.Validate(&dto.Message{Transport: "missing", Tag: "order"}); !errors.Is(err, ErrUnknownTransport) {
		t.Fatalf("Validate() error = %v", err)
	}
	if err := queue.Validate(&dto.Message{Transport: "mail", BodySource: "rendered"}); !errors.Is(err, ErrInvalidBodySource) {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestQueue_Add(t *testing.T) {
	tests := []struct {
		name        string
		message     *dto.Message
		mockTaskErr error
		mockMsgErr  error
		expectedErr bool
		checkTaskFn func(t *testing.T, task *dto.Task)
		checkMsgFn  func(t *testing.T, msg *dto.Message)
	}{
		{
			name: "success with retry",
			message: &dto.Message{
				Transport: "email",
				Retry: &dto.Retry{
					Retries:  3,
					Strategy: retrier.ExponentialBackOff,
					Params: map[retrier.BackOffParam]interface{}{
						retrier.BaseDelayKey: 5 * time.Second,
					},
				},
				Deadline: time.Now().Add(time.Hour),
			},
			expectedErr: false,
			checkTaskFn: func(t *testing.T, task *dto.Task) {
				assert.Equal(t, retrier.StatusPending, task.Status)
				assert.Equal(t, 3, task.MaxRetries)
				assert.Equal(t, retrier.ExponentialBackOff, task.BackOffCode)
				expectedParams := dto.BackOffParams{
					retrier.BaseDelayKey: 5 * time.Second,
				}
				assert.Equal(t, expectedParams, task.BackOffParams)
				assert.NotZero(t, task.Deadline)
				assert.Equal(t, "email", task.Worker)
			},
			checkMsgFn: func(t *testing.T, msg *dto.Message) {
				assert.Equal(t, "email", msg.Transport)
				assert.NotZero(t, msg.ID)
			},
		},
		{
			name: "success with nil retry (defaults)",
			message: &dto.Message{
				Transport: "sms",
				Retry:     nil,
				Deadline:  time.Time{},
			},
			expectedErr: false,
			checkTaskFn: func(t *testing.T, task *dto.Task) {
				assert.Equal(t, 1, task.MaxRetries)
				assert.Equal(t, retrier.JitterLinearBackOff, task.BackOffCode)
				expectedParams := dto.BackOffParams{
					retrier.DurationKey: time.Second,
				}
				assert.Equal(t, expectedParams, task.BackOffParams)
				assert.True(t, task.Deadline.IsZero())
			},
			checkMsgFn: func(t *testing.T, msg *dto.Message) {
				assert.Equal(t, "sms", msg.Transport)
			},
		},
		{
			name: "task creation error",
			message: &dto.Message{
				Transport: "push",
				Retry:     &dto.Retry{Retries: 1, Strategy: "fixed"},
			},
			mockTaskErr: errors.New("task db error"),
			expectedErr: true,
		},
		{
			name: "message creation error",
			message: &dto.Message{
				Transport: "email",
				Retry:     &dto.Retry{Retries: 2, Strategy: "linear"},
			},
			mockMsgErr:  errors.New("message db error"),
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			sqlMock.ExpectBegin()

			mockTaskRepo := new(MockTaskRepo)
			mockMsgRepo := new(MockMessageRepo)

			queue := NewQueue(context.Background(), sqlxDB, mockTaskRepo, mockMsgRepo, map[string]*ProviderConfig{"email": {}, "sms": {}, "push": {}}, queueTestGenerator{})

			switch {
			case tt.mockMsgErr != nil:
				mockMsgRepo.On("Create", mock.Anything, mock.Anything).Return(tt.mockMsgErr).Once()
				sqlMock.ExpectRollback()
			case tt.mockTaskErr != nil:
				mockMsgRepo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
				mockTaskRepo.On("Create", mock.Anything, mock.Anything).Return(nil, tt.mockTaskErr).Once()
				sqlMock.ExpectRollback()
			default:
				mockMsgRepo.On("Create", mock.Anything, mock.Anything).Return(nil).Once()
				mockTaskRepo.On("Create", mock.Anything, mock.Anything).Return(nil, nil).Once()
				sqlMock.ExpectCommit()
			}

			msg, task, err := queue.Add(tt.message)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, msg)
				assert.Nil(t, task)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, msg)
				assert.NotNil(t, task)

				assert.NotZero(t, msg.ID)
				assert.NotZero(t, task.ID)
				assert.Equal(t, msg.ID, task.MessageID)

				assert.Equal(t, tt.message.Transport, task.Worker)
				assert.Equal(t, retrier.StatusPending, task.Status)
				assert.Zero(t, task.LastRun)
				assert.NotZero(t, task.CreatedAt)
				assert.NotZero(t, task.NextRun)

				if tt.checkTaskFn != nil {
					tt.checkTaskFn(t, task)
				}
				if tt.checkMsgFn != nil {
					tt.checkMsgFn(t, msg)
				}
			}

			mockTaskRepo.AssertExpectations(t)
			mockMsgRepo.AssertExpectations(t)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestQueueRejectsUnknownTransportBeforePersistence(t *testing.T) {
	queue := NewQueue(context.Background(), nil, nil, nil, map[string]*ProviderConfig{"beeline": {}}, queueTestGenerator{})
	for _, transport := range []string{"ya-mail", "yamail"} {
		t.Run(transport, func(t *testing.T) {
			message := &dto.Message{Transport: transport}
			_, _, err := queue.Add(message)
			assert.ErrorIs(t, err, ErrUnknownTransport)
			assert.Equal(t, uuid.Nil, message.ID)
		})
	}
}

func TestQueue_Retry(t *testing.T) {
	fixedMsgID1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fixedMsgID2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	fixedMsgID3 := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	fixedMsgID4 := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	fixedMsgID5 := uuid.MustParse("55555555-5555-5555-5555-555555555555")

	tests := []struct {
		name          string
		request       *dto.MessageRetryRequest
		mockMsg       *dto.Message
		mockMsgErr    error
		mockTaskMap   map[uuid.UUID][]dto.Task
		mockTaskErr   error
		mockCreateErr error
		expectedErr   bool
		checkTaskFn   func(t *testing.T, task *dto.Task)
	}{
		{
			name: "successful retry with all tasks finished",
			request: &dto.MessageRetryRequest{
				ID:       fixedMsgID1,
				Schedule: time.Now().Add(5 * time.Minute),
				Deadline: time.Now().Add(2 * time.Hour),
				Retry: &dto.Retry{
					Retries:  5,
					Strategy: "linear",
					Params: map[retrier.BackOffParam]interface{}{
						retrier.DurationKey: 10 * time.Second,
					},
				},
			},
			mockMsg: &dto.Message{
				ID:        fixedMsgID1,
				Transport: "email",
				Retry: &dto.Retry{
					Retries:  3,
					Strategy: retrier.ExponentialBackOff,
				},
			},
			mockMsgErr: nil,
			mockTaskMap: map[uuid.UUID][]dto.Task{
				fixedMsgID1: {
					{Status: retrier.StatusSuccess, IsProcessed: true},
					{Status: retrier.StatusFailure, IsProcessed: true},
				},
			},
			mockTaskErr:   nil,
			mockCreateErr: nil,
			expectedErr:   false,
			checkTaskFn: func(t *testing.T, task *dto.Task) {
				assert.Equal(t, retrier.StatusPending, task.Status)
				assert.Equal(t, 5, task.MaxRetries)
				assert.Equal(t, "linear", task.BackOffCode)
				expectedParams := dto.BackOffParams{
					retrier.DurationKey: 10 * time.Second,
				}
				assert.Equal(t, expectedParams, task.BackOffParams)
				assert.NotZero(t, task.Deadline)
				assert.False(t, task.NextRun.IsZero())
				assert.True(t, task.NextRun.After(time.Now()))
			},
		},
		{
			name: "successful retry with default schedule (now)",
			request: &dto.MessageRetryRequest{
				ID:       fixedMsgID2,
				Schedule: time.Time{},
				Retry:    nil,
			},
			mockMsg: &dto.Message{
				ID:        fixedMsgID2,
				Transport: "sms",
				Retry: &dto.Retry{
					Retries:  2,
					Strategy: retrier.LinearBackOff,
					Params: map[retrier.BackOffParam]interface{}{
						retrier.DurationKey: 1 * time.Second,
					},
				},
			},
			mockMsgErr: nil,
			mockTaskMap: map[uuid.UUID][]dto.Task{
				fixedMsgID2: {
					{Status: retrier.StatusSuccess, IsProcessed: true},
				},
			},
			mockTaskErr:   nil,
			mockCreateErr: nil,
			expectedErr:   false,
			checkTaskFn: func(t *testing.T, task *dto.Task) {
				assert.Equal(t, 2, task.MaxRetries)
				assert.Equal(t, retrier.LinearBackOff, task.BackOffCode)
				assert.True(t, task.Deadline.IsZero())
				assert.WithinDuration(t, time.Now(), task.NextRun, 2*time.Second)
			},
		},
		{
			name: "error getting message",
			request: &dto.MessageRetryRequest{
				ID: fixedMsgID3,
			},
			mockMsg:     nil,
			mockMsgErr:  errors.New("message not found"),
			mockTaskMap: nil,
			mockTaskErr: nil,
			expectedErr: true,
		},
		{
			name: "error listing tasks",
			request: &dto.MessageRetryRequest{
				ID: fixedMsgID4,
			},
			mockMsg: &dto.Message{
				ID:        fixedMsgID4,
				Transport: "push",
			},
			mockMsgErr:  nil,
			mockTaskMap: nil,
			mockTaskErr: errors.New("db error"),
			expectedErr: true,
		},
		{
			name: "not all tasks finished",
			request: &dto.MessageRetryRequest{
				ID: fixedMsgID5,
			},
			mockMsg: &dto.Message{
				ID:        fixedMsgID5,
				Transport: "email",
				Retry: &dto.Retry{
					Retries: 1,
				},
			},
			mockMsgErr: nil,
			mockTaskMap: map[uuid.UUID][]dto.Task{
				fixedMsgID5: {
					{Status: retrier.StatusPending, IsProcessed: false, MaxRetries: 1, Retries: 0},
					{Status: retrier.StatusFailure, IsProcessed: true, MaxRetries: 1, Retries: 1},
				},
			},
			mockTaskErr:   nil,
			mockCreateErr: nil,
			expectedErr:   true,
		},
		{
			name: "error creating new task",
			request: &dto.MessageRetryRequest{
				ID: fixedMsgID1,
			},
			mockMsg: &dto.Message{
				ID:        fixedMsgID1,
				Transport: "email",
				Retry: &dto.Retry{
					Retries: 1,
				},
			},
			mockMsgErr: nil,
			mockTaskMap: map[uuid.UUID][]dto.Task{
				fixedMsgID1: {
					{Status: retrier.StatusFailure, IsProcessed: true, MaxRetries: 1, Retries: 1},
				},
			},
			mockTaskErr:   nil,
			mockCreateErr: errors.New("create task error"),
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")
			if tt.mockMsgErr == nil {
				sqlMock.ExpectBegin()
			}

			mockTaskRepo := new(MockTaskRepo)
			mockMsgRepo := new(MockMessageRepo)

			queue := NewQueue(context.Background(), sqlxDB, mockTaskRepo, mockMsgRepo, map[string]*ProviderConfig{"email": {}, "sms": {}, "push": {}}, queueTestGenerator{})

			lookupCount := 1
			if tt.mockMsgErr == nil {
				lookupCount = 2
			}
			mockMsgRepo.On("GetByID", mock.Anything, tt.request.ID).Return(tt.mockMsg, tt.mockMsgErr).Times(lookupCount)

			if tt.mockMsgErr == nil {
				mockTaskRepo.On("List", mock.Anything, mock.Anything).
					Return(tt.mockTaskMap, tt.mockTaskErr).Once()

				if tt.mockTaskErr != nil {
					sqlMock.ExpectRollback()
				} else {
					allFinished := true
					if tt.mockTaskMap != nil {
						if tasks, ok := tt.mockTaskMap[tt.mockMsg.ID]; ok {
							for _, tsk := range tasks {
								if !tsk.IsFinished() {
									allFinished = false
									break
								}
							}
						}
					}

					if !allFinished {
						sqlMock.ExpectRollback()
					} else if tt.mockCreateErr != nil {
						mockTaskRepo.On("Create", mock.Anything, mock.AnythingOfType("*dto.Task")).
							Return(nil, tt.mockCreateErr).Once()
						sqlMock.ExpectRollback()
					} else {
						mockTaskRepo.On("Create", mock.Anything, mock.AnythingOfType("*dto.Task")).
							Return(&dto.Task{}, nil).Once()
						sqlMock.ExpectCommit()
					}
				}
			}

			msg, task, err := queue.Retry(tt.request)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Nil(t, msg)
				assert.Nil(t, task)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, msg)
				assert.NotNil(t, task)
				assert.Equal(t, tt.mockMsg.ID, task.MessageID)
				assert.Equal(t, tt.mockMsg.Transport, task.Worker)
				assert.Equal(t, retrier.StatusPending, task.Status)

				if tt.checkTaskFn != nil {
					tt.checkTaskFn(t, task)
				}
			}

			mockTaskRepo.AssertExpectations(t)
			mockMsgRepo.AssertExpectations(t)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestQueueRetryValidatesPersistedTaggedMessage(t *testing.T) {
	messageRepo := new(MockMessageRepo)
	message := &dto.Message{ID: uuid.New(), Transport: "custom", Tag: "order"}
	messageRepo.On("GetByID", mock.Anything, message.ID).Return(message, nil).Once()
	queue := NewQueue(context.Background(), nil, new(MockTaskRepo), messageRepo, map[string]*ProviderConfig{"custom": {Provider: "custom"}}, queueTestGenerator{})
	_, _, err := queue.Retry(&dto.MessageRetryRequest{ID: message.ID})
	if !errors.Is(err, ErrUnknownPreferenceChannel) {
		t.Fatalf("Retry() error = %v", err)
	}
	messageRepo.AssertExpectations(t)
}
