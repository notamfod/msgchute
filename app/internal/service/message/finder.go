package message

import (
	"context"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/devian2011/msgchute/internal/dto"
)

type Finder struct {
	db             *sqlx.DB
	msgRepo        messageRepo
	taskRepo       taskRepo
	taskResultRepo taskResultRepo
}

func NewFinder(
	db *sqlx.DB,
	msgRepo messageRepo,
	taskRepo taskRepo,
	taskResultRepo taskResultRepo,
) *Finder {
	return &Finder{
		db:             db,
		msgRepo:        msgRepo,
		taskRepo:       taskRepo,
		taskResultRepo: taskResultRepo,
	}
}

func (f *Finder) Find(filter *dto.MessageFilter) ([]dto.FullMessageInfo, int, error) {
	ctx := context.Background()
	msgs, pages, getMsgErr := f.msgRepo.Find(ctx, filter)
	if getMsgErr != nil {
		return nil, 0, getMsgErr
	}
	if len(msgs) == 0 {
		return []dto.FullMessageInfo{}, int(pages), nil
	}

	msgIDs := make([]uuid.UUID, 0, len(msgs))
	for i := range msgs {
		msgIDs = append(msgIDs, msgs[i].ID)
	}
	tasks, getTasksErr := f.taskRepo.GetByMessageIDs(ctx, msgIDs)
	if getTasksErr != nil {
		return nil, 0, getTasksErr
	}
	taskIDs := make([]uuid.UUID, 0)
	for _, tSlice := range tasks {
		for _, t := range tSlice {
			taskIDs = append(taskIDs, t.ID)
		}
	}
	taskResults, taskResultsGetErr := f.taskResultRepo.GetByTaskIDs(ctx, taskIDs)
	if taskResultsGetErr != nil {
		return nil, 0, taskResultsGetErr
	}

	fullMessages := make([]dto.FullMessageInfo, 0, len(msgs))

	for mI := range msgs {
		fullMessage := dto.FullMessageInfo{
			Message: *msgs[mI],
		}
		if mTasks, exists := tasks[msgs[mI].ID]; exists {
			for mT := range mTasks {
				ft := dto.FullTask{
					Task: tasks[msgs[mI].ID][mT],
				}
				if mResults, mExists := taskResults[mTasks[mT].ID]; mExists {
					ft.Results = mResults
				}
				fullMessage.Tasks = append(fullMessage.Tasks, ft)
			}
		}
		fullMessages = append(fullMessages, fullMessage)
	}

	return fullMessages, int(pages), nil
}

func (f *Finder) FindByID(messageID uuid.UUID) (*dto.FullMessageInfo, error) {
	ctx := context.Background()
	msg, getMsgErr := f.msgRepo.GetByID(ctx, messageID)
	if getMsgErr != nil {
		return nil, getMsgErr
	}
	if msg == nil {
		return nil, nil
	}
	tasks, getTasksErr := f.taskRepo.GetByMessageIDs(ctx, []uuid.UUID{msg.ID})
	if getTasksErr != nil {
		return nil, getTasksErr
	}
	taskIDs := make([]uuid.UUID, 0)
	for _, t := range tasks[msg.ID] {
		taskIDs = append(taskIDs, t.ID)
	}
	taskResults, taskResultsGetErr := f.taskResultRepo.GetByTaskIDs(ctx, taskIDs)
	if taskResultsGetErr != nil {
		return nil, taskResultsGetErr
	}

	fullTasks := make([]dto.FullTask, 0, len(tasks[msg.ID]))
	for i := range tasks[msg.ID] {
		ft := dto.FullTask{
			Task: tasks[msg.ID][i],
		}
		if r, exists := taskResults[tasks[msg.ID][i].ID]; exists {
			ft.Results = r
		}
		fullTasks = append(fullTasks, ft)
	}

	return &dto.FullMessageInfo{
		Message: *msg,
		Tasks:   fullTasks,
	}, nil
}

func (f *Finder) GetSenders(ctx context.Context) ([]string, error) {
	return f.msgRepo.GetSenders(ctx)
}

func (f *Finder) GetTransports(ctx context.Context) ([]string, error) {
	return f.msgRepo.GetTransports(ctx)
}

func (f *Finder) GetTemplates(ctx context.Context) ([]string, error) {
	return f.msgRepo.GetTemplateCodes(ctx)
}

func (f *Finder) GetRecipients(ctx context.Context, search string) ([]string, error) {
	return f.msgRepo.GetRecipients(ctx, search)
}
