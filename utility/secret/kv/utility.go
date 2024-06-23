package jwtkv

import (
	"github.com/desain-gratis/common/utility/secret/kv/hardcode"
)

// Utility to build & sign JWT token using RSA signing method with (with symetric key)
// It is used for simple email validation
type Utility interface {
	Store(keyID string, secret string) (err error)
	Get(keyID string) (secret string, ok bool, err error)
}

var DefaultHandler Utility = hardcode.New()

// usualy payload is the serialized protobuf message
func Store(keyID string, secret string) (err error) {
	return DefaultHandler.Store(keyID, secret)
}

// https://pkg.go.dev/github.com/golang-jwt/jwt
func Get(keyID string) (secret string, ok bool, err error) {
	return DefaultHandler.Get(keyID)
}
