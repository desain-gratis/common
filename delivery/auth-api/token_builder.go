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
	"github.com/desain-gratis/common/types/protobuf/session"
)

type SignInResponse struct {
	// Profile from OIDC provider
	LoginProfile *Profile `json:"login_profile,omitempty"`

	Locale  []string `json:"locale,omitempty"`
	IDToken *string  `json:"id_token,omitempty"`
	// Collection of grants NOT signed, for debugging.
	// DO NOT USE THIS FOR BACK END VALIDATION!!!
	Grants map[string]*session.Grant `json:"grants"`
	Expiry string                    `json:"expiry,omitempty"`
	Data   any                       `json:"data,omitempty"`
}

type Profile struct {
	URL              string `json:"url"`
	DisplayName      string `json:"display_name"`
	ImageDataURL     string `json:"image_data_url"`
	ImageURL         string `json:"image_url"`
	Avatar1x1URL     string `json:"avatar_1x1_url"`
	Background3x1URL string `json:"background_3x1_url"`
	Email            string `json:"email"`
}

// DO NOT USE THIS AS BACK-END VALIDATION!!!!!!!!!!!!!!!!!!!!!!!!!!!
type Grant struct {
	// DO NOT USE THIS AS BACK-END VALIDATION!!!!!!!!!!!!!!!!!!!!!!!!!!!
	UserId string `json:"user_id,omitempty"`
	// DO NOT USE THIS AS BACK-END VALIDATION!!!!!!!!!!!!!!!!!!!!!!!!!!!
	GroupId string `json:"group_id,omitempty"`
	// DO NOT USE THIS AS BACK-END VALIDATION!!!!!!!!!!!!!!!!!!!!!!!!!!!
	Name string `json:"name,omitempty"`
	// DO NOT USE THIS AS BACK-END VALIDATION!!!!!!!!!!!!!!!!!!!!!!!!!!!
	UiAndApiPermission map[string]bool `json:"ui_and_api_permission,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"varint,2,opt,name=value,proto3"`
}

type Data struct {
	URL    string   `json:"url"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
	UserID string   `json:"user_id"`
}

type Organization struct {
	URL    string `json:"url"`
	ApiURL string `json:"api_url"`

	// SignInPK should be the path to the actual key in GSM
	SignInPK    string `json:"sign_in_pk"`
	SignInKeyID string `json:"sign_in_key_id"`
	Auth        Auth   `json:"auth"`
}

type Auth string

const (
	AUTH_GSI Auth = "gsi"

	AuthParserGoogle AuthParser = "gsi"
)

type AuthParser string
type AuthLogic func(req *http.Request, authMethod string, payload *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError)

type TokenBuilder interface {
	BuildToken(req *http.Request, authMethod string, payload *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError)
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
					{Message: "Failed to get user auth: " + errUC.Err().Error(), Code: "SERVER_ERROR"},
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
		newToken, errUC := tokenSigner.Sign(r.Context(), payload, expiry)
		if errUC != nil {
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
