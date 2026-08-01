package sqliteraft

import (
	"context"

	"github.com/desain-gratis/common/delivery/mycontent-api/storage/content"
)

type repository struct {
	app *ContentApp
}

var _ content.Repository = (*repository)(nil)

func (r *repository) Post(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
	data content.Data,
) (content.Data, error) {
	return r.post(ctx, namespace, refIDs, ID, data)
}

func (r *repository) Get(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) ([]content.Data, error) {
	return r.get(ctx, namespace, refIDs, ID)
}

func (r *repository) Delete(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (content.Data, error) {
	return r.delete(ctx, namespace, refIDs, ID)
}

func (r *repository) Stream(
	ctx context.Context,
	namespace string,
	refIDs []string,
	ID string,
) (<-chan content.Data, error) {
	return r.stream(ctx, namespace, refIDs, ID)
}
