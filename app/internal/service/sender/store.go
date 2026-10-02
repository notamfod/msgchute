package sender

import (
	"context"
	"log/slog"
	"time"

	"github.com/bytedance/sonic"
	"github.com/devian2011/retrier"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

type WorkerStore struct {
	ctx            context.Context
	db             *sqlx.DB
	taskResultRepo taskResultRepo
	taskRepo       taskRepo
	messageRepo    messageRepo
	onboarding     *OnboardingStore
}

func NewWorkerStore(
	ctx context.Context,
	db *sqlx.DB,
	taskResultRepo taskResultRepo,
	taskRepo taskRepo,
	messageRepo messageRepo,
	onboarding ...*OnboardingStore,
) *WorkerStore {
	store := &WorkerStore{
		ctx:            ctx,
		db:             db,
		taskResultRepo: taskResultRepo,
		taskRepo:       taskRepo,
		messageRepo:    messageRepo,
	}
	if len(onboarding) > 0 {
		store.onboarding = onboarding[0]
	}
	return store
}

func (s *WorkerStore) GetTasks() ([]retrier.Task, error) {
	now := time.Now()
	result := make([]retrier.Task, 0)

	getErr := storage.InTransaction(context.TODO(), s.db, func(ctx context.Context) error {
		// TODO: May be release in other goroutine
		if releasedErr := s.taskRepo.ReleaseHungTasks(ctx); releasedErr != nil {
			slog.Error("failed to release hung tasks", "error", releasedErr)
		}
		storeTasks, err := s.taskRepo.List(ctx, dto.TaskFilter{
			Statuses:      []retrier.TaskStatus{retrier.StatusPending},
			NextRunBefore: &now,
			IsProcessed:   new(bool),
			Limit:         100, // TODO: Move to config
		})
		if err != nil {
			return err
		}

		if len(storeTasks) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, 0, len(storeTasks))
		for messageID := range storeTasks {
			ids = append(ids, messageID)
		}

		messages, err := s.messageRepo.GetByIDs(ctx, ids)
		if err != nil {
			return err
		}

		taskIDs := make([]uuid.UUID, 0)

		for i := range messages {
			tasks, exists := storeTasks[messages[i].ID]
			if !exists {
				slog.Error("task not found for message",
					slog.String("message_id", messages[i].ID.String()))
				continue
			}

			for _, task := range tasks {
				message := messages[i]
				worker := task.Worker
				if s.onboarding != nil {
					prepared, preparedWorker, ready, prepareErr := s.onboarding.Prepare(ctx, &task, message, now)
					if prepareErr != nil {
						return prepareErr
					}
					if !ready {
						continue
					}
					message = prepared
					worker = preparedWorker
				}
				payload, pErr := sonic.Marshal(message)
				if pErr != nil {
					slog.Error("marshal message error",
						slog.String("message_id", messages[i].ID.String()),
						slog.Any("error", pErr))
					continue
				}
				taskIDs = append(taskIDs, task.ID)
				result = append(result, retrier.Task{
					ID:            task.ID,
					Ctx:           s.ctx,
					Payload:       payload,
					Worker:        worker,
					Status:        task.Status,
					Retries:       task.Retries,
					MaxRetries:    task.MaxRetries,
					BackOffCode:   task.BackOffCode,
					BackOffParams: task.BackOffParams,
					Deadline:      task.Deadline,
					CreatedAt:     task.CreatedAt,
					LastRun:       task.LastRun,
					NextRun:       task.NextRun,
				})
			}
		}

		pkgLockDuration := int64(len(taskIDs)) * 5 * int64(time.Minute)
		return s.taskRepo.Lock(ctx, taskIDs, now.Add(time.Duration(pkgLockDuration)))
	})

	if getErr != nil {
		return nil, getErr
	}
	return result, nil
}

func (s *WorkerStore) SaveTask(task *retrier.Task, result *retrier.TaskExecutionResult) error {
	return storage.InTransaction(context.TODO(), s.db, func(ctx context.Context) error {
		if s.onboarding != nil {
			if _, err := s.onboarding.LockTaskParent(ctx, task.ID); err != nil {
				return err
			}
		}
		taskData, err := s.taskRepo.GetByID(ctx, task.ID)
		if err != nil {
			slog.Error("get task error",
				slog.String("task_id", task.ID.String()),
				slog.Any("error", err))
			return err
		}

		taskData.Status = task.Status
		taskData.NextRun = task.NextRun
		taskData.LastRun = task.LastRun
		taskData.Retries = task.Retries

		if _, err := s.taskRepo.Update(ctx, taskData); err != nil {
			slog.Error("update task error",
				slog.String("task_id", task.ID.String()),
				slog.Any("error", err))
			return err
		}

		if err := s.taskResultRepo.Create(ctx, &dto.TaskExecutionResult{
			ID:            result.ID,
			TaskID:        result.TaskID,
			Status:        result.Status,
			RunAt:         result.RunAt,
			Result:        result.Result,
			IsCritical:    result.IsCritical,
			ExecutionTime: result.ExecutionTime,
		}); err != nil {
			slog.Error("save result error",
				slog.String("task_id", task.ID.String()),
				slog.Any("error", err))
			return err
		}
		if s.onboarding != nil {
			if err := s.onboarding.TaskCompleted(ctx, task.ID, task.Status); err != nil {
				return err
			}
			if _, err := s.onboarding.FinalizeTask(ctx, task.ID); err != nil {
				return err
			}
		}

		return nil
	})
}
