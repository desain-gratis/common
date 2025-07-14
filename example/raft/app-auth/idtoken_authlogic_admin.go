package plugin

import (
	"net/http"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
	"github.com/desain-gratis/common/types/protobuf/session"
	"google.golang.org/api/idtoken"
	"google.golang.org/protobuf/proto"
)

var (
	_ authapi.TokenBuilder = &adminAuth{}
)

type adminAuth struct {
	adminEmail    map[string]struct{}
	expiryMinutes int
}

// AdminAuthLogic from id token (google, microsoft, etc.) in to our own self signed token
func AdminAuthLogic(adminEmail map[string]struct{}, expiryMinutes int) *adminAuth {
	return &adminAuth{
		adminEmail:    adminEmail,
		expiryMinutes: expiryMinutes,
	}
}

func (a *adminAuth) BuildToken(r *http.Request, authMethod string, auth *idtoken.Payload) (tokenData proto.Message, apiData any, expiry time.Time, err *types.CommonError) {
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

	expiry = time.Now().Add(time.Duration(a.expiryMinutes) * time.Minute) // long-lived token

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
