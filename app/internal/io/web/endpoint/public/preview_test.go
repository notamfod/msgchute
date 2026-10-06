package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/devian2011/msgchute/internal/dto"
)

type previewValidationStub struct{ calls int }

func (s *previewValidationStub) Handle(*dto.Message) (dto.MessagePreview, error) {
	s.calls++
	return dto.MessagePreview{}, nil
}

func TestPreviewRejectsUnknownBodySource(t *testing.T) {
	stub := &previewValidationStub{}
	recorder := httptest.NewRecorder()
	NewMessagePreviewEndpoint(stub).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/preview", strings.NewReader(`{"body_source":"rendered"}`)),
	)
	require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
	require.Zero(t, stub.calls)
}
