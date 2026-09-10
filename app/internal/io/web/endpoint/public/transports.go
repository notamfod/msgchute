package public

import (
	"net/http"

	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/devian2011/msgchute/pkg/http/response"
)

// NewTransportsEndpoint lists configured profiles, not historical message values.
//
// @Summary List configured delivery profiles
// @Description Available means configured for acceptance, not remote provider health. Credentials are omitted.
// @Tags public.messages
// @Produce json
// @Success 200 {object} response.Response{data=[]sender.Transport}
// @Router /api/v1/transports [get]
func NewTransportsEndpoint(items []sender.Transport) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteSuccessResponse(w, r, http.StatusOK, items)
	})
}
