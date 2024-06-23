package redis

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"

	types "github.com/desain-gratis/common/types/http"
	// "github.com/desain-gratis/common/repository/limiter"
)

var (
// _ limiter.Repository = &defaultHandler{}
)

type defaultHandler struct {
	client *redis.Client
}

func New(client *redis.Client) *defaultHandler {
	return &defaultHandler{
		client: client,
	}
}

func (d *defaultHandler) Get(ctx context.Context, userID, key string) (counter int, remaining time.Duration, err *types.CommonError) {
	combinedKey := userID + "|" + key

	str := d.client.Get(ctx, combinedKey)
	if str.Err() != nil {
		if str.Err() == redis.Nil {
			return 0, 0, nil
		}
		return 0, 0, &types.CommonError{
			Errors: []types.Error{
				{
					Message: str.Err().Error(),
					Code:    "FAILED_TO_GET_LIMITER",
				},
			},
		}
	}

	strTTL := d.client.TTL(ctx, combinedKey)
	if strTTL.Err() != nil {
		return 0, 0, &types.CommonError{
			Errors: []types.Error{
				{
					Message: strTTL.Err().Error(),
					Code:    "FAILED_TO_GET_LIMITER",
				},
			},
		}
	}

	counter, _err := str.Int()
	if _err != nil {
		return 0, 0, &types.CommonError{
			Errors: []types.Error{
				{
					Message: _err.Error(),
					Code:    "FAILED_TO_CONVERT_TO_INTEGER",
				},
			},
		}
	}

	_ttl := strTTL.Val()

	return counter, _ttl, nil
}

// TODO after MVP implement properly https://redis.io/commands/INCR using LUA if possible
func (d *defaultHandler) Increment(ctx context.Context, userID, key string, expiry time.Duration) (err *types.CommonError) {
	combinedKey := userID + "|" + key

	res := d.client.Incr(ctx, combinedKey)
	if res.Err() != nil {
		return &types.CommonError{
			Errors: []types.Error{
				{
					Code:    "FAILED_TO_INCREMENT",
					Message: res.Err().Error(),
				},
			},
		}
	}

	if res.Val() == 1 {
		exp := d.client.Expire(ctx, combinedKey, expiry)
		if exp.Err() != nil {
			return &types.CommonError{
				Errors: []types.Error{
					{
						Code:    "FAILED_TO_EXPIRE",
						Message: exp.Err().Error(),
					},
				},
			}
		}
	}

	return nil
}
