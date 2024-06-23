package inmemory

import (
	"context"
	"sync"

	types "github.com/desain-gratis/common/types/http"
)

type inmemory[T any] struct {
	data map[string]T
	lock *sync.Mutex
}

func New[T any]() *inmemory[T] {
	return &inmemory[T]{
		data: make(map[string]T),
		lock: &sync.Mutex{},
	}
}

func (i *inmemory[T]) Set(ctx context.Context, userID string, data T) *types.CommonError {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.data[userID] = data
	return nil
}

func (i *inmemory[T]) Get(ctx context.Context, userID string) (T, *types.CommonError) {
	t := i.data[userID]
	return t, nil
}

func (i *inmemory[T]) Delete(ctx context.Context, userID string) *types.CommonError {
	i.lock.Lock()
	defer i.lock.Unlock()
	delete(i.data, userID)
	return nil
}
