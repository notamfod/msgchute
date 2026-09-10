package public

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type metadataStub struct {
	calls int
	err   error
	patch dto.TemplateMetadata
}

func (s *metadataStub) PatchMetadata(_ context.Context, code string, p dto.TemplateMetadata) (dto.TemplateMetadata, error) {
	s.calls++
	s.patch = p
	return p, s.err
}

func TestMetadataPatchHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		err           error
		status, calls int
	}{
		{"object", `{"1c":{"id":9007199254740993},"bitrix24":null}`, nil, 200, 1},
		{"null", `null`, nil, 400, 0}, {"array", `[]`, nil, 400, 0}, {"scalar", `1`, nil, 400, 0},
		{"malformed", `{`, nil, 400, 0}, {"trailing JSON", `{} {}`, nil, 400, 0},
		{"too large", `{"data":"` + strings.Repeat("x", template.MetadataLimit) + `"}`, nil, 413, 0},
		{"not found", `{}`, template.ErrTemplateNotFound, 404, 1},
		{"merged too large", `{}`, template.ErrMetadataTooLarge, 413, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &metadataStub{err: tc.err}
			router := chi.NewRouter()
			router.Method(http.MethodPatch, "/api/v1/templates/{code}/metadata", NewTemplateMetadataEndpoint(stub))
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/api/v1/templates/order/metadata", strings.NewReader(tc.body)))
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			require.Equal(t, tc.calls, stub.calls)
			if tc.name == "object" {
				require.Equal(t, json.Number("9007199254740993"), stub.patch["1c"].(map[string]any)["id"])
			}
		})
	}
}
