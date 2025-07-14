package idtokenverifier

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net/http"

	"google.golang.org/api/idtoken"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
	"github.com/golang-jwt/jwt"
	"github.com/julienschmidt/httprouter"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwk"
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

// Microsoft Identity Platform
func MIPAuth() func(httprouter.Handle) httprouter.Handle {
	return func(handler httprouter.Handle) httprouter.Handle {
		return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			// TODO: move init or cache
			keySet, err := jwk.Fetch(r.Context(), "https://login.microsoftonline.com/common/discovery/v2.0/keys")
			if err != nil {
				errUC := &types.CommonError{
					Errors: []types.Error{
						{Code: "INTERNAL_SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Failed to validate"},
					},
				}
				errMessage := types.SerializeError(errUC)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			token, errUC := getToken(r.Header.Get("Authorization"))
			if errUC != nil {
				errMessage := types.SerializeError(errUC)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			var claims MicrosoftClaims
			_, err = jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
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
				errUC := &types.CommonError{
					Errors: []types.Error{
						{Code: "UNAUTHORIZED", HTTPCode: http.StatusUnauthorized, Message: "Unauthorized"},
					},
				}
				errMessage := types.SerializeError(errUC)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(errMessage)
				return
			}

			result := &idtoken.Payload{
				Issuer:   claims.Issuer,
				Audience: claims.Audience,
				Expires:  claims.ExpiresAt,
				IssuedAt: claims.IssuedAt,
				Subject:  claims.Subject,
				Claims: map[string]interface{}{
					"aio":        claims.AIO,
					"email":      claims.Email,
					"login_hint": claims.LoginHint,
					"nonce":      claims.Nonce,
					"oid":        claims.OID,
					"username":   claims.Username,
					"sid":        claims.SID,
					"tenant_id":  claims.TenantID,
					"version":    claims.Version,
					"xms":        claims.XMS,
				},
			}

			ctx := context.WithValue(r.Context(), authapi.IDTokenKey{}, result)
			ctx = context.WithValue(ctx, authapi.IDTokenNameKey{}, "GSI")
			r = r.WithContext(ctx)
			handler(w, r, p)
		}
	}
}

type microsoftVerifier struct {
	clientID string
}

func NewMicrosoftAuth(clientID string) *microsoftVerifier {
	return &microsoftVerifier{
		clientID: clientID,
	}
}

func (g *microsoftVerifier) VerifyAs(ctx context.Context, token string) (*idtoken.Payload, *types.CommonError) {
	// TODO: move init or cache
	keySet, err := jwk.Fetch(ctx, "https://login.microsoftonline.com/common/discovery/v2.0/keys")
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "INTERNAL_SERVER_ERROR", HTTPCode: http.StatusFailedDependency, Message: "Failed to validate"},
			},
		}
	}

	var claims MicrosoftClaims
	_, err = jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
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
		return nil, &types.CommonError{
			Errors: []types.Error{
				{Code: "UNAUTHORIZED", HTTPCode: http.StatusUnauthorized, Message: "Unauthorized"},
			},
		}
	}

	result := &idtoken.Payload{
		Issuer:   claims.Issuer,
		Audience: claims.Audience,
		Expires:  claims.ExpiresAt,
		IssuedAt: claims.IssuedAt,
		Subject:  claims.Subject,
		Claims: map[string]interface{}{
			"aio":        claims.AIO,
			"email":      claims.Email,
			"login_hint": claims.LoginHint,
			"nonce":      claims.Nonce,
			"oid":        claims.OID,
			"username":   claims.Username,
			"sid":        claims.SID,
			"tenant_id":  claims.TenantID,
			"version":    claims.Version,
			"xms":        claims.XMS,
		},
	}

	return result, nil
}
