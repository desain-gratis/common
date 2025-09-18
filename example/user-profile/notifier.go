package main

import (
	"context"

	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	mycontent_base "github.com/desain-gratis/common/delivery/mycontent-api/mycontent/base"
	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
	types "github.com/desain-gratis/common/types/http"
)

var _ mycontent.Usecase[mycontent.Data] = &withNotifier[mycontent.Data]{}

type withNotifier[T mycontent.Data] struct {
	*mycontent_base.Handler[T]
	notifier notifierapi.Broker
}

func (w *withNotifier[T]) Post(ctx context.Context, data T, meta any) (T, *types.CommonError) {
	v, err := w.Handler.Post(ctx, data, meta)
	if err != nil {
		return v, err
	}

	w.notifier.Broadcast(ctx, v)
	return v, nil
}
