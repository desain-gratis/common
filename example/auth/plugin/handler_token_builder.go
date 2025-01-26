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
	_ authapi.TokenBuilder = (&auth{}).AdminToken
	_ authapi.TokenBuilder = (&auth{}).UserToken
)

type auth struct {
	authUser   mycontent.Usecase[*entity.Payload]
	adminEmail map[string]struct{}
}

func TokenPublisher(authUser mycontent.Usecase[*entity.Payload], adminEmail map[string]struct{}) *auth {
	return &auth{
		authUser:   authUser,
		adminEmail: adminEmail,
	}
}

func (a *auth) AdminToken(r *http.Request, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError) {
	claim := authapi.GetGoogleClaim(auth.Claims)

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
		SignInMethod: "GSI",
		SignInEmail:  claim.Email,
		IsSuperAdmin: true, // ADMIN
	}

	return tokenData, tokenData, expiry, nil
}

func (a *auth) UserToken(r *http.Request, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError) {
	claim := authapi.GetGoogleClaim(auth.Claims)

	// Locale
	// TODO parse to clean string
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]*session.Grant)

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

	for tenantID, auth := range userData.Authorization {
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

	expiry = time.Now().Add(time.Duration(60*9) * time.Minute) // long-lived token

	apiData = &authapi.SignInResponse{
		LoginProfile: &authapi.Profile{
			DisplayName:      claim.Name,
			Email:            claim.Email,
			ImageURL:         userData.Profile.ImageURL,
			Avatar1x1URL:     userData.Profile.Avatar1x1URL,
			Background3x1URL: userData.Profile.Background3x1URL,
		},
		Locale: lang,

		// collection of grants NOT signed, for debugging.
		// DO NOT USE THIS FOR BACK END VALIDATION
		Grants: grants,
		Expiry: expiry.Format(time.RFC3339),
	}

	return &session.SessionData{
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
	}, apiData, expiry, nil
}
