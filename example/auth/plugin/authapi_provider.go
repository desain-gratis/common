package plugin

import (
	"context"
	"encoding/json"
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
)

const (
	authCtxKey key = "auth-ctx-key"
)

type key string

type authProvider struct {
	verifier signing.Verifier
	signer   signing.Signer
}

// Verify application token according to application auth logic,
// and inject auth data to httprouter.Handler
func AuthProvider(verifier signing.Verifier, signer signing.Signer) *authProvider {
	return &authProvider{
		verifier,
		signer,
	}
}

func (v *authProvider) User(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authData, errUC := parseAuthorizationToken(r.Context(), v.verifier, r.Header.Get("Authorization"))
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

func (v *authProvider) AdminOnly(handle httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		authData, errUC := parseAuthorizationToken(r.Context(), v.verifier, r.Header.Get("Authorization"))
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
						Message:  "Unauthorized.",
					},
				},
			})
			log.Warn().Msgf("Someone (%v) tried to sign in as admin but failed.", authData.SignInEmail)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(errMessage)
			return
		}

		handle(w, reqWithAuth, p)
	}
}

func (v *authProvider) Debug(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	authData, errUC := parseAuthorizationToken(r.Context(), v.verifier, r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(errMessage)
		return
	}

	payload, err := json.Marshal(authData)
	if err != nil {
		log.Err(err).Msgf("Error json debug")
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "failed to marshal json", Code: "INTERNAL_SERVER_ERROR"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

// Keys allows other service to verify this published delivery Open ID credential
func (v *authProvider) Keys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	keys, errUC := v.signer.Keys(r.Context())
	if errUC != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(errUC.Err()).Msgf("Failed to get keys")
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, err := json.Marshal(&types.CommonResponse{
		Success: keys,
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(err).Msgf("Failed to parse payload")
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse response", Code: "SERVER_ERROR"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(payload)
}

func getAuth(ctx context.Context) *session.SessionData {
	auth, _ := ctx.Value(authCtxKey).(*session.SessionData)
	return auth
}
