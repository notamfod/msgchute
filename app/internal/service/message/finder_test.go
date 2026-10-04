package message

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
)

func contains(ids []uuid.UUID, id uuid.UUID) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func TestFinder_Find(t *testing.T) {
	msgRepo := new(MockMessageRepo)
	taskRepo := new(MockTaskRepo)
	taskResultRepo := new(MockTaskResultRepo)

	db := &sqlx.DB{}
	finder := NewFinder(db, msgRepo, taskRepo, taskResultRepo)

	ctx := context.Background()
	filter := &dto.MessageFilter{Limit: 10}

	t.Run("success", func(t *testing.T) {
		msgID1 := uuid.New()
		msgID2 := uuid.New()
		msgs := []*dto.Message{
			{ID: msgID1, Subject: "subj1"},
			{ID: msgID2, Subject: "subj2"},
		}
		total := uint64(2)

		task1 := dto.Task{ID: uuid.New(), MessageID: msgID1}
		task2 := dto.Task{ID: uuid.New(), MessageID: msgID2}
		tasksMap := map[uuid.UUID][]dto.Task{
			msgID1: {task1},
			msgID2: {task2},
		}

		taskResult1 := dto.TaskExecutionResult{ID: uuid.New(), TaskID: task1.ID}
		taskResult2 := dto.TaskExecutionResult{ID: uuid.New(), TaskID: task2.ID}
		taskResultsMap := map[uuid.UUID][]dto.TaskExecutionResult{
			task1.ID: {taskResult1},
			task2.ID: {taskResult2},
		}

		msgRepo.On("Find", ctx, filter).Return(msgs, total, nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, []uuid.UUID{msgID1, msgID2}).Return(tasksMap, nil).Once()
		taskResultRepo.On("GetByTaskIDs", ctx, mock.MatchedBy(func(ids []uuid.UUID) bool {
			if len(ids) != 2 {
				return false
			}
			return contains(ids, task1.ID) && contains(ids, task2.ID)
		})).Return(taskResultsMap, nil).Once()

		result, pages, err := finder.Find(filter)
		require.NoError(t, err)
		assert.Equal(t, int(total), pages)
		assert.Len(t, result, 2)
		assert.Equal(t, msgID1, result[0].Message.ID)
		assert.Len(t, result[0].Tasks, 1)
		assert.Equal(t, task1.ID, result[0].Tasks[0].Task.ID)
		assert.Len(t, result[0].Tasks[0].Results, 1)

		msgRepo.AssertExpectations(t)
		taskRepo.AssertExpectations(t)
		taskResultRepo.AssertExpectations(t)
	})

	t.Run("no messages", func(t *testing.T) {
		msgRepo.On("Find", ctx, filter).Return([]*dto.Message{}, uint64(0), nil).Once()
		result, pages, err := finder.Find(filter)
		require.NoError(t, err)
		assert.Equal(t, 0, pages)
		assert.Empty(t, result)
		msgRepo.AssertExpectations(t)
		// taskRepo и taskResultRepo не вызываются
	})

	t.Run("messageRepo error", func(t *testing.T) {
		msgRepo.On("Find", ctx, filter).Return(nil, uint64(0), errors.New("msg error")).Once()
		result, pages, err := finder.Find(filter)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, pages)
	})

	t.Run("taskRepo error", func(t *testing.T) {
		msgRepo.On("Find", ctx, filter).Return([]*dto.Message{{ID: uuid.New()}}, uint64(1), nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, mock.Anything).Return(nil, errors.New("task error")).Once()
		result, pages, err := finder.Find(filter)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, pages)
	})

	t.Run("taskResultRepo error", func(t *testing.T) {
		msgID := uuid.New()
		task := dto.Task{ID: uuid.New(), MessageID: msgID}
		tasksMap := map[uuid.UUID][]dto.Task{msgID: {task}}
		msgRepo.On("Find", ctx, filter).Return([]*dto.Message{{ID: msgID}}, uint64(1), nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, []uuid.UUID{msgID}).Return(tasksMap, nil).Once()
		taskResultRepo.On("GetByTaskIDs", ctx, []uuid.UUID{task.ID}).Return(nil, errors.New("result error")).Once()
		result, pages, err := finder.Find(filter)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, pages)
	})
}

