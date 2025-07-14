package plugin

import (
	"net/http"

	types "github.com/desain-gratis/common/types/http"
	"github.com/julienschmidt/httprouter"
)

func AdminOnly(handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		auth := getAuth(r.Context())
		if auth == nil {
			errUC := &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "EMPTY_AUTHORIZATION",
						Message:  "authorization is configured by the server, but it's empty. Contact server owner.",
					},
				},
			}
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}

		if !auth.IsSuperAdmin {
			errUC := &types.CommonError{
				Errors: []types.Error{
					{
						HTTPCode: http.StatusInternalServerError,
						Code:     "NOT_ADMIN",
						Message:  "Sorry, you're not an atmin :\"",
					},
				},
			}
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}
	}
}
