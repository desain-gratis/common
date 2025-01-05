package authapi

import (
	"context"
	"encoding/json"
	"io"
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
	userUC "github.com/desain-gratis/common/usecase/user"
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

	maximumRequestLength = 1 << 20
)

type googleSignInService struct {
	*signingService
	googleAuth  signing.VerifierOf[idtoken.Payload]
	adminEmails map[string]struct{}
	// myContentAuth mycontent.Usecase[*GSIData]
	userUsecase userUC.UseCase
}

// I think should return authorization token for verifying login
// and also hash of user ID contained inside of JWT Token
func NewGoogleSignInService(
	googleAuth signing.VerifierOf[idtoken.Payload],
	signing signing.Usecase,
	adminEmails map[string]struct{},
	// myContentAuth mycontent.Usecase[*GSIData],
	userUC userUC.UseCase,
) *googleSignInService {
	return &googleSignInService{
		googleAuth: googleAuth,
		signingService: &signingService{
			signing: signing,
		},
		adminEmails: adminEmails,
		userUsecase: userUC,
		// myContentAuth: myContentAuth,
	}
}

func (s *googleSignInService) UpdateAuth(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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

	// TODO: use bcrypt to hash email

	ucPayload := convertUpdatePayload(clientPayload)

	errUserUC := s.userUsecase.Insert(r.Context(), ucPayload)
	if errUserUC != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to insert user: " + errUserUC.Error(), Code: "SERVER_ERROR"},
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

func (s *googleSignInService) SignIn(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, errUC := s.verifyAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	claim := getClaim(payload.Claims)

	// Locale
	// TODO parse to clean string
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]*session.Grant)

	// For sign-in, always use "root" as userId
	// authData, errUC := s.myContentAuth.Get(r.Context(), "root", "", claim.Email)
	// if errUC != nil {
	// 	errMessage := types.SerializeError(errUC)
	// 	w.WriteHeader(http.StatusBadRequest)
	// 	w.Write(errMessage)
	// 	return
	// }

	authData, errUserUC := s.userUsecase.GetDetail(r.Context(), claim.Email)
	if errUserUC != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to get user auth: " + errUserUC.Error(), Code: "SERVER_ERROR"},
			}})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
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
		errUC := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED",
					Message:  "You do not have access to any organization! Please contact API owner team for getting one ☎️",
				},
			},
		}
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	// authDatum := authData[0]

	// for org, auth := range authDatum.Authorization {
	// 	// collection of grants to be signed
	// 	grant := &session.Grant{
	// 		UserId:             auth.UserId,
	// 		GroupId:            auth.GroupId,
	// 		Name:               auth.Name,
	// 		UiAndApiPermission: auth.UiAndApiPermission,
	// 	}
	// 	grants[org] = grant
	// }

	newPayload, err := proto.Marshal(&session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claim.Iss,
			Sub:      claim.Sub,
			Name:     claim.Name,
			Nickname: claim.Nickname,
			Email:    claim.Email,
		},
		Grants:       grants,
		SignInMethod: "GSI",
		SignInEmail:  claim.Email,
		IsSuperAdmin: false,
	})
	if err != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	if s.signing == nil {
		log.Error().Msg("SIGNING NIL LOH")
		if r.Context().Err() != nil {
			return
		}
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	expiry := time.Now().Add(time.Duration(60*9) * time.Minute) // long-lived token
	newToken, errUC := s.signing.Sign(r.Context(), newPayload, expiry)
	if errUC != nil {
		if r.Context().Err() != nil {
			return
		}
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	resp, err := json.Marshal(&types.CommonResponse{
		Success: SignInResponse{
			IDToken: &newToken,
			LoginProfile: &Profile{
				DisplayName:      claim.Name,
				Email:            claim.Email,
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

func (s *googleSignInService) verifyAuthorizationHeader(ctx context.Context, value string) (payload *idtoken.Payload, errUC *types.CommonError) {
	token := strings.Split(value, " ")
	if len(token) < 2 || (len(token) >= 2 && token[1] == "") {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "BAD_REQUEST", HTTPCode: http.StatusBadRequest, Message: "Invalid authentication token"},
			},
		}
	}

	data, errUC := s.googleAuth.VerifyAs(ctx, token[1])
	if errUC != nil {
		return nil, errUC
	}

	return data, nil
}

func getClaim(claims map[string]interface{}) *session.OIDCClaim {
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

func (s *googleSignInService) MultiKeys(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
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

// SuperAdminSignIn validate whether the current user is an admin or not
// The validation happen inside
func (s *googleSignInService) SuperAdminSignIn(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	payload, errUC := s.verifyAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	claim := getClaim(payload.Claims)

	_, ok := s.adminEmails[claim.Email]
	if !ok {
		errUC := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED",
					Message:  "You're not authorized to obtain super-admin token as '" + claim.Email + "'.",
				},
			},
		}
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	newPayload, err := proto.Marshal(&session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claim.Iss,
			Sub:      claim.Sub,
			Name:     claim.Name,
			Nickname: claim.Nickname,
			Email:    claim.Email,
		},
		SignInMethod: "GSI",
		SignInEmail:  claim.Email,
		IsSuperAdmin: true, // TRUE!
	})
	if err != nil {
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	// Sign back
	// (many duplication with non super admin SignIn)
	if s.signing == nil {
		log.Error().Msg("SIGNING NIL LOH")
		if r.Context().Err() != nil {
			return
		}
		errMessage := types.SerializeError(&types.CommonError{
			Errors: []types.Error{
				{Code: "SERVER_ERROR", HTTPCode: http.StatusInternalServerError, Message: "Failed to build token"},
			},
		})
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	expiry := time.Now().Add(time.Duration(180) * time.Minute) // shorter-lived token
	newToken, errUC := s.signing.Sign(r.Context(), newPayload, expiry)
	if errUC != nil {
		if r.Context().Err() != nil {
			return
		}
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errMessage)
		return
	}

	resp, err := json.Marshal(&types.CommonResponse{
		Success: SignInResponse{
			IDToken: &newToken,
			Expiry:  expiry.Format(time.RFC3339),
		},
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
	w.Write(resp)
}

// ParseTokenAsOpenID is a utility function to parse the token published by GoogleSignInService
func ParseTokenAsOpenID(payload []byte) (result *session.SessionData, errUC *types.CommonError) {
	var session session.SessionData
	err := proto.Unmarshal(payload, &session)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "INVALID_TOKEN", HTTPCode: http.StatusBadRequest, Message: "Token schema changed. Please log in again."},
			},
		}
	}

	return &session, nil
}