func TestFinder_FindByID(t *testing.T) {
	msgRepo := new(MockMessageRepo)
	taskRepo := new(MockTaskRepo)
	taskResultRepo := new(MockTaskResultRepo)

	db := &sqlx.DB{}
	finder := NewFinder(db, msgRepo, taskRepo, taskResultRepo)

	ctx := context.Background()
	msgID := uuid.New()

	t.Run("success", func(t *testing.T) {
		msg := &dto.Message{ID: msgID, Subject: "test"}
		task1 := dto.Task{ID: uuid.New(), MessageID: msgID}
		task2 := dto.Task{ID: uuid.New(), MessageID: msgID}
		tasksMap := map[uuid.UUID][]dto.Task{
			msgID: {task1, task2},
		}
		taskResult1 := dto.TaskExecutionResult{ID: uuid.New(), TaskID: task1.ID}
		taskResult2 := dto.TaskExecutionResult{ID: uuid.New(), TaskID: task2.ID}
		taskResultsMap := map[uuid.UUID][]dto.TaskExecutionResult{
			task1.ID: {taskResult1},
			task2.ID: {taskResult2},
		}

		msgRepo.On("GetByID", ctx, msgID).Return(msg, nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, []uuid.UUID{msgID}).Return(tasksMap, nil).Once()
		taskResultRepo.On("GetByTaskIDs", ctx, mock.MatchedBy(func(ids []uuid.UUID) bool {
			if len(ids) != 2 {
				return false
			}
			return contains(ids, task1.ID) && contains(ids, task2.ID)
		})).Return(taskResultsMap, nil).Once()

		fullMsg, err := finder.FindByID(msgID)
		require.NoError(t, err)
		assert.Equal(t, msgID, fullMsg.Message.ID)
		assert.Len(t, fullMsg.Tasks, 2)
		assert.Equal(t, task1.ID, fullMsg.Tasks[0].Task.ID)
		assert.Len(t, fullMsg.Tasks[0].Results, 1)

		msgRepo.AssertExpectations(t)
		taskRepo.AssertExpectations(t)
		taskResultRepo.AssertExpectations(t)
	})

	t.Run("message not found", func(t *testing.T) {
		msgRepo.On("GetByID", ctx, msgID).Return(nil, errors.New("not found")).Once()
		fullMsg, err := finder.FindByID(msgID)
		assert.Error(t, err)
		assert.Nil(t, fullMsg)
	})

	t.Run("taskRepo error", func(t *testing.T) {
		msg := &dto.Message{ID: msgID}
		msgRepo.On("GetByID", ctx, msgID).Return(msg, nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, []uuid.UUID{msgID}).Return(nil, errors.New("task error")).Once()
		fullMsg, err := finder.FindByID(msgID)
		assert.Error(t, err)
		assert.Nil(t, fullMsg)
	})

	t.Run("taskResultRepo error", func(t *testing.T) {
		msg := &dto.Message{ID: msgID}
		task := dto.Task{ID: uuid.New(), MessageID: msgID}
		tasksMap := map[uuid.UUID][]dto.Task{msgID: {task}}
		msgRepo.On("GetByID", ctx, msgID).Return(msg, nil).Once()
		taskRepo.On("GetByMessageIDs", ctx, []uuid.UUID{msgID}).Return(tasksMap, nil).Once()
		taskResultRepo.On("GetByTaskIDs", ctx, []uuid.UUID{task.ID}).Return(nil, errors.New("result error")).Once()
		fullMsg, err := finder.FindByID(msgID)
		assert.Error(t, err)
		assert.Nil(t, fullMsg)
	})
}

