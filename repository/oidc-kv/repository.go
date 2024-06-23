package oidckv

import (
	"context"

	types "github.com/desain-gratis/common/types/http"
)

type Repository[T any] interface {
	Set(ctx context.Context, userID string, data T) *types.CommonError
	Get(ctx context.Context, userID string) (T, *types.CommonError)
	Delete(ctx context.Context, userID string) *types.CommonError
}

// OTP data
// type Data[T any] struct {
// 	OTP      string
// 	Nonce    string
// 	ExpireAt time.Time
// 	Data     T
// }
