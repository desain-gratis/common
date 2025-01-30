package plugin

import (
	"net/http"
	"strings"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	"github.com/desain-gratis/common/example/auth/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"github.com/desain-gratis/common/usecase/mycontent"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/proto"
)

var (
	_ authapi.TokenBuilder = (&auth{}).AdminOnlyToken
	_ authapi.TokenBuilder = (&auth{}).UserToken
)

type auth struct {
	authUser   mycontent.Usecase[*entity.UserAuthorization]
	adminEmail map[string]struct{}
}

// TokenPublisher publish token based on validated identity token.
// Identity provider validation provided by authapi.
func TokenPublisher(authUser mycontent.Usecase[*entity.UserAuthorization], adminEmail map[string]struct{}) *auth {
	return &auth{
		authUser:   authUser,
		adminEmail: adminEmail,
	}
}

func (a *auth) AdminOnlyToken(r *http.Request, authMethod string, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError) {
	claim := authapi.GetOIDCClaims(auth.Claims)

	if _, ok := a.adminEmail[claim.Email]; !ok {
		errUC := &types.CommonError{
			Errors: []types.Error{
				{
					HTTPCode: http.StatusUnauthorized,
					Code:     "UNAUTHORIZED",
					Message:  "You do not have access to any organization! Please contact API owner team for getting one ☎️",
				},
			},
		}
		return nil, nil, expiry, errUC
	}

	expiry = time.Now().Add(time.Duration(5) * time.Minute) // long-lived token

	tokenData = &session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claim.Iss,
			Sub:      claim.Sub,
			Name:     claim.Name,
			Nickname: claim.Nickname,
			Email:    claim.Email,
		},
		SignInMethod: authMethod,
		SignInEmail:  claim.Email,
		IsSuperAdmin: true, // ADMIN
	}

	return tokenData, tokenData, expiry, nil
}

func (a *auth) UserToken(r *http.Request, authMethod string, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError) {
	claim := authapi.GetOIDCClaims(auth.Claims)

	// Locale
	// TODO parse to clean string
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]*session.Grant)

	// notice that "root" is hardcoded
	// also, we get the authentication based on claim.Email.
	authData, errUserUC := a.authUser.Get(r.Context(), "root", []string{}, claim.Email)
	if errUserUC != nil {
		return nil, nil, expiry, errUserUC
	}

	if len(authData) != 1 {
		uerrUserUC := &types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to get user auth: " + errUserUC.Err().Error(), Code: "NOT_FOUND"},
			}}

		return nil, nil, expiry, uerrUserUC
	}

	userData := authData[0]

	expiry = time.Now().Add(time.Duration(60*9) * time.Minute) // long-lived token

	var img string
	if userData.DefaultProfile.Avatar1x1 != nil {
		img = userData.DefaultProfile.Avatar1x1.ThumbnailUrl
	}

	for namespace, auth := range userData.Authorization {
		grants[namespace] = &session.Grant{
			UiAndApiPermission: auth.UiAndApiPermission,
			GroupId:            auth.UserGroupID2, // todo:
		}
	}

	tokenData = &session.SessionData{
		NonRegisteredId: &session.OIDCClaim{
			Iss:      claim.Iss,
			Sub:      claim.Sub,
			Name:     claim.Name,
			Nickname: claim.Nickname,
			Email:    claim.Email,
		},
		Grants:       grants,
		SignInMethod: authMethod,
		SignInEmail:  claim.Email,
		IsSuperAdmin: false,
	}

	apiData = &authapi.SignInResponse{
		LoginProfile: &authapi.Profile{
			DisplayName: claim.Name,
			Email:       claim.Email,
			ImageURL:    img,
		},
		Locale: lang,
		// collection of grants NOT signed, for debugging.
		// DO NOT USE THIS FOR BACK END VALIDATION
		Grants: grants,
		Expiry: expiry.Format(time.RFC3339),
	}

	return tokenData, apiData, expiry, nil
}
