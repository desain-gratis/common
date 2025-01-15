package mycontentapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

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
	myContentUC  mycontent.Usecase[T]
	refIDsParser func(url.Values) []string
}

func New[T mycontent.Data](
	repo content.Repository,
	validate func(T) *types.CommonError,
	refIDsParser func(url.Values) []string,
	urlFormat mycontent_crud.URLFormat,
) *service[T] {
	uc := mycontent_crud.New(
		repo,
		validate,
		urlFormat,
	)

	return &service[T]{
		myContentUC:  uc,
		refIDsParser: refIDsParser,
	}
}

func (i *service[T]) Put(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Read body parse entity and extract metadata

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

	result, errUC := i.myContentUC.Put(r.Context(), resource)
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
	var userID string
	var ID string

	userID = r.URL.Query().Get("user_id")
	if userID == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_USER_ID", Message: "Please specify 'user_id'"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	ID = r.URL.Query().Get("id")
	refIDs := i.refIDsParser(r.URL.Query())

	result, errUC := i.myContentUC.Get(r.Context(), userID, refIDs, ID)
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
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_USER_ID", Message: "Please specify 'user_id'"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	ID := r.URL.Query().Get("id")

	result, errUC := i.myContentUC.Delete(r.Context(), userID, ID)
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
