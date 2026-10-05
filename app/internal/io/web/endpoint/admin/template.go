package admin

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/go-playground/form"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/template"
	"github.com/devian2011/msgchute/pkg/http/pagination"
	"github.com/devian2011/msgchute/pkg/http/response"
	"github.com/devian2011/msgchute/pkg/http/sort"
)

// templateFinderHandler Template finder

type TemplateFinderResult struct {
	Templates  map[string]*dto.Template `json:"templates"`
	Pagination pagination.PageData      `json:"pagination"`
}

type TemplateFilterRequest struct {
	Systems  []string `form:"system"`
	Channels []string `form:"channel"`
	Code     []string `form:"code"`
	Search   *string  `form:"search"`
}

type templateFinderHandler interface {
	Handle(filter *dto.MessageTemplateFilter) (map[string]*dto.Template, uint64, error)
}

type TemplateFinderEndpoint struct {
	h       templateFinderHandler
	decoder *form.Decoder
}

func NewTemplateFinderEndpoint(h templateFinderHandler) *TemplateFinderEndpoint {
	return &TemplateFinderEndpoint{
		h:       h,
		decoder: form.NewDecoder(),
	}
}

// ServeHTTP lists and filters message templates
//
//	@Summary		List and filter templates
//	@Description	Retrieves a paginated collection of message templates filtered by system code arrays or full-text query matches.
//	@Tags			admin.templates
//	@Accept			x-www-form-urlencoded
//	@Produce		json
//	@Param			page		query		int						false	"Page index parameter"		default(1)
//	@Param			per_page	query		int						false	"Page item capacity limit"	default(10)
//	@Param			sort		query		string					false	"Target field criteria for ordering records"
//	@Param			order		query		string					false	"Ascending or descending order selection (asc/desc)"
//	@Param			code		query		[]string				false	"Filter metrics by unique message system code tokens"	collectionFormat(multi)
//	@Param			search		query		string					false	"Generic phrase match expression matching structural text"
//
// @Param system query []string false "System tags (match any)" collectionFormat(multi)
// @Param channel query []string false "Channel tags (match any)" collectionFormat(multi)
//
//	@Success		200			{object}	TemplateFinderResult	"Key-value dictionary mapping matching code IDs to schema definitions"
//	@Failure		400			{object}	response.Response		"Query validation or structured query parameter parse errors"
//	@Failure		500			{object}	response.Response		"Internal catalog resolution context errors"
//	@Router			/api/admin/v1/template [get]
func (e *TemplateFinderEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	page, perPage := pagination.GetPageDataFromRequest(r)
	limit, offset := pagination.GetLimitsByPage(page, perPage)
	respSort := sort.GetSortFromRequest(r)

	// Default respSort.Field == code
	if respSort.Field == "id" {
		respSort.Field = "code"
	}

	filter := &dto.MessageTemplateFilter{
		Limit:     limit,
		Offset:    offset,
		SortField: &respSort.Field,
		SortOrder: &respSort.Order,
	}

	var filterRequest TemplateFilterRequest
	if err := e.decoder.Decode(&filterRequest, r.URL.Query()); err != nil {
		http.Error(w, "Invalid query parameters", http.StatusBadRequest)
		return
	}

	filter.Code = filterRequest.Code
	filter.Systems = filterRequest.Systems
	filter.Channels = filterRequest.Channels
	filter.Search = filterRequest.Search

	templates, totalCnt, getErr := e.h.Handle(filter)
	if getErr != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, getErr)
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, TemplateFinderResult{
		Templates: templates,
		Pagination: pagination.PageData{
			CurrentPage: page,
			PerPage:     perPage,
			Total:       totalCnt,
			TotalPages:  uint64(pagination.GetPageCount(int(perPage), int(totalCnt))),
		},
	})
}

// Template creation

type templateCreationHandler interface {
	Handle(template *dto.Template) (*dto.Template, error)
}
type TemplateCreationEndpoint struct {
	h templateCreationHandler
}

func NewTemplateCreationEndpoint(h templateCreationHandler) *TemplateCreationEndpoint {
	return &TemplateCreationEndpoint{h: h}
}

// ServeHTTP registers a new message template definitions model
//
//	@Summary		Create a template
//	@Description	Creates and stores a completely new message configuration blueprint in the repository.
//	@Tags			admin.templates
//	@Accept			json
//	@Produce		json
//	@Param			request	body		dto.Template		true	"Structural definition specification metrics payload"
//	@Success		200		{object}	dto.Template		"Newly registered template model confirmation metadata"
//	@Failure		400		{object}	response.Response	"Corrupted JSON payload body formatting syntax exception"
//	@Failure		500		{object}	response.Response	"Persistence system errors encountered storing details"
//	@Router			/api/admin/v1/template [post]
//
// @Failure 409 {object} response.Response "Template code already exists"
func (e *TemplateCreationEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var definition *dto.Template
	if encodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&definition); encodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, encodeErr)
		return
	}
	tmpl, createErr := e.h.Handle(definition)
	if createErr != nil {
		status := http.StatusInternalServerError
		if errors.Is(createErr, template.ErrTemplateAlreadyExists) {
			status = http.StatusConflict
		}
		response.WriteErrorResponse(w, r, status, createErr)
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, tmpl)
}

// Template Update

type templateUpdateHandler interface {
	Handle(template *dto.Template) (*dto.Template, error)
}
type TemplateUpdateEndpoint struct {
	h templateUpdateHandler
}

func NewTemplateUpdateEndpoint(h templateUpdateHandler) *TemplateUpdateEndpoint {
	return &TemplateUpdateEndpoint{h: h}
}

// ServeHTTP updates an existing template definition entry by code
//
//	@Summary		Update a template
//	@Description	Modifies variables, subjects, or raw body layouts of an existing layout found by its explicit target code route component.
//	@Tags			admin.templates
//	@Accept			json
//	@Produce		json
//	@Param			code	path		string				true	"Target identifying tracking code token context"
//	@Param			request	body		dto.Template		true	"Updatable structural model fields"
//	@Success		200		{object}	dto.Template		"Refreshed schema specifications entry state details"
//	@Failure		400		{object}	response.Response	"Invalid path payload reference parameters or corrupt body markup values"
//	@Failure		500		{object}	response.Response	"Database or engine exceptions processing modified models"
//	@Router			/api/admin/v1/template/{code} [put]
func (e *TemplateUpdateEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	params := response.GetParams([]string{"code"}, r)
	code, exists := params["code"]
	if !exists {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, fmt.Errorf("code is required"))
		return
	}

	var template *dto.Template
	if decodeErr := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&template); decodeErr != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, decodeErr)
		return
	}
	template.Code = code

	tmpl, updateErr := e.h.Handle(template)
	if updateErr != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, updateErr)
		return
	}

	response.WriteSuccessResponse(w, r, http.StatusOK, tmpl)
}
