package s3

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	blob "github.com/desain-gratis/common/delivery/mycontent-api/storage/blob"
	"github.com/desain-gratis/common/types/entity"
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

func (h *handler) Upload(ctx context.Context, objectPath string, attachment *entity.Attachment, payload io.Reader) (*blob.Data, error) {
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		return nil, fmt.Errorf("%w: failure when accessing storage or bucket not exist %v", err, !exists)
	}

	// TODO: -1 uses memory they said..
	info, err := h.client.PutObject(ctx, h.bucketName, objectPath, payload, int64(attachment.ContentSize), minio.PutObjectOptions{
		ContentType: attachment.ContentType,
	})
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		return nil, fmt.Errorf("%w: failed to put object %v", err, ctx.Err())
	}

	return &blob.Data{
		PublicURL:   h.basePublicUrl + "/" + objectPath,
		Path:        objectPath,
		ContentType: attachment.ContentType,
		ContentSize: info.Size,
	}, nil
}

// Delete generic binary at path
func (h *handler) Delete(ctx context.Context, path string) (*blob.Data, error) {
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		return nil, fmt.Errorf("%w: (delete) failure when accessing storage or bucket not exist %v", err, !exists)
	}

	err = h.client.RemoveObject(ctx, h.bucketName, path, minio.RemoveObjectOptions{})
	if err != nil && err.Error() != "storage: object name is empty" {
		return nil, fmt.Errorf("%w: server error during delete", err)
	}

	return &blob.Data{
		Path: path,
	}, nil
}

// Get the data
// Better just use the public URL,
// But if the data is small & meant to be private then can use this
func (h *handler) Get(ctx context.Context, path string) (io.ReadCloser, *blob.Data, error) {
	exists, err := h.client.BucketExists(ctx, h.bucketName)
	if !exists || err != nil {
		return nil, nil, fmt.Errorf("%w: failure when accessing storage or bucket not exist %v", err, !exists)
	}

	object, err := h.client.GetObject(ctx, h.bucketName, path, minio.GetObjectOptions{})
	if err != nil && err.Error() != "storage: object name is empty" {
		return nil, nil, fmt.Errorf(
			"%w: cannot get object at path %v", err, path)
	}

	return object, nil, nil
}
