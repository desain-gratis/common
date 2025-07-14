package authapi

import (
	"strconv"

	"github.com/desain-gratis/common/types/protobuf/session"
)

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
