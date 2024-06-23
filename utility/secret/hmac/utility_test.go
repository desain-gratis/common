package jwthmac_test

import (
	"testing"
	"time"

	jwthmac "github.com/desain-gratis/common/utility/secret/hmac"
	"github.com/desain-gratis/common/utility/secret/hmac/hardcode"
)

func Test_EncodeDecodeJWT(t *testing.T) {
	jwthmac.DefaultHandler = hardcode.New()

	jwthmac.DefaultHandler.Store("development", "alamantap")

	want := []byte("Example data")
	expireAt := time.Now().Add(15 * time.Minute).Truncate(1 * time.Millisecond)
	token, err := jwthmac.BuildHMACJWTToken(want, expireAt, "development")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	result, err := jwthmac.ParseHMACJWTToken(token)
	if err != nil {
		t.FailNow()
	}

	if string(result) != string(want) {
		t.FailNow()
	}
}

func Test_ExpiredToken(t *testing.T) {
	jwthmac.DefaultHandler = hardcode.New()

	jwthmac.DefaultHandler.Store("development", "alamantap")

	want := []byte("This token should not be valid, since it's already expired")
	expireAt := time.Now().Add(-1 * time.Minute).Truncate(1 * time.Millisecond)
	token, err := jwthmac.BuildHMACJWTToken(want, expireAt, "development")
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	_, err = jwthmac.ParseHMACJWTToken(token)
	if err == nil {
		t.FailNow()
	}
}
