package impl

import (
	"context"
	"math/rand/v2"
	"strconv"
	"sync"

	notifierapi "github.com/desain-gratis/common/delivery/log-api"
)

var _ notifierapi.Topic = &topic{}

// topic
type topic struct {
	listener map[uint64]notifierapi.Subscription
	lock     *sync.RWMutex
	csf      CreateSubscription
}

type CreateSubscription func() notifierapi.Subscription

// NewTopic create a new topic with create subscription function
func NewTopic(csf CreateSubscription) notifierapi.Topic {
	return &topic{
		listener: make(map[uint64]notifierapi.Subscription),
		csf:      csf,
		lock:     &sync.RWMutex{},
	}
}

func (s *topic) Subscribe() (string, notifierapi.Subscription) {
	id := rand.Uint64() // change id impl to avoid conflict if needed

	s.lock.Lock()
	s.listener[id] = s.csf()
	s.lock.Unlock()

	// todo: add timeout if there is no active listener to close the listener.

	return strconv.FormatUint(id, 10), s.listener[id]
}

func (s *topic) GetSubscription(id string) (notifierapi.Subscription, error) {
	iduint, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return nil, err
	}

	s.lock.RLock()
	defer s.lock.RUnlock()

	l := s.listener[iduint]

	return l, nil
}

func (s *topic) Broadcast(ctx context.Context, message any) {
	delKey := make([]uint64, 0)

	wg := new(sync.WaitGroup)
	for key, listener := range s.listener {
		wg.Add(1)
		go func(k uint64, l notifierapi.Subscription) {
			defer wg.Done()
			err := listener.Publish(ctx, message)
			if err != nil {
				delKey = append(delKey, key)
			}
		}(key, listener)
	}
	wg.Wait()

	s.lock.Lock()
	defer s.lock.Unlock()
	for _, key := range delKey {
		delete(s.listener, key)
	}
}
