package sender

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/devian2011/msgchute/internal/dto"
)

type MockTaskRepo struct {
	mock.Mock
}

func (m *MockTaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*dto.Task, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.Task), args.Error(1)
}

func (m *MockTaskRepo) List(ctx context.Context, filter dto.TaskFilter) (map[uuid.UUID][]dto.Task, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[uuid.UUID][]dto.Task), args.Error(1)
}

func (m *MockTaskRepo) Create(ctx context.Context, task *dto.Task) (*dto.Task, error) {
	args := m.Called(ctx, task)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.Task), args.Error(1)
}

func (m *MockTaskRepo) Update(ctx context.Context, task *dto.Task) (*dto.Task, error) {
	args := m.Called(ctx, task)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.Task), args.Error(1)
}

func (m *MockTaskRepo) Lock(ctx context.Context, ids []uuid.UUID, until time.Time) error {
	args := m.Called(ctx, ids)
	return args.Error(0)
}

func (m *MockTaskRepo) ReleaseHungTasks(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

type MockTaskResultRepo struct {
	mock.Mock
}

func (m *MockTaskResultRepo) Create(ctx context.Context, result *dto.TaskExecutionResult) error {
	args := m.Called(ctx, result)
	return args.Error(0)
}

type MockMessageRepo struct {
	mock.Mock
}

func (m *MockMessageRepo) GetByID(ctx context.Context, id uuid.UUID) (*dto.Message, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.Message), args.Error(1)
}

func (m *MockMessageRepo) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*dto.Message, error) {
	args := m.Called(ctx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*dto.Message), args.Error(1)
}

func (m *MockMessageRepo) Create(ctx context.Context, msg *dto.Message) error {
	args := m.Called(ctx, msg)
	return args.Error(0)
}

func (m *MockMessageRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status dto.MessageStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}
