package sku

import (
	"context"
)

type Repository[T any] interface {
	Get(ctx context.Context, organizationID, id string, refID []string) (result []T, err error)
	Insert(ctx context.Context, organizationID, id string, refID []string, data T) (err error)
	Update(ctx context.Context, organizationID, id string, refID []string, data T) (err error)
	Delete(ctx context.Context, organizationID, id string, refID []string) (err error)
}
