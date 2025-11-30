package authapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
)

type Auth string

const (
	AUTH_GSI Auth = "gsi"

	AuthParserGoogle AuthParser = "gsi"
)

type AuthParser string
type AuthLogic func(req *http.Request, authMethod string, payload *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err error)

type TokenBuilder interface {
	BuildToken(req *http.Request, authMethod string, payload *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err error)
}

func GetToken(tokenBuilder TokenBuilder, tokenSigner TokenSigner) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		// 1. Obtain ID Token
		auth, ok := r.Context().Value(IDTokenKey{}).(*idtoken.Payload)
		if auth == nil || !ok {
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

		name := "UNKNOWN"
		if metaName, ok := r.Context().Value(IDTokenNameKey{}).(string); name != "" && ok {
			name = metaName
		}

		// 2. Build proto token
		data, apiData, expiry, errUC := tokenBuilder.BuildToken(r, name, auth)
		if errUC != nil {
			errMessage := types.SerializeError(&types.CommonError{
				Errors: []types.Error{
					{Message: "Failed to get user auth: " + errUC.Error(), Code: "SERVER_ERROR"},
				}})
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(errMessage)
			return
		}

		// 3. Serialize proto token to bytes
		payload, errProto := proto.Marshal(data)
		if errProto != nil {
			log.Err(errProto).Msgf("Proto token build error")
			errMessage := types.SerializeError(&types.CommonError{
				Errors: []types.Error{
					{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"},
				},
			})
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(errMessage)
			return
		}

		// 4. Sign the proto token
		newToken, err := tokenSigner.Sign(r.Context(), payload, expiry)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(errMessage)
			return
		}

		// 5. Publish in our API
		resp, errJson := json.Marshal(&types.CommonResponse{
			Success: SignInResponse{
				IDToken: &newToken,
				Data:    apiData, // DO NOT USE THIS FOR BACK END VALIDATION
			},
		})
		if errJson != nil {
			if r.Context().Err() != nil {
				return
			}

			log.Err(errJson).Msgf("Failed to marshal payload as JSON")
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
		w.Write(resp)
	}
}
