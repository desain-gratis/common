package main

import (
	"context"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	"github.com/desain-gratis/common/lib/notifier"
	types "github.com/desain-gratis/common/types/http"
)

var _ mycontent.Usecase[mycontent.Data] = &withNotifier[mycontent.Data]{}

type withNotifier[T mycontent.Data] struct {
	*mycontent_base.Handler[T]
	notifier notifier.Topic
}

func (w *withNotifier[T]) Post(ctx context.Context, data T, meta any) (T, *types.CommonError) {
	v, err := w.Handler.Post(ctx, data, meta)
	if err != nil {
		return v, err
	}

	// publish locally (only work for 1 server)
	w.notifier.Broadcast(ctx, v)
	return v, nil
}
