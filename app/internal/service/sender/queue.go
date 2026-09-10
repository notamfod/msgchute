package sender

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
	"github.com/devian2011/msgchute/pkg/generate"
)

var ErrUnknownTransport = errors.New("unknown transport")

type Queue struct {
	templates   TemplateGenerator
	providers   map[string]*ProviderConfig
	ctx         context.Context
	db          *sqlx.DB
	taskRepo    taskRepo
	messageRepo messageRepo
}

func NewQueue(ctx context.Context, db *sqlx.DB, taskRepo taskRepo, messageRepo messageRepo, providers map[string]*ProviderConfig, templates TemplateGenerator) *Queue {
	return &Queue{
		templates:   templates,
		providers:   providers,
		ctx:         ctx,
		db:          db,
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
	}
}

func (s *Queue) Add(message *dto.Message) (*dto.Message, *dto.Task, error) {
	if s.providers[message.Transport] == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownTransport, message.Transport)
	}
	if _, _, err := s.templates.GenerateMessage(message); err != nil {
		return nil, nil, err
	}
	message.ID = generate.ID()
	now := time.Now()

	if message.Retry == nil {
		message.Retry = &dto.Retry{
			Retries:  1,
			Strategy: retrier.JitterLinearBackOff,
			Params: map[retrier.BackOffParam]interface{}{
				retrier.DurationKey: time.Second,
			},
		}
	}

	// If set new schedule
	nextRun := now
	if !message.Schedule.IsZero() {
		nextRun = message.Schedule
	}

	task := &dto.Task{
		ID:            generate.ID(),
		MessageID:     message.ID,
		Worker:        message.Transport,
		Status:        retrier.StatusPending,
		Retries:       0,
		MaxRetries:    message.Retry.Retries,
		BackOffCode:   message.Retry.Strategy,
		BackOffParams: message.Retry.Params,
		Deadline:      message.Deadline,
		IsProcessed:   false,

		LockUntil: time.Time{},
		CreatedAt: now,
		LastRun:   time.Time{},
		NextRun:   nextRun,
	}

	if !message.Deadline.IsZero() {
		task.Deadline = message.Deadline
	}

	getErr := storage.InTransaction(context.TODO(), s.db, func(ctx context.Context) error {
		messageCreateErr := s.messageRepo.Create(ctx, message)
		if messageCreateErr != nil {
			slog.Error("Failed to create message", "error", messageCreateErr)
			return messageCreateErr
		}

		_, taskCreateErr := s.taskRepo.Create(ctx, task)
		if taskCreateErr != nil {
			slog.Error("Failed to create task", "error", taskCreateErr)
			return taskCreateErr
		}

		return nil
	})

	if getErr != nil {
		slog.Error("Failed to add message to queue", "error", getErr.Error())
		return nil, nil, getErr
	}

	return message, task, nil
}

// Retry send repeat action for message
func (s *Queue) Retry(mrr *dto.MessageRetryRequest) (*dto.Message, *dto.Task, error) {
	msg, err := s.messageRepo.GetByID(s.ctx, mrr.ID)
	if err != nil {
		return nil, nil, err
	}
	if msg == nil {
		return nil, nil, errors.New("message not found")
	}
	if s.providers[msg.Transport] == nil {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnknownTransport, msg.Transport)
	}
	if _, _, err := s.templates.GenerateMessage(msg); err != nil {
		return nil, nil, err
	}
	var task *dto.Task
	err = storage.InTransaction(context.Background(), s.db, func(ctx context.Context) error {
		now := time.Now()
		// Serialize retries for this message before checking existing tasks.
		locked, err := s.messageRepo.GetByID(ctx, mrr.ID)
		if err != nil {
			return err
		}
		if locked == nil {
			return errors.New("message not found")
		}

		taskMap, selectErr := s.taskRepo.List(ctx, dto.TaskFilter{
			MessageIDs: []uuid.UUID{msg.ID},
		})
		if selectErr != nil {
			return selectErr
		}
		// Check that all tasks is finished
		isFinished := true
		if len(taskMap[msg.ID]) > 0 {
			for _, t := range taskMap[msg.ID] {
				if !t.IsFinished() {
					isFinished = false
					break
				}
			}
		}

		if !isFinished {
			return errors.New("all task not finished, make retry after all tasks will be closed")
		}

		// If set new retry policy
		msgRetry := msg.Retry
		if mrr.Retry != nil {
			msgRetry = mrr.Retry
		}
		// If msg.Retry is nil, set default
		if msgRetry == nil {
			msgRetry = &dto.Retry{
				Retries:  1,
				Strategy: retrier.JitterLinearBackOff,
				Params: map[retrier.BackOffParam]interface{}{
					retrier.DurationKey: time.Second,
				},
			}
		}

		// If set new schedule
		nextRun := now
		if !mrr.Schedule.IsZero() {
			nextRun = mrr.Schedule
		}

		task = &dto.Task{
			ID:            generate.ID(),
			MessageID:     msg.ID,
			Worker:        msg.Transport,
			Status:        retrier.StatusPending,
			Retries:       0,
			MaxRetries:    msgRetry.Retries,
			BackOffCode:   msgRetry.Strategy,
			BackOffParams: msgRetry.Params,
			Deadline:      mrr.Deadline,
			IsProcessed:   false,

			LockUntil: time.Time{},
			CreatedAt: now,
			LastRun:   time.Time{},
			NextRun:   nextRun,
		}

		_, taskCreateErr := s.taskRepo.Create(ctx, task)
		if taskCreateErr != nil {
			slog.Error("Failed to create task", "error", taskCreateErr)
			return taskCreateErr
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}
	return msg, task, nil
}
