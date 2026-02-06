package content

import (
	"context"
	"errors"
)

var (
	ErrNotFound   = errors.New("content not found")
	ErrInvalidKey = errors.New("invalid key")
)

type Repository interface {
	Post(ctx context.Context, namespace string, refIDs []string, ID string, data Data) (Data, error)

	// Get daya by owner ID
	Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]Data, error)

	// Delete specific ID data. If no data, MUST return error
	Delete(ctx context.Context, namespace string, refIDs []string, ID string) (Data, error)

	// Stream Get data
	Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan Data, error)

	// TODO: add ref size
	// RefSize() int // Map key value for param / metadata
}

type Data struct {
	// Incremental value for "log" storage for OLAP maxxing
	EventID uint64

	// For all content ID, we have incremental value
	// might not be not needed, since it can be put inside ID
	// and it's written at Post time
	// Version uint64

	// The location of the data in the repository
	Namespace string
	RefIDs    []string
	ID        string

	// The actual data
	Data []byte
	Meta []byte
}
