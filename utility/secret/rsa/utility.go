package jwtrsa

import (
	"time"

	"github.com/desain-gratis/common/utility/secret/rsa/hardcode"
)

// Provider to build & sign JWT token using RSA signing method with (with symetric key)
// It is used for simple email validation
type Provider interface {
	BuildRSAJWTToken(payload []byte, expireAt time.Time, rsaKeyID string) (token string, err error)

	ParseRSAJWTToken(token string, keyID string) (payload []byte, err error)

	StorePrivateKey(keyID string, privateKeyPEM string) (err error)

	StorePublicKey(keyID string, publicKeyPEM string) (err error)

	GetPublicKey(keyID string) (key []byte, found bool, err error)
}

var Default Provider = hardcode.New("https://desain.gratis")

// usualy payload is the serialized protobuf message
func BuildRSAJWTToken(payload []byte, expireAt time.Time, keyID string) (token string, err error) {
	return Default.BuildRSAJWTToken(payload, expireAt, keyID)
}

// https://pkg.go.dev/github.com/golang-jwt/jwt
func ParseRSAJWTToken(token string, keyID string) (payload []byte, err error) {
	return Default.ParseRSAJWTToken(token, keyID)
}

func StorePrivateKey(keyID string, privateKeyPEM string) (err error) {
	return Default.StorePrivateKey(keyID, privateKeyPEM)
}

func StorePublicKey(keyID string, publicKeyPEM string) (err error) {
	return Default.StorePublicKey(keyID, publicKeyPEM)
}

func GetPublicKey(keyID string) ([]byte, bool, error) {
	return Default.GetPublicKey(keyID)
}
