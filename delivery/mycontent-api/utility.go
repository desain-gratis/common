package mycontentapi

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

var _ Repository = &wrapper{}

// Wrap repo with another repository
type wrapper struct {
	Repository

	post   func(ctx context.Context, userID string, refIDs []string, ID string, data Data) (Data, *types.CommonError)
	get    func(ctx context.Context, userID string, refIDs []string, ID string) ([]Data, *types.CommonError)
	delete func(ctx context.Context, userID string, refIDs []string, ID string) (Data, *types.CommonError)
}

func (w *wrapper) Post(ctx context.Context, userID string, refIDs []string, ID string, data Data) (out Data, err *types.CommonError) {
	if w.post != nil {
		return w.post(ctx, userID, refIDs, ID, data)
	}
	return w.Repository.Post(ctx, userID, refIDs, ID, data)
}

func (w *wrapper) Get(ctx context.Context, userID string, refIDs []string, ID string) ([]Data, *types.CommonError) {
	if w.get != nil {
		return w.get(ctx, userID, refIDs, ID)
	}
	return w.Repository.Get(ctx, userID, refIDs, ID)
}

func (w *wrapper) Delete(ctx context.Context, userID string, refIDs []string, ID string) (Data, *types.CommonError) {
	if w.delete != nil {
		return w.delete(ctx, userID, refIDs, ID)
	}
	return w.Repository.Delete(ctx, userID, refIDs, ID)
}
