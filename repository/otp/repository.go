package otp

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

type OTPData struct {
	Nonce string
	OTP   string
}

type VerifyResponse struct {
	Verified bool `json:"verified"`
}

type InitiateData struct {
	Expiry            time.Duration `json:"expiry,omitempty"`
	Attempts          int           `json:"attempts,omitempty"`
	MaxAttempts       int           `json:"max_attempts,omitempty"`
	AttemptsResetTime time.Time     `json:"attempts_reset_time,omitempty"`
}

// Repository for One Time Password State
type Repository interface {
	Get(ctx context.Context, oidcIssuer, oidcSubject string) (payload []byte, expiry time.Time, errUC *types.CommonError)
	Set(ctx context.Context, oidcIssuer, oidcSubject string, payload []byte, expiry time.Time) (errUC *types.CommonError)
	Expire(ctx context.Context, oidcIssuer, oidcSubject string) (errUC *types.CommonError)
}
