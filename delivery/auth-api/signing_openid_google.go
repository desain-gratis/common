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
	"github.com/desain-gratis/common/usecase/authorization"
	authorization_handler "github.com/desain-gratis/common/usecase/authorization/handler"
	"github.com/desain-gratis/common/usecase/mycontent"
	"github.com/desain-gratis/common/utility/enterpriseauth"
	enterpiseauth "github.com/desain-gratis/common/utility/enterpriseauth"
	"github.com/desain-gratis/common/utility/iplocation"
	jwtrsa_hardcode "github.com/desain-gratis/common/utility/secret/rsa/hardcode"
	"github.com/desain-gratis/common/utility/secretkv"
)

type LoginResponse struct {
	// Profile from our DB
	Profile *Profile `json:"profile,omitempty"`

	// Profile from OIDC provider
	LoginProfile *Profile `json:"login_profile,omitempty"`

	Registration      *Registration      `json:"registration,omitempty"`
	ProfileCompletion *ProfileCompletion `json:"profile_completion,omitempty"`
	Locale            []string           `json:"locale,omitempty"`
	CountryCode       *string            `json:"country_code,omitempty"`
	IDToken           *string            `json:"id_token,omitempty"`
	Grants            map[string]Grant   `json:"grants,omitempty"`
}

type Grant struct {
	UserID  string `json:"user_id,omitempty"`
	URL     string `json:"url,omitempty"`
	ApiURL  string `json:"api_url,omitempty"`
	Email   string `json:"email,omitempty"`
	IDToken string `json:"id_token,omitempty"`
	Expiry  string `json:"expiry,omitempty"`
}

type Profile struct {
	URL          string `json:"url"`
	DisplayName  string `json:"display_name"`
	ImageDataURL string `json:"image_data_url"`
	ImageURL     string `json:"image_url"`
	Email        string `json:"email"`
}

type Registration struct {
	PhoneNumberVerified bool `json:"phone_number_verified"`

	// If pin configured
	PinConfigured bool `json:"pin_configured"`

	// Username if alread configured
	Username string `json:"username"`

	SMSOTPInputPending bool `json:"sms_otp_input_pending"`
}

type ProfileCompletion struct {
	Picture     bool `json:"picture"`
	Description bool `json:"description"`
}

