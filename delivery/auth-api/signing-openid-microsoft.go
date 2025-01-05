package authapi

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/julienschmidt/httprouter"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/proto"

	"github.com/desain-gratis/common/delivery/helper"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/signing"
	"github.com/desain-gratis/common/usecase/user"
)

const (
	AUTH_MIP Auth = "mip"
)

type MicrosoftClaims struct {
	AIO       string `json:"aio,omitempty"`
	Email     string `json:"email,omitempty"`
	LoginHint string `json:"login_hint,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
	OID       string `json:"oid,omitempty"`
	Username  string `json:"preferred_username,omitempty"`
	SID       string `json:"sid,omitempty"`
	TenantID  string `json:"tid,omitempty"`
	Version   string `json:"ver,omitempty"`
	XMS       string `json:"xms_pl,omitempty"`
	jwt.StandardClaims
}

type microsoftSignInService struct {
	*signingService
	userUsecase user.UseCase
}

func NewMicrosoftSignInService(
	signing signing.Usecase,
	user user.UseCase,
) *microsoftSignInService {
	return &microsoftSignInService{
		signingService: &signingService{
			signing: signing,
		},
		userUsecase: user,
	}
}

func (s *microsoftSignInService) UpdateAuth(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	sessionData, errUC := ParseAuthorizationToken(r.Context(), s.signing, r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	if !sessionData.IsSuperAdmin {
		errUC := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED",
					Message:  "Cannot update user authorization. You're not super-admin!🚨🚨🚨",
				},
			},
		}
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// Read body parse entity and extract metadata

	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestLength)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to read all body", Code: "SERVER_ERROR"},
			},
		},
		)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	// var resource GSIData
	// err = json.Unmarshal(payload, &resource)
	// if err != nil {
	// 	errMessage := types.SerializeError(&types.CommonError{
	// 		Errors: []types.Error{
	// 			{Message: "Failed to parse body (content API). Make sure file size does not exceed 200 Kb: " + err.Error(), Code: "BAD_REQUEST"},
	// 		},
	// 	},
	// 	)
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	w.Write(errMessage)
	// 	return
	// }

	// resource.OwnerId = "root" // hardcoded
	// // resource.Url = s // should fill

	// result, errUC := s.myContentAuth.Put(r.Context(), &resource)
	// if errUC != nil {
	// 	d := types.SerializeError(errUC)
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	w.Write(d)
	// 	return
	// }

	var clientPayload Payload
	err = json.Unmarshal(payload, &clientPayload)
	if err != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to parse body (content API). Make sure file size does not exceed 200 Kb: " + err.Error(), Code: "BAD_REQUEST"},
			}})
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	ucPayload := convertUpdatePayload(clientPayload)

	erruser := s.userUsecase.Insert(r.Context(), ucPayload)
	if erruser != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to insert user: " + erruser.Error(), Code: "SERVER_ERROR"},
			}})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	payload, err = json.Marshal(&types.CommonResponse{
		Success: "Success updated user authorization",
	})
	if err != nil {
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

func (s *microsoftSignInService) SignIn(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	token, err := verifyToken(r)
	if err != nil {
		helper.SetError(w, types.Error{HTTPCode: http.StatusBadRequest, Message: err.Error(), Code: "CREDENTIALS_NOT_VALID"}, http.StatusBadRequest)
		return
	}

	claims, ok := token.Claims.(*MicrosoftClaims)
	if !ok {
		helper.SetError(w, types.Error{HTTPCode: http.StatusBadRequest, Message: "Credentials not valid", Code: "CREDENTIALS_NOT_VALID"}, http.StatusBadRequest)
		return
	}
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]*session.Grant)

	// For sign-in, always use "root" as userId
	authData, erruser := s.userUsecase.GetDetail(r.Context(), claims.Email)
	if erruser != nil {
		helper.SetError(w, types.Error{Message: "Failed to get user auth: " + erruser.Error(), Code: "SERVER_ERROR"}, http.StatusInternalServerError)
		return
	}
	if authData.MIP.Email != claims.Email {
		helper.SetError(w, types.Error{HTTPCode: http.StatusBadRequest, Message: "Credentials not valid", Code: "CREDENTIALS_NOT_VALID"}, http.StatusBadRequest)
		return
	}

	for tenantID, auth := range authData.Authorization {
		var arrUserGroup []string
		for ug := range auth.UserGroupID {
			arrUserGroup = append(arrUserGroup, ug)
		}
		userGroup := strings.Join(arrUserGroup, ",")
		grant := &session.Grant{
			UserId:             tenantID,
			GroupId:            userGroup, // multiple group id separated by ','
			Name:               auth.Name,
			UiAndApiPermission: auth.UiAndApiPermission,
		}
		grants[tenantID] = grant
	}

	if authData.ID == "" {
		helper.SetError(w, types.Error{
			HTTPCode: http.StatusUnauthorized,
			Code:     "UNAUTHORIZED",
			Message:  "You do not have access to any organization! Please contact API owner team for getting one ☎️",
		}, http.StatusUnauthorized)
		return
	}

	newPayload, err := proto.Marshal(&session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claims.Issuer,
			Sub:      claims.Subject,
			Name:     authData.Profile.DisplayName,
			Nickname: claims.Username,
			Email:    claims.Email,
		},
		Grants:       grants,
		SignInMethod: "mip",
		SignInEmail:  claims.Email,
		IsSuperAdmin: false,
	})
	if err != nil {
		helper.SetError(w, types.Error{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"}, http.StatusInternalServerError)
		return
	}

	if s.signing == nil {
		log.Error().Msg("SIGNING NIL LOH")
		if r.Context().Err() != nil {
			return
		}
		helper.SetError(w, types.Error{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"}, http.StatusInternalServerError)
		return
	}

	expiry := time.Now().Add(time.Duration(60*9) * time.Minute) // long-lived token
	newToken, errUC := s.signing.Sign(r.Context(), newPayload, expiry)
	if errUC != nil {
		if r.Context().Err() != nil {
			return
		}
		helper.SetError(w, types.Error{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"}, http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(&types.CommonResponse{
		Success: SignInResponse{
			IDToken: &newToken,
			LoginProfile: &Profile{
				DisplayName:      authData.Profile.DisplayName,
				Email:            claims.Email,
				ImageURL:         authData.Profile.ImageURL,
				Avatar1x1URL:     authData.Profile.Avatar1x1URL,
				Background3x1URL: authData.Profile.Background3x1URL,
			},
			Locale: lang,
			// collection of grants NOT signed, for debugging.
			// DO NOT USE THIS FOR BACK END VALIDATION
			Grants: grants,
			Expiry: expiry.Format(time.RFC3339),
		},
	})
	if err != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Err(err).Msgf("Failed to parse payload")
		helper.SetError(w, types.Error{Message: "Failed to parse response", Code: "SERVER_ERROR"}, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

func verifyToken(r *http.Request) (*jwt.Token, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil, fmt.Errorf("please specify 'Authorization' in header")
	}

	tokenString := strings.Replace(auth, "Bearer ", "", -1)

	keySet, err := jwk.Fetch(r.Context(), "https://login.microsoftonline.com/common/discovery/v2.0/keys")

	token, err := jwt.ParseWithClaims(tokenString, &MicrosoftClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwa.RS256.String() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid header not found")
		}

		keys, ok := keySet.LookupKeyID(kid)
		if !ok {
			return nil, fmt.Errorf("key %v not found", kid)
		}

		publickey := &rsa.PublicKey{}
		err = keys.Raw(publickey)
		if err != nil {
			return nil, fmt.Errorf("could not parse pubkey")
		}

		return publickey, nil
	})

	if err != nil {
		return nil, err
	}
	return token, nil
}
