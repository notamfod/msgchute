package public

import (
	"errors"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/devian2011/msgchute/pkg/http/response"
)

type SenderMessageRequest struct {
	SenderID   string            `json:"sender_id" db:"sender_id" validate:"required"`
	Recipients dto.Recipients    `json:"recipients" db:"recipients" validate:"required"`
	Meta       dto.MessageMeta   `json:"meta" db:"meta"`
	Code       *string           `json:"code,omitempty" db:"code"`
	Params     dto.MessageParams `json:"params,omitempty" db:"params"`
	Transport  string            `json:"transport" db:"transport" validate:"required"` // Transport message provider
	Subject    string            `json:"subject" db:"subject"`
	Body       string            `json:"body" db:"body"`
	Deadline   time.Time         `json:"deadline" db:"deadline"`
	Retry      *dto.Retry        `json:"retry,omitempty" db:"retry"`
	Schedule   time.Time         `json:"schedule,omitempty" db:"schedule"`
}

type senderHandler interface {
	Handle(msg *dto.Message) (*dto.Message, *dto.Task, error)
}

type SenderEndpoint struct {
	h senderHandler
}

func NewSenderEndpoint(h senderHandler) *SenderEndpoint {
	return &SenderEndpoint{h}
}

// ServeHTTP handles the message delivery process
//
//	@Summary		Send message
//	@Description	Accepts a message payload, processes its delivery, and registers the tracking task.
//	@Tags			public.messages
//	@Accept			json
//	@Produce		json
//	@Param			request	body		SenderMessageRequest	true	"Message details to be processed and sent"
//	@Success		200		{object}	dto.AddMessageResponse	"Message processed and delivery task created successfully"
//	@Failure		400		{object}	response.Response		"Invalid JSON payload or structural error"
//	@Failure		500		{object}	response.Response		"Internal server error during processing or delivery dispatch"
//	@Router			/api/v1/send [post]
func (e *SenderEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var msgRequest SenderMessageRequest
	if decodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&msgRequest); decodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, decodeErr)
		return
	}

	msgResult, taskResult, sendErr := e.h.Handle(&dto.Message{
		ID:         uuid.UUID{},
		SenderID:   msgRequest.SenderID,
		Recipients: msgRequest.Recipients,
		Status:     "",
		Meta:       msgRequest.Meta,
		Code:       msgRequest.Code,
		Params:     msgRequest.Params,
		Transport:  msgRequest.Transport,
		Subject:    msgRequest.Subject,
		Body:       msgRequest.Body,
		Deadline:   msgRequest.Deadline,
		Retry:      msgRequest.Retry,
		Schedule:   msgRequest.Schedule,
	})
	if sendErr != nil {
		if errors.Is(sendErr, sender.ErrUnknownTransport) || errors.Is(sendErr, template.ErrMissingParameters) {
			response.WriteErrorResponse(w, r, http.StatusBadRequest, sendErr)
			return
		}
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, sendErr)
		return
	}

	response.WriteSuccessResponse(w, r, http.StatusOK, &dto.AddMessageResponse{
		Message: msgResult,
		Task:    taskResult,
	})
}

// Batch

type batchSenderHandler interface {
	Handle(msg []*dto.Message) []*dto.AddBatchMessageResponse
}

type BatchSenderEndpoint struct {
	h batchSenderHandler
}

func NewBatchSenderEndpoint(h batchSenderHandler) *BatchSenderEndpoint {
	return &BatchSenderEndpoint{h}
}

// ServeHTTP handles batch message delivery process
//
//	@Summary		Send multiple messages
//	@Description	Accepts an array of message payloads, processes their delivery, and registers tracking tasks for each.
//	@Tags			public.messages
//	@Accept			json
//	@Produce		json
//	@Param			request	body		[]SenderMessageRequest		true	"List of messages to be processed and sent"
//	@Success		200		{array}		dto.AddBatchMessageResponse	"Messages processed and delivery tasks created successfully"
//	@Failure		400		{object}	response.Response			"Invalid JSON payload or structural error"
//	@Failure		500		{object}	response.Response			"Internal server error during processing or delivery dispatch"
//	@Router			/api/v1/batch/send [post]
func (e *BatchSenderEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request []*SenderMessageRequest
	if decodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, decodeErr)
		return
	}

	messages := make([]*dto.Message, 0, len(request))
	for i := range request {
		messages = append(messages, &dto.Message{
			ID:         uuid.UUID{},
			SenderID:   request[i].SenderID,
			Recipients: request[i].Recipients,
			Status:     "",
			Meta:       request[i].Meta,
			Code:       request[i].Code,
			Params:     request[i].Params,
			Transport:  request[i].Transport,
			Subject:    request[i].Subject,
			Body:       request[i].Body,
			Deadline:   request[i].Deadline,
			Retry:      request[i].Retry,
			Schedule:   request[i].Schedule,
		})
	}

	result := e.h.Handle(messages)

	response.WriteSuccessResponse(w, r, http.StatusOK, result)
}

// Retry

type messageRetryHandler interface {
	Handle(r *dto.MessageRetryRequest) (*dto.Message, *dto.Task, error)
}

type MessageRetryEndpoint struct {
	h messageRetryHandler
}

func NewMessageRetryEndpoint(h messageRetryHandler) *MessageRetryEndpoint {
	return &MessageRetryEndpoint{h: h}
}

// MessageRetryEndpoint handles retry requests for messages.
//
//	@Summary		Retry a message
//	@Description	Creates a new task to retry a previously failed or pending message. It allows overriding retry policy and schedule.
//	@Tags			public.messages
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.MessageRetryRequest	true	"Retry request parameters"
//	@Success		200		{object}	dto.AddMessageResponse	"Message and task details"
//	@Failure		400		{object}	response.Response		"Invalid request payload"
//	@Failure		500		{object}	response.Response		"Internal server error"
//	@Router			/api/v1/message/retry [post]
func (e *MessageRetryEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request *dto.MessageRetryRequest
	if decodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, decodeErr)
		return
	}

	m, t, err := e.h.Handle(request)
	if err != nil {
		if errors.Is(err, sender.ErrUnknownTransport) || errors.Is(err, template.ErrMissingParameters) {
			response.WriteErrorResponse(w, r, http.StatusBadRequest, err)
			return
		}
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, err)
		return
	}

	response.WriteSuccessResponse(w, r, http.StatusOK, dto.AddMessageResponse{
		Message: m,
		Task:    t,
	})
}
