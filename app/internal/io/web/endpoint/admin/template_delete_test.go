package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type templateDeleteStub struct {
	code string
	err  error
}

func (s *templateDeleteStub) Delete(_ context.Context, code string) error {
	s.code = code
	return s.err
}

func TestTemplateDeleteHTTP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"success", nil, http.StatusOK},
		{"missing", template.ErrTemplateNotFound, http.StatusNotFound},
		{"storage failure", errors.New("storage error"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &templateDeleteStub{err: tc.err}
			router := chi.NewRouter()
			router.Method(http.MethodDelete, "/api/admin/v1/template/{code}", NewTemplateDeletionEndpoint(stub))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/api/admin/v1/template/sms_custom_test", nil))
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			require.Equal(t, "sms_custom_test", stub.code)
			if tc.status == http.StatusOK {
				var payload struct {
					Status string `json:"status"`
					Data   struct {
						Code string `json:"code"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
				require.Equal(t, "success", payload.Status)
				require.Equal(t, stub.code, payload.Data.Code)
			}
		})
	}
}

func TestTemplateDeleteWithoutCodeRejectsRequest(t *testing.T) {
	stub := &templateDeleteStub{}
	recorder := httptest.NewRecorder()
	NewTemplateDeletionEndpoint(stub).ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/", nil))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Empty(t, stub.code)
}
