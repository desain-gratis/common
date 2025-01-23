package mycontentapi

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

type AuthorizationFactory[T mycontent.Data] func(ctx context.Context, token string) (Authorization[T], *types.CommonError)

// Authorization for mycontent
type Authorization[T mycontent.Data] interface {
	// Custom post authorization
	CanPost(t T) *types.CommonError

	// Can read this entity?
	CanGet(t T) *types.CommonError

	// Can delete this entity?
	CanDelete(t T) *types.CommonError

	// Can delete parameter
	CheckBeforeDelete(namespace string, refIDs []string, ID string) *types.CommonError

	// Check get parameter
	CheckBeforeGet(namespace string, refIDs []string, ID string) *types.CommonError
}
