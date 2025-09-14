package notifierapi

import (
	"context"
)

type Notifier interface {
	Publish(ctx context.Context, message any) error
	Listen(ctx context.Context) <-chan any
}
