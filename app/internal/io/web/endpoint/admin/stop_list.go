package admin

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/go-chi/chi/v5"

	"github.com/devian2011/msgchute/internal/dto"
	"github.com/devian2011/msgchute/internal/service/stoplist"
	"github.com/devian2011/msgchute/pkg/http/pagination"
	"github.com/devian2011/msgchute/pkg/http/response"
)

const stopListRequestLimit = 16 << 10

type stopListHandler interface {
	Create(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error)
	Delete(context.Context, string) error
	Find(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error)
	List(context.Context, dto.StopListFilter) ([]dto.StopListEntry, uint64, error)
	Upsert(context.Context, *dto.StopListEntry) (*dto.StopListEntry, error)
}

func (e *StopListEndpoint) Subscriptions(w http.ResponseWriter, r *http.Request) {
	page, perPage := stopListPage(r)
	items, total, err := e.h.List(r.Context(), dto.StopListFilter{Search: r.URL.Query().Get("search"), Limit: perPage, Offset: (page - 1) * perPage})
	if err != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, map[string]any{"items": items, "pagination": pagination.PageData{CurrentPage: page, PerPage: perPage, Total: total, TotalPages: uint64(pagination.GetPageCount(int(perPage), int(total)))}})
}

func (e *StopListEndpoint) PutSubscription(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, stopListRequestLimit)
	var request struct {
		Kind          string                `json:"kind"`
		Recipient     string                `json:"recipient"`
		Subscriptions *dto.SubscriptionList `json:"subscriptions"`
		BlockedAll    *bool                 `json:"blocked_all"`
		Reason        string                `json:"reason"`
	}
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&request); err != nil || request.Subscriptions == nil || request.BlockedAll == nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("invalid request"))
		return
	}
	entry, err := e.h.Upsert(r.Context(), &dto.StopListEntry{Kind: request.Kind, Recipient: request.Recipient, Subscriptions: *request.Subscriptions, BlockedAll: *request.BlockedAll, Reason: request.Reason})
	if errors.Is(err, stoplist.ErrInvalidEntry) || errors.Is(err, stoplist.ErrInvalidRecipient) || errors.Is(err, stoplist.ErrInvalidSubscriptions) || errors.Is(err, stoplist.ErrReasonTooLong) {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("invalid subscription preferences"))
		return
	}
	if err != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, entry)
}

type StopListEndpoint struct{ h stopListHandler }

func NewStopListEndpoint(h stopListHandler) *StopListEndpoint { return &StopListEndpoint{h: h} }

func (e *StopListEndpoint) List(w http.ResponseWriter, r *http.Request) {
	page, perPage := stopListPage(r)
	items, total, err := e.h.Find(r.Context(), dto.StopListFilter{Search: r.URL.Query().Get("search"), Limit: perPage, Offset: (page - 1) * perPage})
	if err != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, map[string]any{"items": items, "pagination": pagination.PageData{CurrentPage: page, PerPage: perPage, Total: total, TotalPages: uint64(pagination.GetPageCount(int(perPage), int(total)))}})
}

func (e *StopListEndpoint) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, stopListRequestLimit)
	var request struct {
		Kind      string `json:"kind"`
		Recipient string `json:"recipient"`
		Reason    string `json:"reason"`
	}
	if err := sonic.ConfigDefault.NewDecoder(r.Body).Decode(&request); err != nil {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("invalid request"))
		return
	}
	created, err := e.h.Create(r.Context(), &dto.StopListEntry{Kind: request.Kind, Recipient: request.Recipient, Reason: request.Reason})
	if errors.Is(err, stoplist.ErrInvalidRecipient) {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("invalid recipient"))
		return
	}
	if errors.Is(err, stoplist.ErrReasonTooLong) {
		response.WriteErrorResponse(w, r, http.StatusBadRequest, errors.New("reason is too long"))
		return
	}
	if errors.Is(err, stoplist.ErrAlreadyExists) {
		response.WriteErrorResponse(w, r, http.StatusConflict, errors.New("stop list entry already exists"))
		return
	}
	if err != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, created)
}

func (e *StopListEndpoint) Delete(w http.ResponseWriter, r *http.Request) {
	err := e.h.Delete(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, stoplist.ErrNotFound) {
		response.WriteErrorResponse(w, r, http.StatusNotFound, errors.New("stop list entry not found"))
		return
	}
	if err != nil {
		response.WriteErrorResponse(w, r, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	response.WriteSuccessResponse(w, r, http.StatusOK, map[string]any{})
}

func stopListPage(r *http.Request) (uint64, uint64) {
	parse := func(key string, fallback uint64) uint64 {
		value, err := strconv.ParseUint(r.URL.Query().Get(key), 10, 64)
		if err != nil || value == 0 {
			return fallback
		}
		return value
	}
	page, perPage := parse("page", 1), parse("per_page", 20)
	if perPage > 100 {
		perPage = 100
	}
	if page-1 > math.MaxInt64/perPage {
		page = math.MaxInt64/perPage + 1
	}
	return page, perPage
}
