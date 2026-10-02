package message

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/devian2011/msgchute/internal/dto"
)

type onboardingStatusStub struct {
	aggregateCalls int
}

func (s *onboardingStatusStub) IsOnboardingTask(context.Context, uuid.UUID) (bool, error) {
	s.aggregateCalls++
	return true, nil
}

func TestStatusUpdater_UpdateStatusByTaskID(t *testing.T) {
	tests := []struct {
		name          string
		taskID        uuid.UUID
		newStatus     dto.MessageStatus
		taskExists    bool
		taskErr       error
		msgExists     bool
		msgStatus     dto.MessageStatus
		msgErr        error
		updateErr     error
		expectedError bool
	}{
		{
			name:          "successful update",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusSucceeded,
			taskExists:    true,
			msgExists:     true,
			msgStatus:     dto.MessageStatusRunning,
			updateErr:     nil,
			expectedError: false,
		},
		{
			name:          "status already matches",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusSucceeded,
			taskExists:    true,
			msgExists:     true,
			msgStatus:     dto.MessageStatusSucceeded,
			updateErr:     nil,
			expectedError: false,
		},
		{
			name:          "message already succeeded",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusFailed,
			taskExists:    true,
			msgExists:     true,
			msgStatus:     dto.MessageStatusSucceeded,
			updateErr:     nil,
			expectedError: false,
		},
		{
			name:          "task not found",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusSucceeded,
			taskExists:    false,
			taskErr:       errors.New("task not found"),
			expectedError: true,
		},
		{
			name:          "message not found",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusSucceeded,
			taskExists:    true,
			msgExists:     false,
			msgErr:        errors.New("message not found"),
			expectedError: true,
		},
		{
			name:          "update error",
			taskID:        uuid.New(),
			newStatus:     dto.MessageStatusSucceeded,
			taskExists:    true,
			msgExists:     true,
			msgStatus:     dto.MessageStatusRunning,
			updateErr:     errors.New("update failed"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, sqlMock, err := sqlmock.New()
			assert.NoError(t, err)
			defer db.Close()

			sqlxDB := sqlx.NewDb(db, "postgres")

			sqlMock.ExpectBegin()

			taskRepo := new(MockTaskRepo)
			msgRepo := new(MockMessageRepo)

			updater := NewStatusUpdater(sqlxDB, msgRepo, taskRepo)

			if !tt.taskExists {
				taskRepo.On("GetByID", mock.Anything, tt.taskID).
					Return(nil, tt.taskErr).Once()
				sqlMock.ExpectRollback()
			} else {
				task := &dto.Task{
					ID:        tt.taskID,
					MessageID: uuid.New(),
					Status:    retrier.StatusPending,
				}
				taskRepo.On("GetByID", mock.Anything, tt.taskID).
					Return(task, nil).Once()

				taskRepo.On("Unlock", mock.Anything, []uuid.UUID{task.ID}).
					Return(nil).Once()

				if !tt.msgExists {
					msgRepo.On("GetByID", mock.Anything, task.MessageID).
						Return(nil, tt.msgErr).Once()
					sqlMock.ExpectRollback()
				} else {
					msg := &dto.Message{
						ID:     task.MessageID,
						Status: tt.msgStatus,
					}
					msgRepo.On("GetByID", mock.Anything, task.MessageID).
						Return(msg, nil).Once()

					if tt.msgStatus != tt.newStatus && tt.msgStatus != dto.MessageStatusSucceeded {
						msgRepo.On("UpdateStatus", mock.Anything, msg.ID, tt.newStatus).
							Return(tt.updateErr).Once()
						if tt.updateErr != nil {
							sqlMock.ExpectRollback()
						} else {
							sqlMock.ExpectCommit()
						}
					} else {
						sqlMock.ExpectCommit()
					}
				}
			}

			err = updater.UpdateStatusByTaskID(tt.taskID, tt.newStatus)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			taskRepo.AssertExpectations(t)
			msgRepo.AssertExpectations(t)
			assert.NoError(t, sqlMock.ExpectationsWereMet())
		})
	}
}

func TestOnboardingStatusNeverDowngradesSucceededMessage(t *testing.T) {
	db, sqlMock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "postgres")
	taskRepo := new(MockTaskRepo)
	msgRepo := new(MockMessageRepo)
	onboarding := &onboardingStatusStub{}
	taskID := uuid.New()
	messageID := uuid.New()
	taskRepo.On("GetByID", mock.Anything, taskID).Return(&dto.Task{ID: taskID, MessageID: messageID}, nil).Once()
	taskRepo.On("Unlock", mock.Anything, []uuid.UUID{taskID}).Return(nil).Once()
	sqlMock.ExpectBegin()
	sqlMock.ExpectCommit()

	err = NewStatusUpdater(sqlxDB, msgRepo, taskRepo, onboarding).UpdateStatusByTaskID(taskID, dto.MessageStatusFailed)
	assert.NoError(t, err)
	assert.Equal(t, 1, onboarding.aggregateCalls)
	msgRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}
