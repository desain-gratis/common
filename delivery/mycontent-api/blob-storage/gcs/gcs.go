package gcs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"cloud.google.com/go/storage"
	"github.com/rs/zerolog/log"

	"github.com/desain-gratis/common/delivery/mycontent-api/blob-storage"
	types "github.com/desain-gratis/common/types/http"
)

var _ blob.Repository = &handler{}

type handler struct {
	gcsClient     *storage.Client // TODO MOVE TO UTILS
	bucketName    string
	basePublicUrl string
}

func New(
	bucketName string,
	basePublicUrl string,
) *handler {
	client, _ := storage.NewClient(context.Background()) // TODO
	return &handler{
		gcsClient:     client,
		bucketName:    bucketName,
		basePublicUrl: basePublicUrl,
	}
}

func (h *handler) Upload(ctx context.Context, objectPath string, contentType string, payload io.Reader) (*blob.Data, *types.CommonError) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		log.Err(errors.New("empty bucket")).Msgf("Cannot get gcs bucket %v", h.bucketName)
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "UPLOAD_FAILED",
					Message:  "Server error when accessing storage",
				},
			},
		}
	}

	object := bucket.Object(objectPath)
	objWriter := object.NewWriter(ctx)
	objWriter.ContentType = contentType
	// objWriter.Name = filepath.Base(objectPath)

	length, err := io.Copy(objWriter, payload)
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		message := "Error when writing to data storage. Writen '" + strconv.FormatInt(length, 10) + "' bytes of data to '" + objectPath + "' before error"
		log.Err(err).Msgf("Error when writing data to object in GCS")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusFailedDependency,
					Code:     "UPLOAD_FAILED",
					Message:  message,
				},
			},
		}
	}
	err = objWriter.Close()
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		log.Err(err).Msgf("Error when finish writing data to object in GCS")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusFailedDependency,
					Code:     "UPLOAD_FAILED",
					Message:  "Server error when closing connection to storage",
				},
			},
		}
	}

	return &blob.Data{
		PublicURL:   h.basePublicUrl + "/" + objectPath,
		ContentSize: length,
	}, nil
}

// Delete generic binary at path
func (h *handler) Delete(ctx context.Context, path string) (*blob.Data, *types.CommonError) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		log.Err(errors.New("Empty bucket")).Msgf("Cannot get gcs bucket %v", h.bucketName)
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "DELETE_FAILED",
					Message:  "Server error when accessing storage",
				},
			},
		}
	}

	object := bucket.Object(path)
	err := object.Delete(ctx)
	if err != nil && err.Error() != "storage: object name is empty" {
		log.Err(err).Msgf("Cannot delete object at %v", path)
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "DELETE_FAILED",
					Message:  "Server error when delete storage data",
				},
			},
		}
	}

	return &blob.Data{
		Path: path,
	}, nil
}

// Get the data
// Better just use the public URL,
// But if the data is small & meant to be private then can use this
func (h *handler) Get(ctx context.Context, path string) (io.ReadCloser, *blob.Data, *types.CommonError) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		log.Err(errors.New("Empty bucket")).Msgf("Cannot get gcs bucket %v", h.bucketName)
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "GET_FAILED",
					Message:  "Server error when accessing storage",
				},
			},
		}
	}

	object := bucket.Object(path)
	objReader, err := object.NewReader(ctx)
	if err != nil && err.Error() != "storage: object name is empty" {
		log.Err(err).Msgf("Cannot delete object at %v", path)
		return nil, nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "GET_FAILED",
					Message:  "Server error when get storage data",
				},
			},
		}
	}

	return objReader, nil, nil
}
