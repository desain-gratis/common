package base

import (
	"context"
	"fmt"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
)

var _ mycontent.Usecase[mycontent.Data] = &Handler[mycontent.Data]{}

type ViewOnlyHandler[T mycontent.Data] struct {
	*Handler[T]
}

func (c *ViewOnlyHandler[T]) Post(ctx context.Context, data T, meta any) (T, error) {
	var t T
	return t, fmt.Errorf("viewonlyhandler, unsupported operation: post")
}

func (c *ViewOnlyHandler[T]) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (T, error) {
	var t T
	return t, fmt.Errorf("viewonlyhandler, unsupported operation: delete")
}
