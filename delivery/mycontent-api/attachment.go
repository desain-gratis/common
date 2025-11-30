package mycontentapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
	entity "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
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
	baseURL string,
	refParams []string,
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

	payload, meta, err := i.uc.GetAttachment(r.Context(), namespace, refIDs, ID)
	if err != nil {
		handleGetError(w, err)
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
	_, err = io.Copy(w, payload)
	if err == io.EOF {
		return
	}
	if err != nil {
		err = fmt.Errorf("%w: error when transfering file%v/%v content transfer", err, x, meta.ContentSize)
		handleError(w, "SERVER_ERROR", "server error", http.StatusInternalServerError, err)
		return
	}
}

func (i *uploadService) Upload(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Read body parse entity and extract metadata
	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestLengthAttachment)

	reader, err := r.MultipartReader()
	if err != nil {
		handleError(w, "BAD_REQUEST", "failed to read as multipart/form-data", http.StatusBadRequest, nil)
		return
	}

	part, err := reader.NextPart()
	if err != nil {
		handleError(w, "BAD_REQUEST", "expecting data with form name 'document'", http.StatusBadRequest, nil)
		return
	}
	if part.FormName() != "document" {
		handleError(w, "BAD_REQUEST", "need to specify 'document' field in the first part of request", http.StatusBadRequest, nil)
		return
	}
	_doc, err := io.ReadAll(io.LimitReader(part, 200<<10)) // 200 Kb docs
	if err != nil {
		handleError(w, "SERVER_ERROR", "error while reading data", http.StatusInternalServerError, err)
		return
	}

	attachmentData := &entity.Attachment{}
	err = json.Unmarshal(_doc, attachmentData)
	if err != nil {
		handleError(
			w, "BAD_REQUEST", "failed to parse body. Make sure file size does not exceed 200Kb",
			http.StatusBadRequest, nil)
		return
	}

	if attachmentData.Namespace() == "" {
		handleError(
			w, "BAD_REQUEST", "attachment data namespace cannot be empty header is empty",
			http.StatusBadRequest, nil)
		return
	}

	part, err = reader.NextPart()
	if err != nil {
		handleError(
			w, "BAD_REQUEST", "expecting data next part with form name 'attachment'",
			http.StatusBadRequest, nil)
		return
	}

	if part.FormName() != "attachment" {
		handleError(
			w, "BAD_REQUEST", "expecting data with form name 'attachment'",
			http.StatusBadRequest, nil)
		return
	}

	contentType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil {
		handleError(
			w, "BAD_REQUEST", "invalid content type",
			http.StatusBadRequest, nil)
		return
	}

	// sniff media type, don't follow blindly input
	headerRead := bytes.NewBuffer(make([]byte, 0, 512))
	teeReader := io.TeeReader(part, headerRead)

	header := make([]byte, 512)
	_, err = teeReader.Read(header)
	if err != nil && err != io.EOF {
		handleError(
			w, "BAD_REQUEST", "failed to parse media type",
			http.StatusBadRequest, err)
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

	result, err := i.uc.Attach(r.Context(), attachmentData, multi)
	if err != nil {
		handleAttachError(w, err)
		return
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: &result,
	})
	if err != nil {
		handleError(
			w, "SERVER_ERROR", "server encounter an error",
			http.StatusInternalServerError, err)
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}
func handleAttachError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, content.ErrInvalidKey):
		handleError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest, nil)
	default:
		handleError(w, "SERVER_ERROR", "server error", http.StatusInternalServerError, err)
	}
}
