package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/stretchr/testify/require"
)

type conflictingTemplateCreator struct{}

func (conflictingTemplateCreator) Handle(*dto.Template) (*dto.Template, error) {
	return nil, template.ErrTemplateAlreadyExists
}

func TestTemplateCreateDuplicateCodeIsConflict(t *testing.T) {
	response := httptest.NewRecorder()
	NewTemplateCreationEndpoint(conflictingTemplateCreator{}).ServeHTTP(response,
		httptest.NewRequest(http.MethodPost, "/api/admin/v1/template", strings.NewReader(`{"code":"sms_custom_test"}`)))
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}
