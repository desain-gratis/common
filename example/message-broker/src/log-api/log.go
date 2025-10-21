package logapi

import (
	"context"
)

// Topic implementation
type Topic interface {
	Subscribe() (Subscription, error)
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
	// intended to be called by Broker or for debugging purpose
	Publish(ctx context.Context, message any) error

	// Remove subscription
	Stop()
}

type Listener interface {
	// ID is the listener ID for a given topic
	ID() string

	Listen(ctx context.Context) <-chan any

	IsListening() bool
}
