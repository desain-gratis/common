package content

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

type Repository interface {
	Post(ctx context.Context, namespace string, refIDs []string, ID string, data Data) (Data, *types.CommonError)

	// Get daya by owner ID
	Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]Data, *types.CommonError)

	// Delete specific ID data. If no data, MUST return error
	Delete(ctx context.Context, namespace string, refIDs []string, ID string) (Data, *types.CommonError)

	// Stream Get data
	Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan Data, *types.CommonError)
}

type Data struct {
	// The location of the data in the repository
	Namespace string
	RefIDs    []string
	ID        string

	// The actual data
	Data []byte
	Meta []byte
}
