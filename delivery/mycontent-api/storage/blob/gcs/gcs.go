package gcs

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"cloud.google.com/go/storage"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/blob"
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

func (h *handler) Upload(ctx context.Context, objectPath string, contentType string, payload io.Reader) (*blob.Data, error) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		return nil, fmt.Errorf("empty bucket")
	}

	object := bucket.Object(objectPath)
	objWriter := object.NewWriter(ctx)
	objWriter.ContentType = contentType
	// objWriter.Name = filepath.Base(objectPath)

	length, err := io.Copy(objWriter, payload)
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		message := "error when writing to data storage. Writen '" + strconv.FormatInt(length, 10) + "' bytes of data to '" + objectPath + "' before error"
		return nil, fmt.Errorf("%w: failed to upload. "+message, err)
	}

	err = objWriter.Close()
	if err != nil {
		// generic message for user.
		// we don't want users know where do we store data
		return nil, fmt.Errorf("%w: error when closing obj writer", err)
	}

	return &blob.Data{
		PublicURL:   h.basePublicUrl + "/" + objectPath,
		ContentSize: length,
	}, nil
}

// Delete generic binary at path
func (h *handler) Delete(ctx context.Context, path string) (*blob.Data, error) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		return nil, fmt.Errorf("empty bucket")
	}

	object := bucket.Object(path)
	err := object.Delete(ctx)
	if err != nil && err.Error() != "storage: object name is empty" {
		return nil, fmt.Errorf("%w: failed to delete object", err)
	}

	return &blob.Data{
		Path: path,
	}, nil
}

// Get the data
// Better just use the public URL,
// But if the data is small & meant to be private then can use this
func (h *handler) Get(ctx context.Context, path string) (io.ReadCloser, *blob.Data, error) {
	bucket := h.gcsClient.Bucket(h.bucketName)
	if bucket == nil {
		return nil, nil, fmt.Errorf("empty bucket")
	}

	object := bucket.Object(path)
	objReader, err := object.NewReader(ctx)
	if err != nil && err.Error() != "storage: object name is empty" {
		return nil, nil, fmt.Errorf("%w: server error when getting storage data", err)
	}

	return objReader, nil, nil
}
