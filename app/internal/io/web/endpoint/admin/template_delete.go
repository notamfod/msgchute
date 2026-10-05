package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/devian2011/msgchute/pkg/http/response"
	"github.com/go-chi/chi/v5"
)

type templateDeleter interface {
	Delete(context.Context, string) error
}

type TemplateDeletionResult struct {
	Code string `json:"code"`
}

// @Summary Delete a template
// @Description Deletes a template by code. Historical message bodies are retained; their template reference is cleared.
// @Tags admin.templates
// @Produce json
// @Param code path string true "Stable template code"
// @Success 200 {object} response.Response{data=TemplateDeletionResult}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /api/admin/v1/template/{code} [delete]
func NewTemplateDeletionEndpoint(deleter templateDeleter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		if code == "" {
			response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("code is required"))
			return
		}
		if err := deleter.Delete(r.Context(), code); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, template.ErrTemplateNotFound) {
				status = http.StatusNotFound
			}
			response.WriteErrorResponse(w, r, status, err)
			return
		}
		response.WriteSuccessResponse(w, r, http.StatusOK, TemplateDeletionResult{Code: code})
	})
}
