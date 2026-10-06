package public

import (
	"errors"
	"net/http"

	"github.com/bytedance/sonic"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/devian2011/msgchute/pkg/http/response"
)

type PreviewMessageRequest struct {
	Code       *string               `json:"code,omitempty" db:"code"`
	Params     dto.MessageParams     `json:"params,omitempty" db:"params"`
	Subject    string                `json:"subject" db:"subject"`
	Body       string                `json:"body" db:"body"`
	BodySource dto.MessageBodySource `json:"body_source,omitempty" enums:"template,provided"` // "provided" preserves Body without template rendering.
}

type previewHandler interface {
	Handle(msg *dto.Message) (dto.MessagePreview, error)
}

type MessagePreviewEndpoint struct {
	h previewHandler
}

func NewMessagePreviewEndpoint(h previewHandler) *MessagePreviewEndpoint {
	return &MessagePreviewEndpoint{h: h}
}

// ServeHTTP previews a message template before sending
//
//	@Summary		Preview message
//	@Description	Accepts a raw message template along with dynamic parameters, renders them, and returns the final subject and body preview.
//	@Tags			public.messages
//	@Accept			json
//	@Produce		json
//	@Param			request	body		PreviewMessageRequest	true	"Message template definition and rendering context"
//	@Success		200		{object}	dto.MessagePreview		"Template successfully rendered"
//	@Failure		400		{object}	response.Response		"Invalid JSON payload or syntax error"
//	@Failure		500		{object}	response.Response		"Internal server error during template rendering"
//	@Router			/api/v1/preview [post]
func (e *MessagePreviewEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req PreviewMessageRequest
	if decodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, decodeErr)
		return
	}
	if !req.BodySource.Valid() {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("invalid message body source"))
		return
	}
	msg := &dto.Message{
		Code:       req.Code,
		Params:     req.Params,
		Subject:    req.Subject,
		Body:       req.Body,
		BodySource: req.BodySource,
	}

	preview, previewErr := e.h.Handle(msg)
	if previewErr != nil {
		if errors.Is(previewErr, template.ErrMissingParameters) {
			response.WriteErrorResponse(w, r, http.StatusBadRequest, previewErr)
			return
		}
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, previewErr)
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, preview)
}
