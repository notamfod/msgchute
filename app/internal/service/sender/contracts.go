package sender

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/devian2011/msgchute/internal/dto"
)

type taskRepo interface {
	GetByID(context.Context, uuid.UUID) (*dto.Task, error)
	List(context.Context, dto.TaskFilter) (map[uuid.UUID][]dto.Task, error)
	Create(context.Context, *dto.Task) (*dto.Task, error)
	Update(context.Context, *dto.Task) (*dto.Task, error)
	Lock(context.Context, []uuid.UUID, time.Time) error
	ReleaseHungTasks(ctx context.Context) error
}

type taskResultRepo interface {
	Create(context.Context, *dto.TaskExecutionResult) error
}

type messageRepo interface {
	GetByID(context.Context, uuid.UUID) (*dto.Message, error)
	GetByIDs(context.Context, []uuid.UUID) ([]*dto.Message, error)
	Create(context.Context, *dto.Message) error
	UpdateStatus(context.Context, uuid.UUID, dto.MessageStatus) error
}
