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

var (
	ErrUnknownTransport         = errors.New("unknown transport")
	ErrInvalidTag               = errors.New("invalid message tag")
	ErrUnknownPreferenceChannel = errors.New("unknown preference channel")
	ErrInvalidBodySource        = errors.New("invalid message body source")
)

type Queue struct {
	templates   TemplateGenerator
	providers   map[string]*ProviderConfig
	ctx         context.Context
	db          *sqlx.DB
	taskRepo    taskRepo
	messageRepo messageRepo
	onboarding  *OnboardingStore
}

func NewQueue(ctx context.Context, db *sqlx.DB, taskRepo taskRepo, messageRepo messageRepo, providers map[string]*ProviderConfig, templates TemplateGenerator, onboarding ...*OnboardingStore) *Queue {
	queue := &Queue{
		templates:   templates,
		providers:   providers,
		ctx:         ctx,
		db:          db,
		taskRepo:    taskRepo,
		messageRepo: messageRepo,
	}
	if len(onboarding) > 0 {
		queue.onboarding = onboarding[0]
	}
	return queue
}

func (s *Queue) Add(message *dto.Message) (*dto.Message, *dto.Task, error) {
	subject, body, err := s.renderAndValidate(message)
	if err != nil {
		return nil, nil, err
	}
	if s.onboarding != nil {
		if s.onboarding.Enabled(message.Transport) {
			// Freeze the rendered content used by a later messenger or SMS route.
			message.Subject = subject
			message.Body = body
		}
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

	newTask := func() *dto.Task {
		return &dto.Task{
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
	}
	deliveryRecipients := []string(nil)
	tasks := []*dto.Task{newTask()}
	if s.onboarding != nil && s.onboarding.Enabled(message.Transport) {
		deliveryRecipients = distinctOnboardingRecipients(message.Recipients)
		tasks = make([]*dto.Task, 0, len(deliveryRecipients))
		for range deliveryRecipients {
			tasks = append(tasks, newTask())
		}
	}
	if len(tasks) == 0 {
		tasks = append(tasks, newTask())
	}

	if !message.Deadline.IsZero() {
		for _, task := range tasks {
			task.Deadline = message.Deadline
		}
	}

	getErr := storage.InTransaction(context.TODO(), s.db, func(ctx context.Context) error {
		messageCreateErr := s.messageRepo.Create(ctx, message)
		if messageCreateErr != nil {
			slog.Error("Failed to create message", "error", messageCreateErr)
			return messageCreateErr
		}

		for i, task := range tasks {
			if _, taskCreateErr := s.taskRepo.Create(ctx, task); taskCreateErr != nil {
				slog.Error("Failed to create task", "error", taskCreateErr)
				return taskCreateErr
			}
			if len(deliveryRecipients) > 0 {
				if err := s.onboarding.CreateDelivery(ctx, task, message, deliveryRecipients[i]); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if getErr != nil {
		slog.Error("Failed to add message to queue", "error", getErr.Error())
		return nil, nil, getErr
	}

	return message, tasks[0], nil
}

// Preflight performs all validation that can fail before queue persistence.
// Batch callers use it for every message before adding the first one.
func (s *Queue) Preflight(message *dto.Message) error {
	_, _, err := s.renderAndValidate(message)
	return err
}

func (s *Queue) renderAndValidate(message *dto.Message) (string, string, error) {
	if err := s.Validate(message); err != nil {
		return "", "", err
	}
	subject, body, err := s.templates.GenerateMessage(message)
	if err != nil {
		return "", "", err
	}
	if s.onboarding != nil {
		if err := s.onboarding.ValidateContent(s.ctx, message, subject, body); err != nil {
			return "", "", err
		}
	}
	return subject, body, nil
}

func distinctOnboardingRecipients(recipients dto.Recipients) []string {
	result := make([]string, 0, len(recipients))
	seen := make(map[string]struct{}, len(recipients))
	for _, recipient := range recipients {
		key := "opaque:" + recipient
		if phone, ok := explicitPhone(recipient); ok {
			key = "phone:" + phone
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, recipient)
	}
	return result
}

func (s *Queue) Validate(message *dto.Message) error {
	if message == nil {
		return ErrUnknownTransport
	}
	if !message.BodySource.Valid() {
		return ErrInvalidBodySource
	}
	p := s.providers[message.Transport]
	if p == nil {
		return fmt.Errorf("%w: %s", ErrUnknownTransport, message.Transport)
	}
	if message.Tag == "" {
		return nil
	}
	if !ValidTag(message.Tag) {
		return ErrInvalidTag
	}
	_, err := ResolveChannel(p)
	return err
}

func ValidTag(tag string) bool { return dto.ValidMessageTag(tag) }

func ResolveChannel(p *ProviderConfig) (string, error) {
	if p == nil {
		return "", ErrUnknownTransport
	}
	if p.Channel != "" {
		if p.Channel == "email" || p.Channel == "sms" || p.Channel == "whatsapp" {
			return p.Channel, nil
		}
		return "", ErrUnknownPreferenceChannel
	}
	switch p.Provider {
	case "smtp":
		return "email", nil
	case "smsc", "beeline":
		return "sms", nil
	case "whatsapp":
		return "whatsapp", nil
	default:
		return "", ErrUnknownPreferenceChannel
	}
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
	if err := s.Validate(msg); err != nil {
		return nil, nil, err
	}
	if s.onboarding == nil || !s.onboarding.Enabled(msg.Transport) {
		if _, _, err := s.templates.GenerateMessage(msg); err != nil {
			return nil, nil, err
		}
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

		newTask := func() *dto.Task {
			return &dto.Task{
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
		}
		recipients := []string(nil)
		onboardingMessage := false
		if s.onboarding != nil {
			var retryErr error
			recipients, onboardingMessage, retryErr = s.onboarding.RetryRecipients(ctx, msg.ID)
			if retryErr != nil {
				return retryErr
			}
		}
		if onboardingMessage && len(recipients) == 0 {
			return errors.New("message already delivered to all recipients")
		}
		if !onboardingMessage {
			task = newTask()
			if _, taskCreateErr := s.taskRepo.Create(ctx, task); taskCreateErr != nil {
				slog.Error("Failed to create task", "error", taskCreateErr)
				return taskCreateErr
			}
			return nil
		}
		for _, recipient := range recipients {
			created := newTask()
			if task == nil {
				task = created
			}
			if _, taskCreateErr := s.taskRepo.Create(ctx, created); taskCreateErr != nil {
				return taskCreateErr
			}
			if err := s.onboarding.CreateDelivery(ctx, created, msg, recipient); err != nil {
				return err
			}
		}

		return s.messageRepo.UpdateStatus(ctx, msg.ID, dto.MessageStatusRunning)
	})

	if err != nil {
		return nil, nil, err
	}
	return msg, task, nil
}
