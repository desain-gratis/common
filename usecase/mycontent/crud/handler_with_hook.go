package crud

import (
	"context"

	"github.com/desain-gratis/common/repository/content"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Usecase[any] = &crudWithHook[any]{}

type crudWithHook[T any] struct {
	*crud[T]
	updateHook mycontent.UpdateHook[T]
}

func NewWithHook[T any](
	repo content.Repository[T],
	wrap func(T) mycontent.Data,
	validate func(T) *types.CommonError,
	updateHook mycontent.UpdateHook[T],
	urlFormat URLFormat,
) *crudWithHook[T] {
	return &crudWithHook[T]{
		crud: &crud[T]{
			repo:      repo,
			wrap:      wrap,
			validate:  validate,
			urlFormat: urlFormat,
		},
		updateHook: updateHook,
	}
}

func (c *crudWithHook[T]) Put(ctx context.Context, data T) (T, *types.CommonError) {
	var t T
	err := c.validate(data)
	if err != nil {
		return t, err
	}

	wrap := c.wrap(data)

	// get previous data first
	previous, err := c.repo.GetByID(ctx, wrap.OwnerID(), wrap.ID())
	if err != nil {
		return t, err
	}

	current, err := c.crud.Put(ctx, data)
	if err != nil {
		return current, err
	}

	err = c.updateHook.OnUpdate(previous.Data, current)
	if err != nil {
		return current, err
	}

	return current, nil
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (c *crudWithHook[T]) Delete(ctx context.Context, userID string, ID string) (T, *types.CommonError) {
	res, err := c.crud.Delete(ctx, userID, ID)
	if err != nil {
		return res, err
	}
	err = c.updateHook.OnDelete(res)
	if err != nil {
		return res, err
	}
	return res, nil
}
