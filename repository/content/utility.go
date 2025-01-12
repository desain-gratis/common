package content

import (
	"context"
	"net/http"
	"sync"

	types "github.com/desain-gratis/common/types/http"
)

var _ Repository = &wrapper{}

// Wrap repo with another repository
type wrapper struct {
	Repository

	post           func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError)
	put            func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError)
	get            func(ctx context.Context, userID, ID string, refIDs []string) ([]Data, *types.CommonError)
	delete         func(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError)
	getByID        func(ctx context.Context, userID, ID string) (Data, *types.CommonError)
	getByMainRefID func(ctx context.Context, userID, mainRefID string) ([]Data, *types.CommonError)
}

func (w *wrapper) Post(ctx context.Context, userID, ID string, refIDs []string, data Data) (out Data, err *types.CommonError) {
	if w.post != nil {
		return w.post(ctx, userID, ID, refIDs, data)
	}
	return w.Repository.Post(ctx, userID, ID, refIDs, data)
}

func (w *wrapper) Put(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError) {
	if w.put != nil {
		return w.put(ctx, userID, ID, refIDs, data)
	}
	return w.Repository.Put(ctx, userID, ID, refIDs, data)
}

func (w *wrapper) Get(ctx context.Context, userID, ID string, refIDs []string) ([]Data, *types.CommonError) {
	if w.get != nil {
		return w.get(ctx, userID, ID, refIDs)
	}
	return w.Repository.Get(ctx, userID, ID, refIDs)
}

func (w *wrapper) Delete(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError) {
	if w.delete != nil {
		return w.delete(ctx, userID, ID, refIDs)
	}
	return w.Repository.Delete(ctx, userID, ID, refIDs)
}

func (w *wrapper) GetByID(ctx context.Context, userID, ID string) (Data, *types.CommonError) {
	if w.getByID != nil {
		return w.getByID(ctx, userID, ID)
	}
	return w.Repository.GetByID(ctx, userID, ID)
}

func (w *wrapper) GetByMainRefID(ctx context.Context, userID, mainRefID string) ([]Data, *types.CommonError) {
	if w.getByMainRefID != nil {
		return w.getByMainRefID(ctx, userID, mainRefID)
	}
	return w.Repository.GetByMainRefID(ctx, userID, mainRefID)
}

// Should be not business logic (eg. global) to protect the DB itself
func LimitSize[T any](a Repository, configuredMax int) Repository {
	return &wrapper{
		Repository: a,
		put: func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError) {
			var existing []Data
			var err *types.CommonError

			// Limit based on wether they're dependent service or not.
			// If it's main entity, then limit based on user ID
			// If it's dependent entity, then limit based on the main entity
			parentID := ""
			if len(data.RefIDs) > 0 {
				parentID = data.RefIDs[len(data.RefIDs)-1]
			}
			if parentID != "" {
				existing, err = a.GetByMainRefID(ctx, userID, data.RefIDs[len(data.RefIDs)-1])
				if err != nil {
					return Data{}, err
				}
			} else {
				existing, err = a.Get(ctx, userID, ID, refIDs)
				if err != nil {
					return Data{}, err
				}
			}

			if len(existing) >= configuredMax && data.ID == "" {
				return Data{}, &types.CommonError{
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
func Link(a Repository, b Repository) (Repository, Repository) {
	lock := &sync.Mutex{}

	return &wrapper{
			Repository: a,
			delete: func(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				_, err := b.GetByID(ctx, userID, ID)
				if err != nil {
					return a.Delete(ctx, userID, ID, refIDs)
				}

				return Data{}, &types.CommonError{
					Errors: []types.Error{
						{
							Code:     "DATA_REFERENCE",
							HTTPCode: http.StatusNotAcceptable,
							Message:  "Delete failed. This content is being linked by another  Delete them first.",
						},
					},
				}
			},
		}, &wrapper{
			Repository: b,
			put: func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				if data.ID == "" {
					return Data{}, &types.CommonError{
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
					return Data{}, &types.CommonError{
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
func Reference(a Repository, b Repository) (Repository, Repository) {
	lock := &sync.Mutex{}

	return &wrapper{
			Repository: a,
			delete: func(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				result, err := b.GetByMainRefID(ctx, userID, ID)
				if err != nil {
					return Data{}, err
				}

				if len(result) > 0 {
					return Data{}, &types.CommonError{
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
		}, &wrapper{
			Repository: b,
			put: func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError) {
				lock.Lock()
				defer lock.Unlock()

				parentID := ""
				if len(data.RefIDs) > 0 {
					parentID = data.RefIDs[len(data.RefIDs)-1]
				}

				if parentID == "" {
					return Data{}, &types.CommonError{
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
				_, err := a.GetByID(ctx, userID, parentID)
				if err != nil {
					return Data{}, &types.CommonError{
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
