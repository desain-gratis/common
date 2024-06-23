package secretkv

import (
	"context"
	"time"
)

var Default Provider

type Provider interface {
	GetF(key string, version int) func() (Payload, error)
	Get(ctx context.Context, key string, version int) (Payload, error)
	List(ctx context.Context, key string) ([]Payload, error)
}

type Payload struct {
	Key       string
	Version   int
	CreatedAt time.Time
	Payload   []byte
	Meta      map[string]any
}

func Get(ctx context.Context, key string, version int) (Payload, error) {
	return Default.Get(ctx, key, version)
}

func List(ctx context.Context, key string) ([]Payload, error) {
	return Default.List(ctx, key)
}
