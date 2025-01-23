package mycontentapi

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ Authorization[mycontent.Data] = &emptyAuthorization[mycontent.Data]{}

type emptyAuthorization[T mycontent.Data] struct{}

func EmptyAuthorizationFactory[T mycontent.Data](ctx context.Context, token string) (Authorization[T], *types.CommonError) {
	return &emptyAuthorization[T]{}, nil
}

// Custom post authorization
func (e *emptyAuthorization[T]) CanPost(t T) *types.CommonError {
	return nil
}

// Can read this entity?
func (e *emptyAuthorization[T]) CanGet(t T) *types.CommonError {
	return nil
}

// Can delete this entity?
func (e *emptyAuthorization[T]) CanDelete(t T) *types.CommonError {
	return nil
}

// Can delete parameter
func (e *emptyAuthorization[T]) CheckBeforeDelete(namespace string, refIDs []string, ID string) *types.CommonError {
	return nil
}

// Check get parameter
func (e *emptyAuthorization[T]) CheckBeforeGet(namespace string, refIDs []string, ID string) *types.CommonError {
	return nil
}
