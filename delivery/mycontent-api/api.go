package mycontentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	types "github.com/desain-gratis/common/types/http"
)

const maximumRequestLength = 1 << 20
const maximumRequestLengthAttachment = 100 << 20

type service[T mycontent.Data] struct {
	uc              mycontent.Usecase[T]
	refParams       []string
	whitelistParams map[string]struct{}
	postProcess     []PostProcess[T]
}

type PostProcess[T mycontent.Data] func(t T)

func NewFromStorage[T mycontent.Data](baseURL string, refParams []string, store content.Repository, refSize int) *service[T] {
	base := mycontent_base.New[T](store, refSize) // todo use refSize from store
	return New(base, baseURL, refParams)
}

func NewFromStorageVersioned[T mycontent.VersionedData](baseURL string, refParams []string, store content.Repository, refSize int) *service[T] {
	base := mycontent_base.NewVersioned[T](store, refSize) // todo use refSize from store
	return New(base, baseURL, refParams)
}

func New[T mycontent.Data](
	uc mycontent.Usecase[T],
	baseURL string,
	refParams []string,
) *service[T] {
	whitelistParams := map[string]struct{}{
		"id": {},
	}
	for _, refParams := range refParams {
		whitelistParams[refParams] = struct{}{}
	}

	return &service[T]{
		uc:              uc,
		refParams:       refParams,
		whitelistParams: whitelistParams,
		postProcess: []PostProcess[T]{
			FormatURL[T](baseURL, refParams),
		},
	}
}

func (i *service[T]) Post(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Read body parse entity and extract metadata

	if len(r.URL.Query()) > 0 {
		handleError(w, "BAD_REQUEST", "URL Parameter should not be specified", http.StatusBadRequest, nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestLength)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(w, "SERVER_ERROR", "failed to read payload", http.StatusInternalServerError, err)
		return
	}

	var resource T
	err = json.Unmarshal(payload, &resource)
	if err != nil {
		handleError(
			w, "BAD_REQUEST", "failed to parse body. Make sure file size does not exceed 200Kb",
			http.StatusBadRequest, nil)
		return
	}

	err = resource.Validate()
	if err != nil {
		handleError(
			w, "BAD_REQUEST", fmt.Sprintf("validation errors: %v.", err),
			http.StatusBadRequest, nil)
		return
	}

	result, err := i.uc.Post(r.Context(), resource, map[string]string{
		"created_at": time.Now().Format(time.RFC3339),
	})
	if err != nil {
		handlePostError(w, err)
		return
	}

	// post-process
	for _, pp := range i.postProcess {
		pp(result)
	}

	// since we wrap using types.CommonResponse, it cannot use protojson to unmarshal
	// for now can remove omitempty manually in the generated proto, based on the usecase
	// or trade offs, convert common response / common error to their proto counterpart
	// (either use adaptar or change the whole code)
	// let's assess them later
	// eg. (the need for, for example, price 0) to be shown. or we can just determine implicitly
	payload, err = json.Marshal(&types.CommonResponse{
		Success: &result,
	})
	if err != nil {
		handleError(
			w, "SERVER_ERROR", "server encounter an error",
			http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

func (i *service[T]) Get(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	namespace := r.Header.Get("X-Namespace")
	if namespace == "" {
		handleError(
			w, "BAD_REQUEST", "'X-Namespace' header is empty",
			http.StatusBadRequest, nil)
		return
	}

	invalidParams := validateParams(i.whitelistParams, r.URL.Query())
	if len(invalidParams) > 0 {
		handleError(
			w, "BAD_REQUEST", "invalid parameter(s): "+strings.Join(invalidParams, ","),
			http.StatusBadRequest, nil)
		return
	}

	ID := r.URL.Query().Get("id")
	refIDs := make([]string, 0, len(i.refParams))
	for _, param := range i.refParams {
		refIDs = append(refIDs, r.URL.Query().Get(param))
	}

	// Actually get the data
	result, err := i.uc.Get(r.Context(), namespace, refIDs, ID)
	if err != nil {
		handleGetError(w, err)
		return
	}

	for _, pp := range i.postProcess {
		for idx := range result {
			pp(result[idx])
		}
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: result,
	})

	if err != nil {
		handleError(
			w, "SERVER_ERROR", "server encounter an error",
			http.StatusInternalServerError, err)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

func (i *service[T]) Delete(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	namespace := r.Header.Get("X-Namespace")
	if namespace == "" {
		handleError(
			w, "BAD_REQUEST", "'X-Namespace' header is empty",
			http.StatusBadRequest, nil)
		return
	}

	invalidParams := validateParams(i.whitelistParams, r.URL.Query())
	if len(invalidParams) > 0 {
		handleError(
			w, "BAD_REQUEST", "invalid parameter(s): "+strings.Join(invalidParams, ","),
			http.StatusBadRequest, nil)
		return
	}

	ID := r.URL.Query().Get("id")
	refIDs := make([]string, 0, len(i.refParams))
	for _, param := range i.refParams {
		refIDs = append(refIDs, r.URL.Query().Get(param))
	}

	// Get the data first.
	getBeforeDeleteResult, err := i.uc.Get(r.Context(), namespace, refIDs, ID)
	if err != nil {
		handleGetError(w, err)
		return
	}

	if len(getBeforeDeleteResult) != 1 {
		handleError(w, "BAD_REQUEST", "not found", http.StatusNotFound, nil)
		return
	}

	// Do the actual deletion
	result, err := i.uc.Delete(r.Context(), namespace, refIDs, ID)
	if err != nil {
		handleDeleteError(w, err)
		return
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: &result,
	})

	if err != nil {
		log.Err(err).Msgf("Failed to parse payload")
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse response", Code: "SERVER_ERROR"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)

}

func serializeError(err *types.CommonError) []byte {
	d, errMarshal := json.Marshal(&types.CommonResponse{
		Error: err,
	})
	if errMarshal != nil {
		log.Err(errMarshal).Msgf("Failed to parse err")
	}
	return d
}

func validateParams(whitelisted map[string]struct{}, params url.Values) (invalidParams []string) {
	for param := range params {
		if _, ok := whitelisted[param]; !ok {
			invalidParams = append(invalidParams, param)
		}
	}
	return invalidParams
}

func handlePostError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mycontent.ErrValidation):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, mycontent.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	case errors.Is(err, content.ErrInvalidKey):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, content.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	default:
		handleError(w, "SERVER_ERROR", "server error", http.StatusInternalServerError, err)
	}
}

func handleGetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mycontent.ErrValidation):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, mycontent.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	case errors.Is(err, content.ErrInvalidKey):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, content.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	default:
		handleError(w, "SERVER_ERROR", "server error", http.StatusInternalServerError, err)
	}
}

func handleDeleteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mycontent.ErrValidation):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, mycontent.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	case errors.Is(err, content.ErrInvalidKey):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	case errors.Is(err, content.ErrNotFound):
		handleError(w, "NOT_FOUND", err.Error(), http.StatusNotFound, nil)
	default:
		handleError(w, "SERVER_ERROR", "server error", http.StatusInternalServerError, err)
	}
}

func handleError(w http.ResponseWriter, code, msg string, httpStatus int, err error) {
	if err != nil {
		slog.Error("failed to serve request", slog.String("error", err.Error()))
	}

	w.WriteHeader(http.StatusInternalServerError)
	message := serializeError(&types.CommonError{
		Errors: []types.Error{
			{Message: msg, Code: code, HTTPCode: httpStatus},
		},
	})

	w.Write(message)
}
