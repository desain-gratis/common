package content

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

var _ Repository = &wrapper{}

// Wrap repo with another repository
type wrapper struct {
	Repository

	post   func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError)
	put    func(ctx context.Context, userID, ID string, refIDs []string, data Data) (Data, *types.CommonError)
	get    func(ctx context.Context, userID, ID string, refIDs []string) ([]Data, *types.CommonError)
	delete func(ctx context.Context, userID, ID string, refIDs []string) (Data, *types.CommonError)
}

func (w *wrapper) Post(ctx context.Context, userID, ID string, refIDs []string, data Data) (out Data, err *types.CommonError) {
	if w.post != nil {
		return w.post(ctx, userID, ID, refIDs, data)
	}
	return w.Repository.Post(ctx, userID, ID, refIDs, data)
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
