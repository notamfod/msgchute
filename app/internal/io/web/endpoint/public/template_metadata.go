package public

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/devian2011/msgchute/pkg/http/response"
	"github.com/go-chi/chi/v5"
)

type templateMetadataPatcher interface {
	PatchMetadata(context.Context, string, dto.TemplateMetadata) (dto.TemplateMetadata, error)
}

// NewTemplateMetadataEndpoint partially updates client annotations.
// @Summary Merge template metadata
// @Description Objects merge recursively, null deletes keys, arrays and scalars replace values. No per-client ownership enforcement.
// @Tags public.templates
// @Accept json
// @Produce json
// @Param code path string true "Stable template code"
// @Param request body dto.TemplateMetadata true "Metadata merge patch, maximum 64 KiB"
// @Success 200 {object} response.Response{data=dto.TemplateMetadata}
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 413 {object} response.Response
// @Router /api/v1/templates/{code}/metadata [patch]
func NewTemplateMetadataEndpoint(patcher templateMetadataPatcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, template.MetadataLimit)
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		var patch dto.TemplateMetadata
		err := decoder.Decode(&patch)
		if err == nil {
			var extra any
			if nextErr := decoder.Decode(&extra); nextErr != io.EOF {
				err = nextErr
				if err == nil {
					err = errors.New("expected one JSON object")
				}
			}
		}
		if err != nil || patch == nil {
			if err == nil {
				err = errors.New("metadata must be a JSON object")
			}
			status := http.StatusBadRequest
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				status = http.StatusRequestEntityTooLarge
			}
			response.WriteErrorResponse(w, r, status, err)
			return
		}
		code := chi.URLParam(r, "code")
		if code == "" {
			response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("code is required"))
			return
		}
		metadata, err := patcher.PatchMetadata(r.Context(), code, patch)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, template.ErrTemplateNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, template.ErrMetadataTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			response.WriteErrorResponse(w, r, status, err)
			return
		}
		response.WriteSuccessResponse(w, r, http.StatusOK, metadata)
	})
}
