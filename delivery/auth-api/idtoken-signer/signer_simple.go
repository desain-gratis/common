package idtokensigner

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
	types "github.com/desain-gratis/common/types/http"
	"github.com/golang-jwt/jwt"
)

var (
	ErrInvalidSigningMethod  = errors.New("invalid signing method")
	ErrKeyIdentifierNotFound = errors.New("key identifier not found")
	ErrKeyIdentifierEmpty    = errors.New("key identifier empty")
	ErrInvalidToken          = errors.New("invalid token")
	ErrInvalidClaim          = errors.New("invalid claim")
	ErrInvalidPayload        = errors.New("invalid payload")
)

var _ authapi.TokenSigner = &simpleSigner{}
var _ authapi.TokenVerifier = &simpleSigner{}

const KID_HEADER = "kid"
const PAYLOAD_CLAIM = "payload"

type CustomClaim struct {
	jwt.StandardClaims
	Payload string `json:"payload"`
}

type simpleSigner struct {
	issuer   string
	hmacKeys map[string]string
	keyID    string
}

func New(issuer string, hmacKeys map[string]string, keyID string) *simpleSigner {
	return &simpleSigner{
		issuer:   issuer,
		hmacKeys: hmacKeys,
		keyID:    keyID,
	}
}

func (s *simpleSigner) Sign(ctx context.Context, claim []byte, expire time.Time) (token string, errUC *types.CommonError) {
	_token := jwt.NewWithClaims(jwt.SigningMethodES256, CustomClaim{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expire.Unix(),
			Issuer:    s.issuer,
		},
		Payload: base64.RawStdEncoding.EncodeToString(claim),
	})
	_token.Header[KID_HEADER] = s.keyID

	token, err := _token.SignedString(s.hmacKeys[s.keyID])
	if err != nil {
		return "", &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "GENERATE_TOKEN_FAILED",
					HTTPCode: http.StatusInternalServerError,
					Message:  "Failed to generate token",
				},
			},
		}
	}
	return token, nil
}

func (s *simpleSigner) Verify(ctx context.Context, token string) (claim []byte, errUC *types.CommonError) {
	claim, err := ParseHMACJWTToken(s.hmacKeys, token)
	if err != nil {
		return nil, &types.CommonError{
			Errors: []types.Error{
				{
					Code:     "VERIFY_TOKEN_FAILED",
					HTTPCode: http.StatusInternalServerError,
					Message:  "Failed to verify token",
				},
			},
		}
	}
	return claim, nil
}

func (s *simpleSigner) Keys(ctx context.Context) ([]authapi.Keys, *types.CommonError) {
	result := make([]authapi.Keys, 0, len(s.hmacKeys))
	for keyID, key := range s.hmacKeys {
		result = append(result, authapi.Keys{
			KeyID: keyID, CreatedAt: "now", Key: key,
		})
	}
	return result, nil
}

// https://pkg.go.dev/github.com/golang-jwt/jwt#example-Parse-Hmac
func ParseHMACJWTToken(hmacKeys map[string]string, token string) (payload []byte, err error) {
	parsed, err := jwt.Parse(token, func(parsed *jwt.Token) (interface{}, error) {
		if _, ok := parsed.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSigningMethod //  token.Header["alg"]
		}

		key, ok := parsed.Header[KID_HEADER].(string)
		if !ok {
			return nil, err
		}
		secret := hmacKeys[key]

		return secret, nil
	})

	if !parsed.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	data, ok := claims[PAYLOAD_CLAIM]
	if !ok {
		return []byte(""), nil // just empty payload
	}

	b, ok := data.(string)
	if !ok {
		return nil, ErrInvalidPayload
	}

	result, err := base64.RawStdEncoding.DecodeString(b)
	if err != nil {
		return nil, ErrInvalidPayload
	}

	return result, nil
}
