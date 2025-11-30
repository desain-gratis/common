package plugin

import (
	"net/http"
	"strings"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	"github.com/desain-gratis/common/delivery/mycontent-api/mycontent"
	"github.com/desain-gratis/common/example/auth/entity"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/proto"
)

var (
	_ authapi.TokenBuilder = &userAuth{}
)

const (
	authKey key = "auth"
)

type userAuth struct {
	authUser      mycontent.Usecase[*entity.UserAuthorization]
	expiryMinutes int
}

// AdminAuthLogic from id token (google, microsoft, etc.) in to our own self signed token
func NewUserAuthLogic(authUser mycontent.Usecase[*entity.UserAuthorization], expiryMinutes int) *userAuth {
	return &userAuth{
		authUser:      authUser,
		expiryMinutes: expiryMinutes,
	}
}

func (a *userAuth) BuildToken(r *http.Request, authMethod string, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err error) {
	claim := authapi.GetOIDCClaims(auth.Claims)

	// Locale
	// TODO parse to clean string
	lang := strings.Split(r.Header.Get("Accept-Language"), ";")

	// Enterprise capability (login based on organization)
	grants := make(map[string]*session.Grant)

	// notice that "root" is hardcoded
	// also, we get the authentication based on claim.Email.
	authData, err := a.authUser.Get(r.Context(), "root", []string{}, claim.Email)
	if err != nil {
		return nil, nil, expiry, err
	}

	if len(authData) != 1 {
		uerrUserUC := &types.CommonError{
			Errors: []types.Error{
				{Message: "Failed to get user auth: " + err.Error(), Code: "NOT_FOUND"},
			}}

		return nil, nil, expiry, uerrUserUC
	}

	userData := authData[0]

	expiry = time.Now().Add(time.Duration(a.expiryMinutes) * time.Minute) // long-lived token

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

	apiData = &AuthData{
		LoginProfile: &Profile{
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