func TestFinder_FindByID_MissingRecord(t *testing.T) {
	for _, id := range []uuid.UUID{uuid.Nil, uuid.New()} {
		t.Run(id.String(), func(t *testing.T) {
			msgRepo := new(MockMessageRepo)
			msgRepo.On("GetByID", mock.Anything, id).Return(nil, nil).Once()
			finder := NewFinder(nil, msgRepo, nil, nil)

			result, err := finder.FindByID(id)

			require.NoError(t, err)
			assert.Nil(t, result)
			msgRepo.AssertExpectations(t)
		})
	}
}

func TestFinder_GetSenders(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		expectedSenders := []string{"sender1", "sender2"}

		// Настраиваем мок: ожидаем context.Background() и возвращаем данные без ошибки
		mockRepo.On("GetSenders", mock.Anything).Return(expectedSenders, nil)

		result, err := finder.GetSenders(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedSenders, result)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error from repository", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		expectedErr := errors.New("database connection failed")

		// Настраиваем мок на возврат ошибки
		mockRepo.On("GetSenders", mock.Anything).Return(nil, expectedErr)

		result, err := finder.GetSenders(context.Background())

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestFinder_GetTransports(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		expectedTransports := []string{"email", "sms", "telegram"}

		mockRepo.On("GetTransports", mock.Anything).Return(expectedTransports, nil)

		result, err := finder.GetTransports(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedTransports, result)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error from repository", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		expectedErr := errors.New("repository error")

		mockRepo.On("GetTransports", mock.Anything).Return(nil, expectedErr)

		result, err := finder.GetTransports(context.Background())

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestFinder_GetTemplates(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		ctx := context.WithValue(context.Background(), "test-key", "test-value")
		expectedTemplates := []string{"welcome_email", "password_reset", "order_confirmation"}

		// Проверяем, что передается именно наш контекст ctx
		mockRepo.On("GetTemplateCodes", ctx).Return(expectedTemplates, nil)

		result, err := finder.GetTemplates(ctx)

		assert.NoError(t, err)
		assert.Equal(t, expectedTemplates, result)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error from repository", func(t *testing.T) {
		mockRepo := new(MockMessageRepo)
		finder := &Finder{msgRepo: mockRepo}

		ctx := context.Background()
		expectedErr := errors.New("failed to fetch templates")

		mockRepo.On("GetTemplateCodes", ctx).Return(nil, expectedErr)

		result, err := finder.GetTemplates(ctx)

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, result)
		mockRepo.AssertExpectations(t)
	})
}

func TestFinder_GetRecipients(t *testing.T) {
	tests := []struct {
		name      string
		search    string
		mockSetup func(mockRepo *MockMessageRepo)
		want      []string
		wantErr   bool
	}{
		{
			name:   "success with search substring",
			search: "example",
			mockSetup: func(mockRepo *MockMessageRepo) {
				mockRepo.On("GetRecipients", mock.Anything, "example").
					Return([]string{"user@example.com", "admin@example.org"}, nil)
			},
			want:    []string{"user@example.com", "admin@example.org"},
			wantErr: false,
		},
		{
			name:   "success with empty search (all recipients)",
			search: "",
			mockSetup: func(mockRepo *MockMessageRepo) {
				mockRepo.On("GetRecipients", mock.Anything, "").
					Return([]string{"alice@mail.com", "bob@mail.com", "+79001234567"}, nil)
			},
			want:    []string{"alice@mail.com", "bob@mail.com", "+79001234567"},
			wantErr: false,
		},
		{
			name:   "no matching recipients returns empty slice",
			search: "nonexistent",
			mockSetup: func(mockRepo *MockMessageRepo) {
				mockRepo.On("GetRecipients", mock.Anything, "nonexistent").
					Return([]string{}, nil)
			},
			want:    []string{},
			wantErr: false,
		},
		{
			name:   "repository returns error",
			search: "test",
			mockSetup: func(mockRepo *MockMessageRepo) {
				mockRepo.On("GetRecipients", mock.Anything, "test").
					Return(nil, assert.AnError) // или конкретная ошибка
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockMessageRepo)
			tt.mockSetup(mockRepo)

			finder := &Finder{
				msgRepo: mockRepo, // поле, в котором хранится репозиторий
			}
			ctx := context.Background()

			got, err := finder.GetRecipients(ctx, tt.search)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
