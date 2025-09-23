package notifierapi

import (
	"context"
)

// Topic implementation
type Topic interface {
	Subscribe() (string, Subscription)
	GetSubscription(id string) (Subscription, error)
	Broadcast(ctx context.Context, message any)
}

// It can be a "FSM" that lives on each request
type Subscription interface {
	// Subscription is also a listener
	Listener

	// Publish to this single subscription
	// intended to be called by Broker or for debugging purpose
	Publish(ctx context.Context, message any) error
}

type Listener interface {
	Listen(ctx context.Context) <-chan any
}
