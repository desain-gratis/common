package hardcode

import (
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt"
)

var (
	ErrInvalidSigningMethod  = errors.New("invalid signing method")
	ErrKeyIdentifierNotFound = errors.New("key identifier not found")
	ErrInvalidToken          = errors.New("invalid token")
	ErrInvalidPayload        = errors.New("invalid payload")
)

const (
	ISS           = "account-service"
	KID_HEADER    = "kid"
	PAYLOAD_CLAIM = "payload"
)

type CustomClaim struct {
	jwt.StandardClaims
	Payload string `json:"payload"`
}

type defaultHandler struct {
	keyLock  *sync.Mutex
	hmacKeys map[string][]byte
}

func New() *defaultHandler {
	return &defaultHandler{
		keyLock:  &sync.Mutex{},
		hmacKeys: make(map[string][]byte),
	}
}

func (d *defaultHandler) Store(keyID string, secret string) (err error) {
	d.keyLock.Lock()
	defer d.keyLock.Unlock()

	d.hmacKeys[keyID] = []byte(secret)

	return nil
}

func (d *defaultHandler) Get(keyID string) (secret string, ok bool, err error) {
	d.keyLock.Lock()
	defer d.keyLock.Unlock()

	_secret, ok := d.hmacKeys[keyID]
	return string(_secret), ok, nil
}

// usualy payload is the serialized protobuf message
func (d *defaultHandler) BuildHMACJWTToken(payload []byte, expireAt time.Time, hmacKeyID string) (token string, err error) {
	signingKey, ok := d.hmacKeys[hmacKeyID]
	if !ok {
		return "", ErrKeyIdentifierNotFound
	}

	_token := jwt.NewWithClaims(jwt.SigningMethodHS512, CustomClaim{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expireAt.Unix(),
			Issuer:    ISS,
		},
		Payload: base64.RawStdEncoding.EncodeToString(payload),
	})
	_token.Header[KID_HEADER] = hmacKeyID

	token, err = _token.SignedString(signingKey)
	if err != nil {
		return "", err
	}

	return token, nil
}

// https://pkg.go.dev/github.com/golang-jwt/jwt#example-Parse-Hmac
func (d *defaultHandler) ParseHMACJWTToken(token string) (payload []byte, err error) {
	parsed, err := jwt.Parse(token, func(parsed *jwt.Token) (interface{}, error) {
		if _, ok := parsed.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSigningMethod //  token.Header["alg"]
		}

		key, ok := parsed.Header[KID_HEADER].(string)
		if !ok {
			return nil, err
		}
		secret := d.hmacKeys[key]

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
