package s3

import (
	"context"
	"io"
	"net/http"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"

	blob "github.com/desain-gratis/common/delivery/mycontent-api/storage/blob"
	types "github.com/desain-gratis/common/types/http"
)

var _ blob.Repository = &handler{}

type handler struct {
	client        *minio.Client
	basePublicUrl string
	bucketName    string
}

func New(
	endpoint string,
	accessKeyID string,
	secretAccessKey string,
	useSSL bool,
	bucketName string,
	basePublicUrl string,
) (*handler, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	return &handler{
		client:        client,
		bucketName:    bucketName,
		basePublicUrl: basePublicUrl,
	}, nil
}

func (h *handler) Upload(ctx context.Context, objectPath string, contentType string, payload io.Reader) (*blob.Data, *types.CommonError) {
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		log.Err(err).Msgf("Cannot get s3 bucket %v", h.bucketName)
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

	// TODO: -1 uses memory they said..
	info, err := h.client.PutObject(ctx, h.bucketName, objectPath, payload, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		log.Err(err).Msgf("Error when finish writing data to object in")
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusFailedDependency,
					Code:     "UPLOAD_FAILED",
					Message:  "failed to put object",
				},
			},
		}
	}

	log.Info().Msgf("upload info: %v", info)

	return &blob.Data{
		PublicURL:   h.basePublicUrl + "/" + objectPath,
		Path:        objectPath,
		ContentType: contentType,
		ContentSize: info.Size,
	}, nil
}

// Delete generic binary at path
func (h *handler) Delete(ctx context.Context, path string) (*blob.Data, *types.CommonError) {
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		log.Err(err).Msgf("del: cannot get bucket %v", h.bucketName)
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

	err = h.client.RemoveObject(ctx, h.bucketName, path, minio.RemoveObjectOptions{})
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
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		log.Err(err).Msgf("get: cannot get bucket %v", h.bucketName)
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

	object, err := h.client.GetObject(ctx, h.bucketName, path, minio.GetObjectOptions{})
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

	return object, nil, nil
}
