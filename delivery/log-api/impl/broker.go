package impl

import (
	"context"
	"math/rand/v2"
	"strconv"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
)

var _ notifierapi.Broker = &broker{}

// broker without lock, because it is itended to only be called inside FSM
type broker struct {
	// todo: check whether we need lock or not if we're using FSM..
	// of course to be safe we can just add them.

	listener map[uint64]notifierapi.Subscription

	listenerBufferSize uint32
	async              bool
	exitMessage        any

	csf CreateSubscription
}

type CreateSubscription func() notifierapi.Subscription

// NewBroker maintains a listener pool
func NewBroker(csf CreateSubscription) notifierapi.Broker {
	return &broker{
		listener: make(map[uint64]notifierapi.Subscription),
		csf:      csf,
	}
}

func (s *broker) Subscribe() (string, notifierapi.Subscription) {
	id := rand.Uint64() // change id impl to avoid conflict if needed

	s.listener[id] = s.csf()

	// todo: add timeout if there is no active listener to close the listener.

	return strconv.FormatUint(id, 10), s.listener[id]
}

func (s *broker) GetSubscription(id string) (notifierapi.Subscription, error) {
	iduint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, err
	}

	return s.listener[iduint], nil
}

func (s *broker) Broadcast(ctx context.Context, message any) {
	for key, listener := range s.listener {
		err := listener.Publish(ctx, message)
		if err != nil {
			delete(s.listener, key)
		}
	}
}
