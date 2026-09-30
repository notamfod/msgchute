package public

import (
	"github.com/devian2011/msgchute/internal/dto"
)

type queue interface {
	Add(*dto.Message) (*dto.Message, *dto.Task, error)
	Validate(*dto.Message) error
}

func (h *BatchSenderHandler) Validate(messages []*dto.Message) error {
	for _, message := range messages {
		if err := h.q.Validate(message); err != nil {
			return err
		}
	}
	return nil
}

type SenderHandler struct {
	q queue
}

func NewSenderHandler(q queue) *SenderHandler {
	return &SenderHandler{q: q}
}

func (h *SenderHandler) Handle(msg *dto.Message) (*dto.Message, *dto.Task, error) {
	return h.q.Add(msg)
}

// Batch

type BatchSenderHandler struct {
	q queue
}

func NewBatchSenderHandler(q queue) *BatchSenderHandler {
	return &BatchSenderHandler{q: q}
}

func (h *BatchSenderHandler) Handle(msg []*dto.Message) []*dto.AddBatchMessageResponse {
	var results []*dto.AddBatchMessageResponse
	for i := range msg {
		m, t, e := h.q.Add(msg[i])
		results = append(results, &dto.AddBatchMessageResponse{
			Message: m,
			Task:    t,
			Err:     e,
		})
	}

	return results
}

// Retry

type RetryableQueue interface {
	Retry(request *dto.MessageRetryRequest) (*dto.Message, *dto.Task, error)
}

type RetryHandler struct {
	queue RetryableQueue
}

func NewMessageRetryHandler(queue RetryableQueue) *RetryHandler {
	return &RetryHandler{queue: queue}
}

func (h *RetryHandler) Handle(r *dto.MessageRetryRequest) (*dto.Message, *dto.Task, error) {
	return h.queue.Retry(r)
}
