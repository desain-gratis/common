package jwthmac

import (
	"time"

	"github.com/desain-gratis/common/utility/secret/hmac/hardcode"
)

// Utility to build & sign JWT token using HMAC signing method with (with symetric key)
// It is used for simple email validation
type Utility interface {
	BuildHMACJWTToken(payload []byte, expireAt time.Time, hmacKeyID string) (token string, err error)
	ParseHMACJWTToken(token string) (payload []byte, err error)
	Store(keyID string, secret string) (err error)
	Get(keyID string) (secret string, ok bool, err error)
}

var DefaultHandler Utility = hardcode.New()

// usualy payload is the serialized protobuf message
func BuildHMACJWTToken(payload []byte, expireAt time.Time, hmacKeyID string) (token string, err error) {
	return DefaultHandler.BuildHMACJWTToken(payload, expireAt, hmacKeyID)
}

// https://pkg.go.dev/github.com/golang-jwt/jwt#example-Parse-Hmac
func ParseHMACJWTToken(token string) (payload []byte, err error) {
	return DefaultHandler.ParseHMACJWTToken(token)
}

func Store(keyID string, secret string) (err error) {
	return DefaultHandler.Store(keyID, secret)
}

func Get(keyID string) (secret string, ok bool, err error) {
	return DefaultHandler.Get(keyID)
}
