package blob

import (
	"context"
	"io"
)

type Repository interface {
	// Upload generic binary to path
	// Path is internal address
	Upload(ctx context.Context, path string, contentType string, payload io.Reader) (*Data, error)

	// Delete generic binary at path
	Delete(ctx context.Context, path string) (*Data, error)

	// Get the data
	// Better just use the public URL,
	// But if the data is small & meant to be private then can use this
	Get(ctx context.Context, path string) (io.ReadCloser, *Data, error)
}

type Data struct {
	// The location of the data in the repository
	Path        string
	PublicURL   string
	ContentType string
	ContentSize int64
}