type googleSignInService struct {
	*signingService
	googleAuth authorization.VerifierOf[idtoken.Payload]
	profile    mycontent.Usecase[*session.Profile]

	enteprise map[string]*signingService
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

// I think should return authorization token for verifying login
// and also hash of user ID contained inside of JWT Token
func NewGoogleSignInService(
	googleAuth authorization.VerifierOf[idtoken.Payload],
	authorization authorization.Usecase,
	profile mycontent.Usecase[*session.Profile],
	kvStore secretkv.Provider,
) *googleSignInService {

	// Initialize usecase by organization
	enterprise := make(map[string]*signingService)
	allOrg, _ := enterpriseauth.GetAll(context.Background())
	for orgName, org := range allOrg {
		// Dynamically create auth usecase
		enterprise[orgName] = New(
			authorization_handler.New(
				authorization_handler.Config{
					SigningConfig: authorization_handler.SigningConfig{
						Secret:   org.SignInPK,
						PollTime: 30 * time.Second,
						ID:       org.SignInKeyID,
					},
					Issuer:             orgName,
					TokenExpiryMinutes: 60 * 8,
				},
				jwtrsa_hardcode.New(org.URL),
				kvStore,
			),
		)
	}

	return &googleSignInService{
		googleAuth: googleAuth,
		signingService: &signingService{
			authorization: authorization,
		},
		profile:   profile,
		enteprise: enterprise,
	}
}

// Debug allows validates token published by this service
func (s *googleSignInService) SignIn(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// maybe get hostname as well..
	payload, errUC := s.verifyAuthorizationHeader(r.Context(), r.Header.Get("Authorization"))
	if errUC != nil {
		errMessage := types.SerializeError(errUC)
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errMessage)
		return
	}

	claim := getClaim(payload.Claims)

	// 2. Get profile data by user ID -> mycontent usecase
	// Check Profile is s.profile is not nil
	var profile *Profile
	if s.profile != nil {
		_profile, errUC := s.profile.Get(r.Context(), claim.Sub, "", claim.Iss)
		if errUC != nil {
			if r.Context().Err() != nil {
				return
			}
		}
		if len(_profile) > 0 {
			profile = &Profile{
				DisplayName:  _profile[0].DisplayName,
				URL:          _profile[0].Url,
				ImageDataURL: _profile[0].ImageDataUrl,
				ImageURL:     _profile[0].ImageUrl,
			}
		}
	}

	// 3. Get registration data by user ID -> registration usecase (new)
	// 4. Get profile completion data -> mycontent usecase, related to #2
	// 5. Get IP Geolocation -> utility common
	// 6. Build locale fallback
	// 7. Generate ID token

	// ip error are ignored, client must handle this empty result
	forwardedIP := r.Header.Get("X-Real-IP") // since the service is sitting behind NGINX
	ip, err := iplocation.Get(context.Background(), forwardedIP)
	log.Info().Msgf("IP Detail %+v %+v", ip, forwardedIP)
	if err != nil {
		log.Err(err).Msgf("Cannot get IP Detail %+v", forwardedIP)
	}
	var countryCode *string
	if ip != nil {
		countryCode = &ip.CountryCode2
	}

	// Locale
	// TODO parse to clean string
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]Grant)
	_grants := make(map[string]*session.Grant)
	authorizedOrgs, _ := enterpiseauth.Get(r.Context(), claim.Email)

	for org, data := range authorizedOrgs {
		if data.Organization.Auth != enterpiseauth.AUTH_GSI {
			continue
		}

		if _, ok := s.enteprise[org]; !ok {
			// Moved to init
			log.Warn().Msgf("Auth usecase not yet created for org %+v, creating one immediately.", org)
			continue
		}

		grant := &session.Grant{
			Url:    data.Organization.URL,
			Email:  data.Email,
			Roles:  data.Roles,
			UserId: data.UserID,
		}
		newPayload, err := proto.Marshal(grant)
		if err != nil {
			log.Err(err).Msg("Could not marshal payload")
			continue
		}

		token, expiry, errUC := s.enteprise[org].authorization.Sign(r.Context(), newPayload)
		if errUC != nil {
			if r.Context().Err() != nil {
				return
			}
			log.Err(errUC.Err()).Msg("Could not sign token")
			continue
		}

		// collection of grants for (not signed)
		grants[data.Organization.URL] = Grant{
			URL:     data.Organization.URL,
			ApiURL:  data.Organization.ApiURL,
			IDToken: token,
			UserID:  data.UserID,
			Expiry:  expiry.Format(time.RFC3339),
			// Email:   data.Email,
		}

		// should be signed
		_grants[data.Organization.URL] = grant
	}

	newPayload, err := proto.Marshal(&session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claim.Iss,
			Sub:      claim.Sub,
			Name:     claim.Name,
			Nickname: claim.Nickname,
			Email:    claim.Email,
		},
		Grants: _grants,
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

	var newToken string
	if s.authorization != nil {
		newToken, _, errUC = s.authorization.Sign(r.Context(), newPayload)
		log.Debug().Msgf("Signed message for  %v %v \n", newToken, err)
		if errUC != nil {
			if r.Context().Err() != nil {
				return
			}
			errMessage := types.SerializeError(errUC)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(errMessage)
			return
		}
	}

	log.Info().Msgf("%+v", claim)
	resp, err := json.Marshal(&types.CommonResponse{
		Success: LoginResponse{
			IDToken:     &newToken,
			CountryCode: countryCode,
			LoginProfile: &Profile{
				DisplayName: claim.Name,
				Email:       claim.Email,
			},
			Profile: profile,
			Locale:  lang,
			Grants:  grants,
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
	if len(token) == 0 || (len(token) >= 2 && token[1] == "") {
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

	uc, ok := s.enteprise[orgID]
	if !ok {
		cer := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusNotFound,
					Code:     "ORGANIZATION_NOT_FOUND",
					Message:  "Keys with org `" + orgID + "` not found",
				},
			},
		}
		errMessage := types.SerializeError(cer)
		w.WriteHeader(http.StatusNotFound)
		w.Write(errMessage)
		return
	}

	// let the base delivery execute
	uc.Keys(w, r, p)
}
