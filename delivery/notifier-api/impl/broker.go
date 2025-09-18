package impl

import (
	"context"
	"math/rand/v2"

	notifierapi "github.com/desain-gratis/common/delivery/notifier-api"
)

var _ notifierapi.Broker = &broker{}

// broker without lock, because it is itended to only be called inside FSM
type broker struct {
	// todo: check whether we need lock or not if we're using FSM..
	// of course to be safe we can just add them.

	listener map[uint64]*listener

	listenerBufferSize uint32
	async              bool
	exitMessage        any
}

func NewBroker(async bool, bufferSize uint32, exitMessage any) notifierapi.Broker {
	return &broker{
		listener:           make(map[uint64]*listener),
		listenerBufferSize: bufferSize,
		async:              async,
		exitMessage:        exitMessage,
	}
}

func (s *broker) Subscribe() notifierapi.Subscription {
	id := rand.Uint64() // change id impl to avoid conflict if needed

	listener := &listener{
		ch:          make(chan any, s.listenerBufferSize),
		exitMessage: s.exitMessage,
		async:       s.async,
	}

	s.listener[id] = listener

	return listener
}

func (s *broker) Unsubscribe(id uint64) {
	delete(s.listener, id)
}

func (s *broker) Broadcast(ctx context.Context, message any) {
	for key, listener := range s.listener {
		err := listener.Publish(ctx, message)
		if err != nil {
			delete(s.listener, key)
		}
	}
}
