package plugin

import (
	"context"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
)

const (
	AuthCtxKey key = "auth-ctx-key"
)

type key string

func getAuth(ctx context.Context) *session.SessionData {
	auth, _ := ctx.Value(AuthCtxKey).(*session.SessionData)
	return auth
}

func verifyNamespace(auth *session.SessionData, namespace string) *types.CommonError {
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

	// if super admin, then bypass namespace authorization
	if auth.IsSuperAdmin {
		return nil
	}

	// is this user authorized to post on behalf of data's namespace ..?
	if _, ok := auth.Grants[namespace]; !ok {
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
