package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/sender"
)

type batchValidationStub struct{ handled bool }
type retryValidationStub struct{ err error }

func (s *batchValidationStub) Validate(messages []*dto.Message) error {
	if len(messages) == 2 && messages[0].Tag == "" && messages[1].Tag == "invalid" {
		return sender.ErrInvalidTag
	}
	return nil
}
func (s *batchValidationStub) Handle([]*dto.Message) []*dto.AddBatchMessageResponse {
	s.handled = true
	return nil
}
func (s *retryValidationStub) Handle(*dto.MessageRetryRequest) (*dto.Message, *dto.Task, error) {
	return nil, nil, s.err
}

func TestBatchSenderRejectsInvalidTagBeforeQueueing(t *testing.T) {
	stub := &batchValidationStub{}
	recorder := httptest.NewRecorder()
	NewBatchSenderEndpoint(stub).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/batch/send", strings.NewReader(`[{"transport":"mail"},{"transport":"mail","tag":"invalid"}]`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if stub.handled {
		t.Fatal("batch was queued before validation")
	}
}

func TestRetrySenderMapsPersistedTagValidationToBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewMessageRetryEndpoint(&retryValidationStub{err: sender.ErrUnknownPreferenceChannel}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/message/retry", strings.NewReader(`{"id":"00000000-0000-0000-0000-000000000001"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
