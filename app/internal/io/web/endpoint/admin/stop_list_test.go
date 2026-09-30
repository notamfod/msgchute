package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type stopListEndpointStub struct {
	calls int
	err   error
}

func (s *stopListEndpointStub) Create(_ context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	s.calls++
	entry.ID = uuid.New()
	return entry, s.err
}
func (s *stopListEndpointStub) Delete(context.Context, string) error { s.calls++; return s.err }
func (s *stopListEndpointStub) Find(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	s.calls++
	return []dto.StopListEntry{}, 0, s.err
}
func (s *stopListEndpointStub) List(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error) {
	s.calls++
	return []dto.StopListEntry{}, 0, s.err
}
func (s *stopListEndpointStub) Upsert(_ context.Context, entry *dto.StopListEntry) (*dto.StopListEntry, error) {
	s.calls++
	return entry, s.err
}

func TestStopListEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, method, url, body string
		err                     error
		want                    int
	}{
		{"list", http.MethodGet, "/api/admin/v1/stop-list?per_page=20", "", nil, 200},
		{"create", http.MethodPost, "/api/admin/v1/stop-list", `{"kind":"email","recipient":"a@example.com"}`, nil, 200},
		{"invalid create", http.MethodPost, "/api/admin/v1/stop-list", `{`, nil, 400},
		{"not found", http.MethodDelete, "/api/admin/v1/stop-list/" + uuid.NewString(), "", stoplist.ErrNotFound, 404},
		{"internal error is safe", http.MethodGet, "/api/admin/v1/stop-list", "", errors.New("database password"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stopListEndpointStub{err: tc.err}
			endpoint := NewStopListEndpoint(stub)
			router := chi.NewRouter()
			router.Method(http.MethodGet, "/api/admin/v1/stop-list", http.HandlerFunc(endpoint.List))
			router.Method(http.MethodPost, "/api/admin/v1/stop-list", http.HandlerFunc(endpoint.Create))
			router.Method(http.MethodDelete, "/api/admin/v1/stop-list/{id}", http.HandlerFunc(endpoint.Delete))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body)))
			assert.Equal(t, tc.want, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), "database password")
		})
	}
}
