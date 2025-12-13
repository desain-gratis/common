package idtokensigner

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	authapi "github.com/desain-gratis/common/delivery/auth-api"
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

func (s *simpleSigner) Sign(ctx context.Context, claim []byte, expire time.Time) (token string, errUC error) {
	_token := jwt.NewWithClaims(jwt.SigningMethodHS512, CustomClaim{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expire.Unix(),
			Issuer:    s.issuer,
		},
		Payload: base64.RawStdEncoding.EncodeToString(claim),
	})
	_token.Header[KID_HEADER] = s.keyID

	token, err := _token.SignedString([]byte(s.hmacKeys[s.keyID]))
	if err != nil {
		return "", fmt.Errorf("failed to sign using key ID %v %v: %w", s.keyID, s.hmacKeys[s.keyID], err)
	}
	return token, nil
}

func (s *simpleSigner) Verify(ctx context.Context, token string) (claim []byte, errUC error) {
	claim, err := ParseHMACJWTToken(s.hmacKeys, token)
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func (s *simpleSigner) Keys(ctx context.Context) ([]authapi.Keys, error) {
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
			return nil, fmt.Errorf("token doesn't have a 'kid' header %w", err)
		}
		secret := []byte(hmacKeys[key])

		return secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	if !parsed.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("token doesn't contain claim: %w", ErrInvalidToken)
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
