package content

import (
	"context"
	"net/http"
	"sync"

	types "github.com/desain-gratis/common/types/http"
)

var _ Repository[any] = &wrapper[any]{}

// Wrap repo with another repository
type wrapper[T any] struct {
	Repository[T]

	post           func(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) *types.CommonError
	put            func(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) (Data[T], *types.CommonError)
	get            func(ctx context.Context, userID, ID string, refIDs []string) ([]Data[T], *types.CommonError)
	delete         func(ctx context.Context, userID, ID string, refIDs []string) (Data[T], *types.CommonError)
	getByID        func(ctx context.Context, userID, ID string) (Data[T], *types.CommonError)
	getByMainRefID func(ctx context.Context, userID, mainRefID string) ([]Data[T], *types.CommonError)
}

func (w *wrapper[T]) Post(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) *types.CommonError {
	if w.post != nil {
		return w.post(ctx, userID, ID, refIDs, data)
	}
	return w.Repository.Post(ctx, userID, ID, refIDs, data)
}

func (w *wrapper[T]) Put(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) (Data[T], *types.CommonError) {
	if w.put != nil {
		return w.put(ctx, userID, ID, refIDs, data)
	}
	return w.Repository.Put(ctx, userID, ID, refIDs, data)
}

func (w *wrapper[T]) Get(ctx context.Context, userID, ID string, refIDs []string) ([]Data[T], *types.CommonError) {
	if w.get != nil {
		return w.get(ctx, userID, ID, refIDs)
	}
	return w.Repository.Get(ctx, userID, ID, refIDs)
}

func (w *wrapper[T]) Delete(ctx context.Context, userID, ID string, refIDs []string) (Data[T], *types.CommonError) {
	if w.delete != nil {
		return w.delete(ctx, userID, ID, refIDs)
	}
	return w.Repository.Delete(ctx, userID, ID, refIDs)
}

func (w *wrapper[T]) GetByID(ctx context.Context, userID, ID string) (Data[T], *types.CommonError) {
	if w.getByID != nil {
		return w.getByID(ctx, userID, ID)
	}
	return w.Repository.GetByID(ctx, userID, ID)
}

func (w *wrapper[T]) GetByMainRefID(ctx context.Context, userID, mainRefID string) ([]Data[T], *types.CommonError) {
	if w.getByMainRefID != nil {
		return w.getByMainRefID(ctx, userID, mainRefID)
	}
	return w.Repository.GetByMainRefID(ctx, userID, mainRefID)
}

// Should be not business logic (eg. global) to protect the DB itself
func LimitSize[T any](a Repository[T], configuredMax int) Repository[T] {
	return &wrapper[T]{
		Repository: a,
		put: func(ctx context.Context, userID, ID string, refIDs []string, data Data[T]) (Data[T], *types.CommonError) {
			var existing []Data[T]
			var err *types.CommonError

			// Limit based on wether they're dependent service or not.
			// If it's main entity, then limit based on user ID
			// If it's dependent entity, then limit based on the main entity
			if data.ParentID() != "" {
				existing, err = a.GetByMainRefID(ctx, userID, data.ParentID())
				if err != nil {
					return Data[T]{}, err
				}
			} else {
				existing, err = a.Get(ctx, userID, ID, refIDs)
				if err != nil {
					return Data[T]{}, err
				}
			}

			if len(existing) >= configuredMax && data.ID == "" {
				return Data[T]{}, &types.CommonError{
					Errors: []types.Error{
						{
							HTTPCode: http.StatusNotAcceptable,
							Code:     "MAX_LIMIT_REACHED",
							Message:  "Put failed. Maximum limit for the content reached",
						},
					},
				}
			}
			return a.Put(ctx, userID, ID, refIDs, data)
		},
	}
}

// Creation of entity in B dependend on entity already exist (have ID) in repository A
// Deletion of entity in A will be canceled if entity B still have reference to A
func Link[T any, U any](a Repository[T], b Repository[U]) (Repository[T], Repository[U]) {
	lock := &sync.Mutex{}

	return &wrapper[T]{
			Repository: a,
			delete: func(ctx context.Context, userID, ID string, refIDs []string) (Data[T], *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				_, err := b.GetByID(ctx, userID, ID)
				if err != nil {
					return a.Delete(ctx, userID, ID, refIDs)
				}

				return Data[T]{}, &types.CommonError{
					Errors: []types.Error{
						{
							Code:     "DATA_REFERENCE",
							HTTPCode: http.StatusNotAcceptable,
							Message:  "Delete failed. This content is being linked by another  Delete them first.",
						},
					},
				}
			},
		}, &wrapper[U]{
			Repository: b,
			put: func(ctx context.Context, userID, ID string, refIDs []string, data Data[U]) (Data[U], *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				if data.ID == "" {
					return Data[U]{}, &types.CommonError{
						Errors: []types.Error{
							{
								Code:     "BAD_REQUEST",
								HTTPCode: http.StatusBadRequest,
								Message:  "Put failed. Empty ID.",
							},
						},
					}
				}
				// check if exist in the main DB first.
				// if not we  fail
				_, err := a.GetByID(ctx, userID, data.ID)
				if err != nil {
					return Data[U]{}, &types.CommonError{
						Errors: []types.Error{
							{
								Code:     "DATA_REFERENCE",
								HTTPCode: http.StatusNotFound,
								Message:  "Put failed. The content linked to this ID must exist first",
							},
						},
					}
				}

				return b.Put(ctx, userID, ID, refIDs, data)
			},
		}
}

// Reference is Link Many
func Reference[T any, U any](a Repository[T], b Repository[U]) (Repository[T], Repository[U]) {
	lock := &sync.Mutex{}

	return &wrapper[T]{
			Repository: a,
			delete: func(ctx context.Context, userID, ID string, refIDs []string) (Data[T], *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				result, err := b.GetByMainRefID(ctx, userID, ID)
				if err != nil {
					return Data[T]{}, err
				}

				if len(result) > 0 {
					return Data[T]{}, &types.CommonError{
						Errors: []types.Error{
							{
								Code:     "DATA_REFERENCE",
								HTTPCode: http.StatusNotAcceptable,
								Message:  "Delete failed. This content is being linked by another  Delete them first.",
							},
						},
					}
				}

				return a.Delete(ctx, userID, ID, refIDs)
			},
		}, &wrapper[U]{
			Repository: b,
			put: func(ctx context.Context, userID, ID string, refIDs []string, data Data[U]) (Data[U], *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				if data.ParentID() == "" {
					return Data[U]{}, &types.CommonError{
						Errors: []types.Error{
							{
								Code:     "BAD_REQUEST",
								HTTPCode: http.StatusBadRequest,
								Message:  "Put failed. Please specify ID to main content",
							},
						},
					}
				}
				// check if exist in the main DB first.
				// if not we  fail
				_, err := a.GetByID(ctx, userID, data.ParentID())
				if err != nil {
					return Data[U]{}, &types.CommonError{
						Errors: []types.Error{
							{
								Code:     "DATA_REFERENCE",
								HTTPCode: http.StatusNotFound,
								Message:  "Put failed. The content linked to this ID must exist first",
							},
						},
					}
				}

				return b.Put(ctx, userID, ID, refIDs, data)
			},
		}
}
