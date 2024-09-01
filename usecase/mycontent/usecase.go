package mycontent

import (
	"context"
	"io"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type Usecase[T any] interface {
	// Put (create new or overwrite) resource here
	Put(ctx context.Context, data T) (T, *types.CommonError)

	// Get all of your resource for your user ID here
	Get(ctx context.Context, userID string, mainRefID string, ID string) ([]T, *types.CommonError)

	// Delete your resource here
	// the implementation can check whether there are linked resource or not
	Delete(ctx context.Context, userID string, ID string) (T, *types.CommonError)
}

type Attachable[T any] interface {
	// Attach generic binary to path
	// Path is internal address
	Attach(ctx context.Context, meta T, payload io.Reader) (T, *types.CommonError)

	GetAttachment(ctx context.Context, userID string, mainRefID string, ID string) (payload io.ReadCloser, meta T, err *types.CommonError)
}

// Data is the main data structure used in the my content usecase
type Data interface {
	any

	ID[Data]
	Locatable[Data]
	Ownership[Data]
	Created[Data]
	Temporal[Data]
	MainRefID
}

// Mutator or modifier, fluent style
// It enables the usecase to get and modify the underlying data
type ID[T any] interface {
	WithID(id string) T
	ID() string
}

type Locatable[T any] interface {
	URL() string
	WithURL(url string) T
}

type MainRefID interface {
	MainRefID() string
}

type Ownership[T any] interface {
	WithOwnerID(id string) T
	OwnerID() string
}

type Created[T any] interface {
	WithCreatedTime(t time.Time) T
	CreatedTime() time.Time
}

type Temporal[T any] interface {
	WithStartTime(t time.Time) T
	StartTime() time.Time
	WithEndTime(t time.Time) T
	EndTime() time.Time
}

type UpdateHook[T any] interface {
	OnDelete(context.Context, T) *types.CommonError
	OnUpdate(context.Context, T, T) *types.CommonError
}
