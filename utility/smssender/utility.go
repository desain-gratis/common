package smssender

import (
	"context"
	"errors"
)

var (
	ErrNotImplemented = errors.New("not implemented")
)

type Utility interface {
	Send(ctx context.Context, phoneNumber string, payload string) error
}

var Default Utility = nil

func Send(ctx context.Context, phoneNumber string, payload string) error {
	if Default == nil {
		return ErrNotImplemented
	}
	return Default.Send(ctx, phoneNumber, payload)
}
