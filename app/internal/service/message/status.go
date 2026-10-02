package message

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/io/storage"
)

type onboardingStatus interface {
	IsOnboardingTask(context.Context, uuid.UUID) (bool, error)
}

type StatusUpdater struct {
	db         *sqlx.DB
	msgRepo    messageRepo
	taskRepo   taskRepo
	onboarding onboardingStatus
}

func NewStatusUpdater(
	db *sqlx.DB,
	msgRepo messageRepo,
	taskRepo taskRepo,
	onboarding ...onboardingStatus,
) *StatusUpdater {
	updater := &StatusUpdater{
		db:       db,
		msgRepo:  msgRepo,
		taskRepo: taskRepo,
	}
	if len(onboarding) > 0 {
		updater.onboarding = onboarding[0]
	}
	return updater
}

func (s *StatusUpdater) UpdateStatusByTaskID(taskID uuid.UUID, status dto.MessageStatus) error {
	return storage.InTransaction(context.Background(), s.db, func(ctx context.Context) error {
		task, getTaskErr := s.taskRepo.GetByID(ctx, taskID)
		if getTaskErr != nil {
			return getTaskErr
		}
		unlockErr := s.taskRepo.Unlock(ctx, []uuid.UUID{task.ID})
		if unlockErr != nil {
			return unlockErr
		}
		if s.onboarding != nil {
			onboardingTask, err := s.onboarding.IsOnboardingTask(ctx, task.ID)
			if err != nil {
				return err
			}
			if onboardingTask {
				return nil
			}
		}

		m, getErr := s.msgRepo.GetByID(ctx, task.MessageID)
		if getErr != nil {
			return getErr
		}
		if m.Status == status || m.Status == dto.MessageStatusSucceeded {
			return nil
		}
		return s.msgRepo.UpdateStatus(ctx, m.ID, status)
	})
}
