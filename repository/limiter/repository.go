package limiter

import (
	"context"
	"time"

	types "github.com/desain-gratis/common/types/http"
)

// Implementation needs to be aware of distributed system nature
type Repository interface {
	Get(ctx context.Context, oidcIssuer, oidcSubject, id string) (counter int, expiredAt time.Time, err *types.CommonError)
	Increment(ctx context.Context, oidcIssuer, oidcSubject, id string, expiredAt time.Time) (err *types.CommonError)
	Expire(ctx context.Context, oidcIssuer, oidcSubject, id string) (err *types.CommonError)
}

type unlimited struct{}

func NewUnlimited() *unlimited {
	return &unlimited{}
}

func (u *unlimited) Get(ctx context.Context, userID, id string) (counter int, createdAt time.Time, err *types.CommonError) {
	return 0, time.Time{}, nil
}
func (u *unlimited) Increment(ctx context.Context, userID, id string, expiryAt time.Time) (err *types.CommonError) {
	return nil
}
