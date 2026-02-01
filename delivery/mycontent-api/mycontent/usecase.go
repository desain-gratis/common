package mycontent

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrValidation used to communicate input error to user
	ErrValidation = errors.New("validation")

	// ErrStorage is error in the underlying storage
	ErrStorage = errors.New("storage")

	// ErrNotFound when content is not found during Post, Delete, and Get (by ID)
	ErrNotFound = errors.New("not found")
)

type UsecaseAttachment[T any] interface {
	Usecase[T]
	Attachable[T]
}
type Usecase[T any] interface {
	// Post (create new or overwrite) resource here
	Post(ctx context.Context, data T, meta any) (T, error)

	// Get all of your resource for your user ID here
	Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, error)

	// Stream response
	Stream(ctx context.Context, namespace string, refIDs []string, ID string) (<-chan T, error)

	// Delete your resource here
	// the implementation can check whether there are linked resource or not
	Delete(ctx context.Context, namespace string, refIDs []string, ID string) (T, error)
}

type Attachable[T any] interface {
	// Attach generic binary to path
	// Path is internal address
	Attach(ctx context.Context, meta T, payload io.Reader) (T, error)

	GetAttachment(ctx context.Context, userID string, refIDs []string, ID string) (payload io.ReadCloser, meta T, err error)
}

// Data is the main data structure used in the my content usecase
type Data interface {
	ID
	Locatable
	Namespace
	Created
	RefIDs
	Validator

	// TODO: Add EventID
}

type VersionedData interface {
	Data
	WithEventID(eventID uint64) VersionedData
}

// Mutator or modifier, fluent style
// It enables the usecase to get and modify the underlying data
type ID interface {
	WithID(id string) Data
	ID() string
}

type Locatable interface {
	URL() string
	WithURL(url string) Data
}

type Namespace interface {
	WithNamespace(id string) Data
	Namespace() string
}

type Created interface {
	WithCreatedTime(t time.Time) Data
	CreatedTime() time.Time
}

// Secondary indexes of the content, allowing to be queried other than user ID, parent ID, & user ID
type RefIDs interface {
	RefIDs() []string
}

type Validator interface {
	Validate() error
}

// Todo serializable
