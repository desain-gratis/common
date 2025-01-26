package plugin

import (
	"context"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
	"github.com/julienschmidt/httprouter"
)

const (
	authCtxKey key = "auth-ctx-key"
)

type key string

type verifier struct {
	uc signing.Verifier
}

func AuthProvider(uc signing.Verifier) *verifier {
	return &verifier{uc}
}

func (v *verifier) User(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authData, errUC := parseAuthorizationToken(r.Context(), v.uc, r.Header.Get("Authorization"))
		if errUC != nil {
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(errMessage)
			return
		}

		// inject authData to request-scoped context
		reqWithAuth := r.WithContext(context.WithValue(r.Context(), authCtxKey, authData))

		handle(w, reqWithAuth, p)
	}
}

func (v *verifier) AdminOnly(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authData, errUC := parseAuthorizationToken(r.Context(), v.uc, r.Header.Get("Authorization"))
		if errUC != nil {
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(errMessage)
			return
		}

		// inject authData to request-scoped context
		reqWithAuth := r.WithContext(context.WithValue(r.Context(), authCtxKey, authData))

		if !authData.IsSuperAdmin {
			errMessage := types.SerializeError(&types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusUnauthorized,
						Code:     "UNAUTHORIZED",
						Message:  "Your role is unauthorized for this API.",
					},
				},
			})
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(errMessage)
			return
		}

		handle(w, reqWithAuth, p)
	}
}

func getAuth(ctx context.Context) *session.SessionData {
	auth, _ := ctx.Value(authCtxKey).(*session.SessionData)
	return auth
}
