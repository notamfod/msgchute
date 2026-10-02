package public

import (
	"net/http"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/devian2011/msgchute/pkg/http/response"
)

type Enums struct {
	Transports      []sender.Transport              `json:"transports"`
	MessageStatuses []dto.MessageStatus             `json:"message_statuses"`
	Tags            []string                        `json:"tags"`
	RecipientKinds  []string                        `json:"recipient_kinds"`
	Subscriptions   map[string]dto.SubscriptionList `json:"subscriptions"`
}

// @Summary		List API reference enums
// @Description	Returns configured transport profiles and valid message, recipient, and subscription codes.
// @Description	Omitting a message tag applies only the full opt-out check. Tagged TG and MAX delivery is not currently supported.
// @Tags			public.messages
// @Produce		json
// @Success		200	{object}	response.Response{data=Enums}
// @Router			/api/v1/enums [get]
func NewEnumsEndpoint(transports []sender.Transport) http.Handler {
	data := Enums{
		Transports:      transports,
		MessageStatuses: dto.MessageStatuses(),
		Tags:            dto.MessageTags(),
		RecipientKinds:  stoplist.RecipientKinds(),
		Subscriptions:   stoplist.Subscriptions(),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteSuccessResponse(w, r, http.StatusOK, data)
	})
}
