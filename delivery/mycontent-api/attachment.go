package mycontentapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/blob"
	entity "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

// maybe later change it to mycontentapi

// TODO might be in different folder

type uploadService struct {
	*service[*entity.Attachment]
	uc           mycontent.Attachable[*entity.Attachment]
	cacheControl string
}

// Behave exatctly like the other API, but
// Can only do repository "Put" via Upload API
func NewAttachment(
	base mycontent.UsecaseAttachment[*entity.Attachment],
	blobRepo blob.Repository,
	baseURL string,
	refParams []string,
	hideUrl bool,
	namespace string, // in blob storage
	cacheControl string,
) *uploadService {
	whitelistParams := map[string]struct{}{
		"id":   {},
		"data": {},
	}
	for _, refParams := range refParams {
		whitelistParams[refParams] = struct{}{}
	}

	return &uploadService{
		service: &service[*entity.Attachment]{
			uc:              base,
			refParams:       refParams,
			whitelistParams: whitelistParams,
			postProcess: []PostProcess[*entity.Attachment]{
				FormatURL[*entity.Attachment](baseURL, refParams),
			},
		},
		uc:           base, // uc with advanced functionality
		cacheControl: cacheControl,
	}
}

func (i *uploadService) Get(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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

	isData := r.URL.Query().Get("data")

	if isData != "true" {
		i.service.Get(w, r, p)
		return
	}

	if ID == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "INVALID_PARAMS", Message: "You specify data=true but does not provide 'id' parameter"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	payload, meta, errUC := i.uc.GetAttachment(r.Context(), namespace, refIDs, ID)
	if errUC != nil {
		errMessage := serializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}
	defer payload.Close()

	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.ContentSize, 10))
	if i.cacheControl != "" {
		w.Header().Set("Cache-Control", strconv.FormatInt(meta.ContentSize, 10))
	}
	w.WriteHeader(http.StatusOK)

	var x int64
	_, err := io.Copy(w, payload)
	if err == io.EOF {
		return
	}
	if err != nil {
		log.Err(err).Msgf("Error when transfering file. %v/%v out of bytes read", x, meta.ContentSize)
		return
	}
}

func (i *uploadService) Upload(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Read body parse entity and extract metadata
	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestLengthAttachment)

	reader, err := r.MultipartReader()
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Message: "Failed to read as multipart/form-data", Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	part, err := reader.NextPart()
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Message: "Expecting data with form name 'document'", Code: "BAD_REQUEST"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}
	if part.FormName() != "document" {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Message: "Need to specify 'document' field in the first part of requeset", Code: "BAD_REQUEST"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}
	_doc, err := io.ReadAll(io.LimitReader(part, 200<<10)) // 200 Kb docs
	if err != nil {
		log.Err(err).Msgf("Some error happened when reading data")
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

	attachmentData := &entity.Attachment{}
	err = json.Unmarshal(_doc, attachmentData)
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse body (attachment API). Make sure file size does not exceed 200 Kb: " + err.Error(), Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	if attachmentData.Namespace() == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_NAMESPACE", Message: "Please specify 'Namespace'"},
			},
		})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(d)
		return
	}

	part, err = reader.NextPart()
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Message: "Expecting data with form name 'attachment'", Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	if part.FormName() != "attachment" {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Message: "Expecting data with form name 'attachment'", Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	contentType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil {
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusInternalServerError, Message: "Failed to parse content type", Code: "BAD_REQUEST"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// sniff media type, don't follow blindly input
	headerRead := bytes.NewBuffer(make([]byte, 0, 512))
	teeReader := io.TeeReader(part, headerRead)

	header := make([]byte, 512)
	_, err = teeReader.Read(header)
	if err != nil && err != io.EOF {
		log.Err(err).Str("namespace", attachmentData.Namespace()).Msgf("Failed to read. Declared ct %v. Header %v", contentType, string(header))
		errMessage := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusInternalServerError, Message: "Failed to parse media type", Code: "SERVER_ERROR"},
			},
		},
		)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	attachmentData.ContentType = http.DetectContentType(header)
	if attachmentData.Name == "" {
		attachmentData.Name = part.FileName()
	}
	if attachmentData.ContentType == "application/octet-stream" && contentType != "" {
		attachmentData.ContentType = contentType
	}
	if attachmentData.ContentSize == 0 && part.Header.Get("Content-Length") != "" {
		size, _ := strconv.ParseInt(part.Header.Get("Content-Length"), 10, 64)
		attachmentData.ContentSize = size
	}
	// TODO: validate name
	// TODO: validate everything to make it secure

	multi := io.MultiReader(headerRead, part)

	result, errUC := i.uc.Attach(r.Context(), attachmentData, multi)
	if errUC != nil {
		d := serializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(d)
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
