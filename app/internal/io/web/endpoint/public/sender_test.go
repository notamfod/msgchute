package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	publichandler "github.com/devian2011/msgchute/internal/handler/public"
	"github.com/devian2011/msgchute/internal/service/sender"
)

type batchValidationStub struct {
	handled bool
	err     error
}
type retryValidationStub struct{ err error }
type senderValidationStub struct{ err error }

type batchPreflightQueue struct {
	adds       int
	preflights int
}

func (q *batchPreflightQueue) Validate(*dto.Message) error { return nil }
func (q *batchPreflightQueue) Preflight(*dto.Message) error {
	q.preflights++
	if q.preflights == 2 {
		return sender.ErrUnsupportedOnboardingContent
	}
	return nil
}
func (q *batchPreflightQueue) Add(*dto.Message) (*dto.Message, *dto.Task, error) {
	q.adds++
	return nil, nil, nil
}

func (s *batchValidationStub) Validate(messages []*dto.Message) error {
	if s.err != nil {
		return s.err
	}
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
func (s *senderValidationStub) Handle(*dto.Message) (*dto.Message, *dto.Task, error) {
	return nil, nil, s.err
}

func TestBatchSenderRejectsUnknownTransportBeforeQueueing(t *testing.T) {
	for _, transport := range []string{"ya-mail", "yamail"} {
		t.Run(transport, func(t *testing.T) {
			stub := &batchValidationStub{err: sender.ErrUnknownTransport}
			recorder := httptest.NewRecorder()
			body := `[{"transport":"beeline"},{"transport":"` + transport + `"}]`
			NewBatchSenderEndpoint(stub).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/batch/send", strings.NewReader(body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if stub.handled {
				t.Fatal("batch was queued before validation")
			}
		})
	}
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

func TestBatchSenderPreflightsRichUnknownPhoneBeforeQueueing(t *testing.T) {
	queue := &batchPreflightQueue{}
	recorder := httptest.NewRecorder()
	handler := publichandler.NewBatchSenderHandler(queue)
	NewBatchSenderEndpoint(handler).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/batch/send", strings.NewReader(`[
		{"sender_id":"integration","transport":"tg","recipients":["@known"],"body":"plain"},
		{"sender_id":"integration","transport":"tg","recipients":["+79991234567"],"body":"<b>rich</b>"}
	]`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if queue.adds != 0 {
		t.Fatalf("queued %d messages before batch preflight completed", queue.adds)
	}
}

func TestRetrySenderMapsPersistedTagValidationToBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewMessageRetryEndpoint(&retryValidationStub{err: sender.ErrUnknownPreferenceChannel}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/message/retry", strings.NewReader(`{"id":"00000000-0000-0000-0000-000000000001"}`)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSenderMapsUnsupportedOnboardingContentToBadRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewSenderEndpoint(&senderValidationStub{err: sender.ErrUnsupportedOnboardingContent}).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/send", strings.NewReader(`{"transport":"tg","recipients":["+79991234567"],"body":"<b>markup</b>"}`)),
	)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
