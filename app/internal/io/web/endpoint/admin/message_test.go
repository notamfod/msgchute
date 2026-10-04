package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type messageByIDStub struct {
	result *dto.FullMessageInfo
	err    error
	calls  int
}

func (s *messageByIDStub) Handle(uuid.UUID) (*dto.FullMessageInfo, error) {
	s.calls++
	return s.result, s.err
}

func TestMessageFinderByIDEndpoint(t *testing.T) {
	id := uuid.New()
	for _, tc := range []struct {
		name   string
		id     string
		result *dto.FullMessageInfo
		err    error
		status int
		calls  int
	}{
		{"found", id.String(), &dto.FullMessageInfo{Message: dto.Message{ID: id}}, nil, 200, 1},
		{"missing zero UUID", uuid.Nil.String(), nil, nil, 404, 1},
		{"missing random UUID", id.String(), nil, nil, 404, 1},
		{"repository error", id.String(), nil, errors.New("database unavailable"), 500, 1},
		{"invalid UUID", "invalid", nil, nil, 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &messageByIDStub{result: tc.result, err: tc.err}
			router := chi.NewRouter()
			router.Method(http.MethodGet, "/api/admin/v1/message/{id}", NewMessageFinderByIDEndpoint(stub))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/admin/v1/message/"+tc.id, nil))

			assert.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			assert.Equal(t, tc.calls, stub.calls)
			if tc.status == http.StatusNotFound {
				assert.JSONEq(t, `{"status":"error","error":"message not found"}`, recorder.Body.String())
			}
		})
	}
}
