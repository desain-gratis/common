package plugin

import (
	"context"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Usecase[mycontent.Data] = &mcAuth[mycontent.Data]{}

type mcAuth[T mycontent.Data] struct {
	mycontent.Usecase[T]
}

func MyContentWithAuth[T mycontent.Data](base mycontent.Usecase[T]) *mcAuth[T] {
	return &mcAuth[T]{
		Usecase: base,
	}
}

func (a *mcAuth[T]) Post(ctx context.Context, data T, meta any) (T, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return data, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, data.Namespace())
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Usecase.Post(ctx, data, meta)
}

// Get all of your resource for your user ID here
func (a *mcAuth[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, namespace)
	if err != nil {
		return nil, err
	}

	return a.Usecase.Get(ctx, namespace, refIDs, ID)

	// you can also filter result based on each get result afterward based on "permission"
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (a *mcAuth[T]) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (T, *types.CommonError) {
	var data T

	auth := getAuth(ctx)
	if auth == nil {
		return data, &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusInternalServerError,
					Code:     "EMPTY_AUTHORIZATION",
					Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
				},
			},
		}
	}

	// verify namespace
	err := verifyNamespace(auth, namespace)
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Usecase.Delete(ctx, namespace, refIDs, ID)
}
