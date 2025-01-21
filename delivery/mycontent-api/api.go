package mycontentapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	mycontent_crud "github.com/desain-gratis/common/usecase/mycontent/crud"
)

const maximumRequestLength = 1 << 20
const maximumRequestLengthAttachment = 100 << 20

type service[T mycontent.Data] struct {
	myContentUC     mycontent.Usecase[T]
	refParams       []string
	whitelistParams map[string]struct{}
}

func New[T mycontent.Data](
	repo content.Repository,
	urlFormat mycontent_crud.URLFormat, // TODO: integrate
	refParams []string,
) *service[T] {
	uc := mycontent_crud.New[T](
		repo,
		urlFormat,
		refParams,
	)

	whitelistParams := map[string]struct{}{
		"id": {},
	}
	for _, refParams := range refParams {
		whitelistParams[refParams] = struct{}{}
	}

	return &service[T]{
		myContentUC: uc,
		refParams:   refParams,
	}
}

func (i *service[T]) Post(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Read body parse entity and extract metadata

	if len(r.URL.Query()) > 0 {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Please do not enter URL parameter in Post request", Code: "BAD_REQUEST", HTTPCode: http.StatusBadRequest},
			},
		},
		)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestLength)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to read all body", Code: "SERVER_ERROR"},
			},
		},
		)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	var resource T
	err = json.Unmarshal(payload, &resource)
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse body (content API). Make sure file size does not exceed 200 Kb: " + err.Error(), Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// Basically, the Use case / Repo for put is to Put Identifier to the object if not exist yet
	// If identifier already exist, previous data will be overwritten

	result, errUC := i.myContentUC.Post(r.Context(), resource, map[string]string{
		"created_at": time.Now().Format(time.RFC3339),
	})
	if errUC != nil {
		d := serializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(d)
		return
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

func (i *service[T]) Get(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	namespace := r.Header.Get("X-Namespace")
	if namespace == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_NAMESPACE", Message: "Please specify header 'X-Namespace'"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	invalidParams := validateParams(i.whitelistParams, r.URL.Query())
	if len(invalidParams) > 0 {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "INVALID_PARAMS", Message: "Invalid parameter(s):" + strings.Join(invalidParams, ",")},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	ID := r.URL.Query().Get("id")
	refIDs := make([]string, 0, len(i.refParams))
	for _, param := range i.refParams {
		refIDs = append(refIDs, r.URL.Query().Get(param))
	}

	result, errUC := i.myContentUC.Get(r.Context(), namespace, refIDs, ID)
	if errUC != nil {
		errMessage := serializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: result,
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

func (i *service[T]) Delete(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	namespace := r.Header.Get("X-Namespace")
	if namespace == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_NAMESPACE", Message: "Please specify header 'X-Namespace'"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	invalidParams := validateParams(i.whitelistParams, r.URL.Query())
	if len(invalidParams) > 0 {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "INVALID_PARAMS", Message: "Invalid parameter(s):" + strings.Join(invalidParams, ",")},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	ID := r.URL.Query().Get("id")
	refIDs := make([]string, 0, len(i.refParams))
	for _, param := range i.refParams {
		refIDs = append(refIDs, r.URL.Query().Get(param))
	}

	result, errUC := i.myContentUC.Delete(r.Context(), namespace, refIDs, ID)
	if errUC != nil {
		errMessage := serializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
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
