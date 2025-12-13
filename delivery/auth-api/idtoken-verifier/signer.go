package idtokenverifier

import (
	"context"
	"net/http"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
	"github.com/julienschmidt/httprouter"
)

func AppAuth(verifier authapi.TokenVerifier, authKey any, parser authapi.TokenParser) func(httprouter.Handle) httprouter.Handle {
	return func(handler httprouter.Handle) httprouter.Handle {
		return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			token, errUC := getToken(r.Header.Get("Authorization"))
			if errUC != nil {
				errMessage := types.SerializeError(errUC)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			result, err := verifier.Verify(r.Context(), token)
			if err != nil {
				errMessage := types.SerializeError(&types.CommonError{
					Errors: []types.Error{
						{Code: "UNAUTHORIZED", HTTPCode: http.StatusBadRequest, Message: "Unauthorized. Please specify correct token. " + err.Error()},
					},
				})
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			authPayload, err := parser(r.Context(), result)
			if err != nil {
				errMessage := types.SerializeError(&types.CommonError{
					Errors: []types.Error{
						{Code: "CLIENT_ERROR", HTTPCode: http.StatusBadRequest, Message: "Failed to parse auth payload."},
					},
				})
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			ctx := context.WithValue(r.Context(), authKey, authPayload)
			r = r.WithContext(ctx)
			handler(w, r, p)
		}
	}
}
