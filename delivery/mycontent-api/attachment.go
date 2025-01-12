package mycontentapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/repository/blob"
	"github.com/desain-gratis/common/repository/content"
	entity "github.com/desain-gratis/common/types/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
	mycontent_crud "github.com/desain-gratis/common/usecase/mycontent/crud"
)

// maybe later change it to mycontentapi

// TODO might be in different folder

type ContentUploadMetadata struct {
	*ResourceManagerService[*entity.Attachment]
	repo         content.Repository
	uc           mycontent.Attachable[*entity.Attachment]
	cacheControl string
}

// Behave exatctly like the other API, but
// Can only do repository "Put" via Upload API
// Can on
func NewAttachment(
	repo content.Repository, // todo, change catalog.Attachment location to more common location (not uc specific)
	blobRepo blob.Repository,
	refIDsParser func(url.Values) []string,
	hideUrl bool,
	namespace string, // in blob storage
	urlFormat mycontent_crud.URLFormat,
	cacheControl string,
) *ContentUploadMetadata {

	uc := mycontent_crud.NewAttachment(
		repo,
		blobRepo,
		hideUrl,
		namespace,
		urlFormat,
	)

	return &ContentUploadMetadata{
		ResourceManagerService: &ResourceManagerService[*entity.Attachment]{
			myContentUC:  uc,
			refIDsParser: refIDsParser,
		},
		uc:           uc, // uc with advanced functionality
		cacheControl: cacheControl,
	}
}

func (i *ContentUploadMetadata) Get(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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

	isData := r.URL.Query().Get("data")

	if isData != "true" {
		i.ResourceManagerService.Get(w, r, p)
		return
	}

	payload, meta, errUC := i.uc.GetAttachment(r.Context(), userID, refIDs, ID)
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

func (i *ContentUploadMetadata) Upload(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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

	if attachmentData.OwnerId == "" {
		d := serializeError(&types.CommonError{
			Errors: []types.Error{
				{HTTPCode: http.StatusBadRequest, Code: "EMPTY_OWNER_ID", Message: "Please specify 'OwnerId'"},
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
		log.Err(err).Str("owner_id", attachmentData.OwnerId).Msgf("Failed to read. Declared ct %v. Header %v", contentType, string(header))
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

	// naise. the wrap can be considered as message!
	// result := res.(any)
	// payload, err = protojson.MarshalOptions{
	// 	UseProtoNames: true,
	// 	EmitUnpopulated: true,
	// }.Marshal(result)

	// since we wrap using types.CommonResponse, it cannot use protojson to unmarshal
	// for now can remove omitempty manually in the generated proto, based on the usecase
	// or trade offs, convert common response / common error to their proto counterpart
	// (either use adaptar or change the whole code)
	// let's assess them later
	// eg. (the need for, for example, price 0) to be shown. or we can just determine implicitly
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
