package authapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/proto"

	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
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

	keyGoogleAuth key = "google-auth"

	AuthParserGoogle AuthParser = "gsi"
)

type key string
type AuthParser string
type TokenBuilder func(req *http.Request, authMethod string, payload *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError)

type IdTokenExchanger struct {
	verifierName string
	verifier     signing.VerifierOf[*idtoken.Payload]
	signer       signing.Signer
}

func NewIdTokenExchanger(
	verifierName string,
	verifier signing.VerifierOf[*idtoken.Payload],
	signer signing.Signer,
) *IdTokenExchanger {
	return &IdTokenExchanger{verifierName, verifier, signer}
}

// Convenient handler for exchanging token
func (g *IdTokenExchanger) ExchangeToken(
	tokenBuilder TokenBuilder,
) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		token, err := getToken(r.Header.Get("Authorization"))
		if err != nil {
			errMessage := types.SerializeError(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}

		auth, err := g.verifier.VerifyAs(r.Context(), token)
		if err != nil {
			errMessage := types.SerializeError(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}

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

		// 2. Build proto token
		data, apiData, expiry, errUC := tokenBuilder(r, g.verifierName, auth)
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
		newToken, errUC := g.signer.Sign(r.Context(), payload, expiry)
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

// WithAuthorization is for more generic authorization
func (g *IdTokenExchanger) WithAuthorization(handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		token, err := getToken(r.Header.Get("Authorization"))
		if err != nil {
			errMessage := types.SerializeError(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}

		payload, err := g.verifier.VerifyAs(r.Context(), token)
		if err != nil {
			errMessage := types.SerializeError(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write(errMessage)
			return
		}

		handler(w, r.WithContext(context.WithValue(r.Context(), keyGoogleAuth, payload)), p)
	}
}

func GetOIDCClaims(claims map[string]interface{}) *session.OIDCClaim {
	var claim session.OIDCClaim
	if v, ok := claims["iss"]; ok {
		claim.Iss, _ = v.(string)
	}
	if v, ok := claims["sub"]; ok {
		claim.Sub, _ = v.(string)
	}
	if v, ok := claims["email"]; ok {
		claim.Email, _ = v.(string)
	}
	if v, ok := claims["email_verified"]; ok {
		b, _ := v.(string)
		claim.EmailVerified, _ = strconv.ParseBool(b)
	}
	if v, ok := claims["name"]; ok {
		claim.Name, _ = v.(string)
	}
	if v, ok := claims["family_name"]; ok {
		claim.FamilyName, _ = v.(string)
	}
	if v, ok := claims["given_name"]; ok {
		claim.GivenName, _ = v.(string)
	}
	if v, ok := claims["nickname"]; ok {
		claim.Nickname, _ = v.(string)
	}
	if v, ok := claims["given_name"]; ok {
		claim.GivenName, _ = v.(string)
	}
	return &claim
}

func (s *signingService) MultiKeys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		cer := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "EMPTY_ORGANIZATION",
					Message:  "Please spcecify `org` parameter",
				},
			},
		}
		errMessage := types.SerializeError(cer)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	s.Keys(w, r, p)
}

func getToken(authorizationToken string) (string, *types.CommonError) {
	token := strings.Split(authorizationToken, " ")
	if len(token) < 2 {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusBadRequest,
					Code:     "INVALID_OR_EMPTY_AUTHORIZATION",
					Message:  "Authorization header is not valid",
				},
			},
		}
	}
	return token[1], nil
}
