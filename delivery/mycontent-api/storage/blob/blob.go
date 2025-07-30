package blob

import (
	"context"
	"io"

	types "github.com/desain-gratis/common/types/http"
)

type Repository interface {
	// Upload generic binary to path
	// Path is internal address
	Upload(ctx context.Context, path string, contentType string, payload io.Reader) (*Data, *types.CommonError)

	// Delete generic binary at path
	Delete(ctx context.Context, path string) (*Data, *types.CommonError)

	// Get the data
	// Better just use the public URL,
	// But if the data is small & meant to be private then can use this
	Get(ctx context.Context, path string) (io.ReadCloser, *Data, *types.CommonError)
}

type Data struct {
	// The location of the data in the repository
	Path        string
	PublicURL   string
	ContentType string
	ContentSize int64
}
