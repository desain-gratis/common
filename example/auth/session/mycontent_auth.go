package session

import (
	"context"
	"io"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/mycontent"
)

var _ mycontent.Usecase[mycontent.Data] = &mycontentWithAuth[mycontent.Data]{}

type mycontentWithAuth[T mycontent.Data] struct {
	// we "extend" (compose) the capability of both mycontent.Usecase & mycontent.Attachable with authorization logic
	// inside ctx

	mycontent.Usecase[T]
	mycontent.Attachable[T]
	ctx context.Context
}

func MyContentWithAuth[T mycontent.Data](ctx context.Context, base mycontent.Usecase[T]) *mycontentWithAuth[T] {
	return &mycontentWithAuth[T]{
		Usecase: base,
		ctx:     ctx,
	}
}

func (a *mycontentWithAuth[T]) Post(ctx context.Context, data T, meta any) (T, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil || auth.Grants == nil {
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
	err := a.verifyNamespace(auth, data.Namespace())
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Usecase.Post(ctx, data, meta)
}

// Get all of your resource for your user ID here
func (a *mycontentWithAuth[T]) Get(ctx context.Context, namespace string, refIDs []string, ID string) ([]T, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil || auth.Grants == nil {
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
	err := a.verifyNamespace(auth, namespace)
	if err != nil {
		return nil, err
	}

	return a.Usecase.Get(ctx, namespace, refIDs, ID)

	// you can also filter result based on each get result afterward based on "permission"
}

// Delete your resource here
// the implementation can check whether there are linked resource or not
func (a *mycontentWithAuth[T]) Delete(ctx context.Context, namespace string, refIDs []string, ID string) (T, *types.CommonError) {
	var data T

	auth := getAuth(ctx)
	if auth == nil || auth.Grants == nil {
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
	err := a.verifyNamespace(auth, namespace)
	if err != nil {
		return data, err
	}

	// you can get existing first to check the "permission"

	return a.Usecase.Delete(ctx, namespace, refIDs, ID)
}

func (a *mycontentWithAuth[T]) Attach(ctx context.Context, meta T, payload io.Reader) (T, *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil || auth.Grants == nil {
		return meta, &types.CommonError{
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
	err := a.verifyNamespace(auth, meta.Namespace())
	if err != nil {
		return meta, err
	}

	return a.Attachable.Attach(ctx, meta, payload)
}

func (a *mycontentWithAuth[T]) GetAttachment(ctx context.Context, userID string, refIDs []string, ID string) (payload io.ReadCloser, meta T, err *types.CommonError) {
	auth := getAuth(ctx)
	if auth == nil || auth.Grants == nil {
		return nil, meta, &types.CommonError{
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
	err = a.verifyNamespace(auth, meta.Namespace())
	if err != nil {
		return nil, meta, err
	}

	return a.Attachable.GetAttachment(ctx, userID, refIDs, ID)
}

func (a *mycontentWithAuth[T]) verifyNamespace(auth *session.SessionData, namespace string) *types.CommonError {
	// is this a valid namespace ?
	if namespace == "" {
		return &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "EMPTY_NAMESPACE",
					Message:  "Every entity in mycontent have a namespace, please specify one",
				},
			},
		}
	}

	// is this user authorized to post on behalf of data's namespace ..?
	if _, ok := auth.Grants[namespace]; !ok && !auth.IsSuperAdmin {
		return &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED_NAMESPACE",
					Message:  "You're not authorized to post on behalf of this namespace",
				},
			},
		}
	}
	return nil
}
