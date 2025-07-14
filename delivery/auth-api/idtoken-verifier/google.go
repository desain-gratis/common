package idtokenverifier

import (
	"context"
	"net/http"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
	"github.com/julienschmidt/httprouter"
	"google.golang.org/api/idtoken"
)

func GSIAuth(clientID string) func(httprouter.Handle) httprouter.Handle {
	return func(handler httprouter.Handle) httprouter.Handle {
		return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			token, errUC := getToken(r.Header.Get("Authorization"))
			if errUC != nil {
				errMessage := types.SerializeError(errUC)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			result, err := idtoken.Validate(r.Context(), token, clientID)
			if err != nil {
				errMessage := types.SerializeError(&types.CommonError{
					Errors: []types.Error{
						{Code: "UNAUTHORIZED", HTTPCode: http.StatusBadRequest, Message: "Unauthorized. Please specify correct token."},
					},
				})
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			ctx := context.WithValue(r.Context(), authapi.IDTokenKey{}, result)
			ctx = context.WithValue(ctx, authapi.IDTokenNameKey{}, "GSI")
			r = r.WithContext(ctx)
			handler(w, r, p)
		}
	}
}
