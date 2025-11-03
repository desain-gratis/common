package notifier

import (
	"context"
)

type CreateSubscription func(ctx context.Context, id string) Subscription

// Topic implementation
type Topic interface {
	Subscribe(ctx context.Context, fn CreateSubscription) (Subscription, error)
	GetSubscription(id string) (Subscription, error)
	RemoveSubscription(id string) error
	Broadcast(ctx context.Context, message any) error
}

// It can be a "FSM" that lives on each request
type Subscription interface {
	// Subscription is also a listener
	Listener

	// Start receiving message
	Start()

	// Publish to this single subscription
	// intended to be called by state machine / app or for debugging purpose
	Publish(message any) error
}

type Listener interface {
	// ID is the listener ID for a given topic
	ID() string

	Listen() <-chan any
}
