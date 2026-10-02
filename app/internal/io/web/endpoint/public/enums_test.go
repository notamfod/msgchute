package public

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devian2011/msgchute/internal/service/sender"
	"github.com/stretchr/testify/require"
)

func TestEnumsEndpointReturnsConfiguredReferenceValues(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewEnumsEndpoint([]sender.Transport{{Code: "ya_mail", Provider: "smtp", Available: true}}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/enums", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{
		"status":"success",
		"data":{
			"transports":[{"code":"ya_mail","provider":"smtp","available":true}],
			"message_statuses":["running","succeeded","failed","declined"],
			"tags":["order","promotion","news"],
			"recipient_kinds":["email","phone"],
			"subscriptions":{
				"email":["email.order","email.promotion","email.news"],
				"phone":["sms.order","sms.promotion","sms.news","whatsapp.order","whatsapp.promotion","whatsapp.news"]
			}
		}
	}`, recorder.Body.String())
}
